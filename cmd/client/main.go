package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/shtsukada/cloudnative-observability-app/pkg/observability"
	grpcburnerv1 "github.com/shtsukada/cloudnative-observability-proto/gen/go/observability/grpcburner/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type options struct {
	Addr         string
	Timeout      time.Duration
	Mode         string
	Payload      string
	WorkMode     string
	WorkDuration time.Duration
	AllocMB      int
	Parallelism  int
	IOBytes      int
	Latency      time.Duration
	ErrorRate    float64
	Repeat       int
}

const (
	defaultAddr    = "localhost:8080"
	defaultTimeout = "3s"

	envAddr    = "CNO_APP_CLIENT_ADDR"
	envTimeout = "CNO_APP_CLIENT_TIMEOUT"
	envMode    = "CNO_APP_CLIENT_MODE"
	envPayload = "CNO_APP_CLIENT_PAYLOAD"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "client error:", err)
		os.Exit(1)
	}
}

func run() error {
	opts, err := parseOptions()
	if err != nil {
		return err
	}

	// TracerProviderをクライアント用に初期化
	ctx := context.Background()
	shutdown, err := observability.InitClientTracerProvider(ctx)
	if err != nil {
		return fmt.Errorf("init client tracer provider: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdown(shutdownCtx)
	}()

	// NOTE:今はHealthを叩くだけだが、PingRequest型をimportしておき、
	// 今後Dowork/Ping呼び出しに差し替えるまで「proto依存」にしておく
	_ = grpcburnerv1.PingRequest{}

	conn, err := grpc.NewClient(
		opts.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return fmt.Errorf("failed to dial %s: %w", opts.Addr, err)
	}
	defer func() {
		_ = conn.Close()
	}()

	switch opts.Mode {
	case "health", "":
		return callHealth(conn, opts)
	case "ping":
		return callPing(conn, opts)
	case "do-work-unary":
		return callDoWorkUnary(conn, opts)
	case "do-work-server":
		return callDoWorkServerStreaming(conn, opts)
	case "do-work-client":
		return callDoWorkClientStreaming(conn, opts)
	case "do-work-bidi":
		return callDoWorkBidiStreaming(conn, opts)
	default:
		return fmt.Errorf("unsupported mode %q", opts.Mode)
	}
}

func parseOptions() (*options, error) {
	addrDefault := getenvOrDefault(envAddr, defaultAddr)
	timeoutDefault := getenvOrDefault(envTimeout, defaultTimeout)
	modeDefault := getenvOrDefault(envMode, "health")
	payloadDefault := getenvOrDefault(envPayload, "")

	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	addr := fs.String("addr", addrDefault, "gRPC server address (host:port)")
	timeoutStr := fs.String("timeout", timeoutDefault, "request timeout (e.g. 3s, 500ms)")
	mode := fs.String("mode", modeDefault, "client mode (health, ,ping, do-work-unary, do-work-server, do-work-client, do-work-bidi)")
	payload := fs.String("payload", payloadDefault, "optional payload for future use")

	workMode := fs.String("work-mode", "cpu", "work load mode (cpu, mem, cpu-mem, io)")
	workDuration := fs.Duration("work-duration", 3*time.Second, "duration for each work (e.g. 3s)")
	allocMB := fs.Int("alloc-mb", 32, "memory allocation in MB for mem/cpu-mem mode")
	parallelism := fs.Int("parallelism", 1, "number of goroutines for cpu/cpu-mem mode")
	ioBytes := fs.Int("io-bytes", 1024*64, "I/O bytes per loop for io mode")
	latency := fs.Duration("latency", 0, "fixed latency per work (e.g. 200ms)")
	errorRate := fs.Float64("error-rate", 0.0, "error rate between 0.0 and 1.0")

	repeat := fs.Int("repeat", 3, "number of works for streaming modes")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	dur, err := time.ParseDuration(*timeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout %q: %w", *timeoutStr, err)
	}
	if dur <= 0 {
		return nil, fmt.Errorf("timeout must be > 0, got %s", dur)
	}

	if *repeat <= 0 {
		return nil, fmt.Errorf("repeat must be > 0, got %d", *repeat)
	}

	return &options{
		Addr:         *addr,
		Timeout:      dur,
		Mode:         *mode,
		Payload:      *payload,
		WorkMode:     *workMode,
		WorkDuration: *workDuration,
		AllocMB:      *allocMB,
		Parallelism:  *parallelism,
		IOBytes:      *ioBytes,
		Latency:      *latency,
		ErrorRate:    *errorRate,
		Repeat:       *repeat,
	}, nil
}

// callHealth は HealthチェックRPCを実行し、
// request_id 生成+metadata伝播/trace_id付きログを出力する
func callHealth(conn *grpc.ClientConn, opts *options) error {
	logger := observability.NewLogger()
	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	requestID := uuid.New().String()

	md := metadata.New(map[string]string{
		"x-request-id": requestID,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	tracer := otel.Tracer("cno-app-client")
	ctx, span := tracer.Start(ctx, "grpc.client/HealthCheck")
	defer span.End()

	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	traceID := spanCtx.TraceID().String()

	req := &healthpb.HealthCheckRequest{Service: ""}

	reqBytes, _ := proto.Marshal(req)

	start := time.Now()

	// リクエスト開始ログ
	logger.Infow("client request start",
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"request_id", requestID,
	)

	cl := healthpb.NewHealthClient(conn)
	resp, err := cl.Check(ctx, req)

	latencyMs := time.Since(start).Milliseconds()

	bytesIn := 0
	if resp != nil {
		if b, errMarshal := proto.Marshal(resp); errMarshal == nil {
			bytesIn = len(b)
		}
	}

	code := "OK"
	if st, ok := status.FromError(err); ok {
		code = st.Code().String()
	}

	fields := []any{
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"request_id", requestID,
		"code", code,
		"latency_ms", latencyMs,
		"bytes_out", len(reqBytes),
		"bytes_in", bytesIn,
	}

	if err != nil {
		fields = append(fields, "error", err)
		logger.Errorw("client request end", fields...)
		return fmt.Errorf("health check failed: %w", err)
	}

	logger.Infow("client request end", fields...)

	fmt.Printf("health:%+v\n", resp)

	return nil
}

func callPing(conn *grpc.ClientConn, opts *options) error {
	logger := observability.NewLogger()
	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	requestID := uuid.New().String()

	md := metadata.New(map[string]string{
		"x-request-id": requestID,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	tracer := otel.Tracer("cno-app-client")
	ctx, span := tracer.Start(ctx, "grpc.client/Burner.Ping")
	defer span.End()

	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	traceID := spanCtx.TraceID().String()

	req := &grpcburnerv1.PingRequest{}
	reqBytes, _ := proto.Marshal(req)

	start := time.Now()

	logger.Infow("client request start",
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"request_id", requestID,
	)

	cl := grpcburnerv1.NewBurnerClient(conn)
	resp, err := cl.Ping(ctx, req)

	latencyMs := time.Since(start).Milliseconds()

	bytesIn := 0
	if resp != nil {
		if b, errMarshal := proto.Marshal(resp); errMarshal == nil {
			bytesIn = len(b)
		}
	}

	code := "OK"
	if st, ok := status.FromError(err); ok {
		code = st.Code().String()
	}

	fields := []any{
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"request_id", requestID,
		"code", code,
		"latency_ms", latencyMs,
		"bytes_out", len(reqBytes),
		"bytes_in", bytesIn,
	}

	if err != nil {
		fields = append(fields, "error", err)
		logger.Errorw("client request end", fields...)
		return fmt.Errorf("ping failed: %w", err)
	}

	logger.Infow("client request end", fields...)

	fmt.Printf("ping reply: %s\n", resp.GetMessage())
	return nil
}

func callDoWorkUnary(conn *grpc.ClientConn, opts *options) error {
	logger := observability.NewLogger()
	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	requestID := uuid.New().String()
	md := metadata.New(map[string]string{
		"x-request-id": requestID,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	tracer := otel.Tracer("cno-app-client")
	ctx, span := tracer.Start(ctx, "grpc.client/Burner.DoWork")
	defer span.End()

	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	traceID := spanCtx.TraceID().String()

	wc, err := workConfigFromOptions(opts)
	if err != nil {
		return err
	}

	req := &grpcburnerv1.DoWorkRequest{
		RequestId: requestID,
		Config:    wc,
	}
	reqBytes, _ := proto.Marshal(req)

	start := time.Now()

	logger.Infow("client request start",
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"request_id", requestID,
		"work_mode", opts.WorkMode,
	)

	cl := grpcburnerv1.NewBurnerClient(conn)
	resp, err := cl.DoWork(ctx, req)

	latencyMs := time.Since(start).Milliseconds()

	bytesIn := 0
	if resp != nil {
		if b, errMarshal := proto.Marshal(resp); errMarshal == nil {
			bytesIn = len(b)
		}
	}

	code := "OK"
	if st, ok := status.FromError(err); ok {
		code = st.Code().String()
	}

	fields := []any{
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"request_id", requestID,
		"code", code,
		"latency_ms", latencyMs,
		"bytes_out", len(reqBytes),
		"bytes_in", bytesIn,
		"work_mode", opts.WorkMode,
	}

	if err != nil {
		fields = append(fields, "error", err)
		logger.Errorw("client request end", fields...)
		return fmt.Errorf("do-work failed: %w", err)
	}

	logger.Infow("client request end", fields...)

	fmt.Printf("do-work unary: ok=%v error=%s\n", resp.GetOk(), resp.GetErrorMessage())
	return nil
}

func callDoWorkServerStreaming(conn *grpc.ClientConn, opts *options) error {
	logger := observability.NewLogger()
	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	requestID := uuid.New().String()
	md := metadata.New(map[string]string{
		"x-request-id": requestID,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	tracer := otel.Tracer("cno-app-client")
	ctx, span := tracer.Start(ctx, "grpc.client/Burner.DoWorkServerStreaming")
	defer span.End()

	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	traceID := spanCtx.TraceID().String()

	wc, err := workConfigFromOptions(opts)
	if err != nil {
		return err
	}

	rep32, err := mustInt32("repeat", opts.Repeat)
	if err != nil {
		return err
	}

	req := &grpcburnerv1.DoWorkServerStreamingRequest{
		RequestId: requestID,
		Config:    wc,
		Repeat:    rep32,
	}
	reqBytes, _ := proto.Marshal(req)

	start := time.Now()

	logger.Infow("client request start",
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"request_id", requestID,
		"repeat", opts.Repeat,
		"work_mode", opts.WorkMode,
	)

	cl := grpcburnerv1.NewBurnerClient(conn)
	stream, err := cl.DoWorkServerStreaming(ctx, req)
	if err != nil {
		logger.Errorw("stream open error", "err", err)
		return fmt.Errorf("do-work-server: open stream: %w", err)
	}

	recvCount := 0
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Errorw("stream recv error", "err", err)
			return fmt.Errorf("do-work-server: recv: %w", err)
		}
		recvCount++
		fmt.Printf("server stream [%d/%d]: ok=%v error=%s\n",
			recvCount, opts.Repeat, resp.GetOk(), resp.GetErrorMessage())
	}

	latencyMs := time.Since(start).Milliseconds()

	code := "OK"
	fields := []any{
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"request_id", requestID,
		"code", code,
		"latency_ms", latencyMs,
		"bytes_out", len(reqBytes),
		"recv_count", recvCount,
		"repeat", opts.Repeat,
	}

	logger.Infow("client request end", fields...)

	return nil
}

func callDoWorkClientStreaming(conn *grpc.ClientConn, opts *options) error {
	logger := observability.NewLogger()
	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	md := metadata.New(nil)
	ctx = metadata.NewOutgoingContext(ctx, md)

	tracer := otel.Tracer("cno-app-client")
	ctx, span := tracer.Start(ctx, "grpc.client/Burner.DoWorkClientStreaming")
	defer span.End()

	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	traceID := spanCtx.TraceID().String()

	wc, err := workConfigFromOptions(opts)
	if err != nil {
		return err
	}

	cl := grpcburnerv1.NewBurnerClient(conn)
	stream, err := cl.DoWorkClientStreaming(ctx)
	if err != nil {
		logger.Errorw("stream open error", "err", err)
		return fmt.Errorf("do-work-client: open stream: %w", err)
	}

	start := time.Now()

	logger.Infow("client stream start",
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"repeat", opts.Repeat,
		"work_mode", opts.WorkMode,
	)

	sent := 0
	for i := 0; i < opts.Repeat; i++ {
		requestID := uuid.New().String()
		req := &grpcburnerv1.DoWorkRequest{
			RequestId: requestID,
			Config:    wc,
		}
		if err := stream.Send(req); err != nil {
			logger.Errorw("stream send error", "err", err)
			return fmt.Errorf("do-work-client: send: %w", err)
		}
		sent++
	}

	summary, err := stream.CloseAndRecv()
	if err != nil {
		logger.Errorw("stream close/recv error", "err", err)
		return fmt.Errorf("do-work-client: close/recv: %w", err)
	}

	latencyMs := time.Since(start).Milliseconds()

	fields := []any{
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"code", "OK",
		"latency_ms", latencyMs,
		"sent", sent,
		"summary_total", summary.GetTotal(),
		"summary_success", summary.GetSuccess(),
		"summary_failed", summary.GetFailed(),
	}

	logger.Infow("client stream end", fields...)

	fmt.Printf("client stream summary: total=%d success=%d failed=%d\n", summary.GetTotal(), summary.GetSuccess(), summary.GetFailed())
	return nil
}

func callDoWorkBidiStreaming(conn *grpc.ClientConn, opts *options) error {
	logger := observability.NewLogger()
	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	md := metadata.New(nil)
	ctx = metadata.NewOutgoingContext(ctx, md)

	tracer := otel.Tracer("cno-app-client")
	ctx, span := tracer.Start(ctx, "grpc.client/Burner.DoWorkBidiStreaming")
	defer span.End()

	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	traceID := spanCtx.TraceID().String()

	wc, err := workConfigFromOptions(opts)
	if err != nil {
		return err
	}

	cl := grpcburnerv1.NewBurnerClient(conn)
	stream, err := cl.DoWorkBidiStreaming(ctx)
	if err != nil {
		logger.Errorw("stream open error", "err", err)
		return fmt.Errorf("do-work-bidi: open stream: %w", err)
	}

	start := time.Now()

	logger.Infow("client bidi start",
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"repeat", opts.Repeat,
		"work_mode", opts.WorkMode,
	)

	sent := 0
	received := 0

	for i := 0; i < opts.Repeat; i++ {
		requestID := uuid.New().String()
		req := &grpcburnerv1.DoWorkRequest{
			RequestId: requestID,
			Config:    wc,
		}
		if err := stream.Send(req); err != nil {
			logger.Errorw("bidi send error", "err", err)
			return fmt.Errorf("do-work-bidi: send: %w", err)
		}
		sent++

		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Errorw("bidi recv error", "err", err)
			return fmt.Errorf("do-work-bidi: recv: %w", err)
		}
		received++

		fmt.Printf("bidi [%d/%d]: ok=%v error=%s\n", received, opts.Repeat, resp.GetOk(), resp.GetErrorMessage())
	}

	if err := stream.CloseSend(); err != nil {
		logger.Errorw("bidi close send error", "err", err)
	}

	latencyMs := time.Since(start).Milliseconds()

	fields := []any{
		"trace_id", traceID,
		"mode", opts.Mode,
		"addr", opts.Addr,
		"code", "OK",
		"latency_ms", latencyMs,
		"sent", sent,
		"received", received,
	}

	logger.Infow("client bidi end", fields...)

	return nil

}

func workConfigFromOptions(opts *options) (*grpcburnerv1.WorkConfig, error) {
	var mode grpcburnerv1.LoadMode

	switch opts.WorkMode {
	case "cpu":
		mode = grpcburnerv1.LoadMode_LOAD_MODE_CPU
	case "mem":
		mode = grpcburnerv1.LoadMode_LOAD_MODE_MEM
	case "cpu-mem":
		mode = grpcburnerv1.LoadMode_LOAD_MODE_CPU_MEM
	case "io":
		mode = grpcburnerv1.LoadMode_LOAD_MODE_IO
	default:
		return nil, fmt.Errorf("invalid work-mode %q (expected cpu|mem|cpu-mem|io)", opts.WorkMode)
	}

	if opts.WorkDuration <= 0 {
		return nil, fmt.Errorf("work-duration must be > 0")
	}
	if opts.ErrorRate < 0.0 || opts.ErrorRate > 1.0 {
		return nil, fmt.Errorf("error-rate must be between 0.0 and 1.0")
	}

	alloc32, err := mustInt32("alloc-mb", opts.AllocMB)
	if err != nil {
		return nil, err
	}
	par32, err := mustInt32("parallelism", opts.Parallelism)
	if err != nil {
		return nil, err
	}
	io32, err := mustInt32("io-bytes", opts.IOBytes)
	if err != nil {
		return nil, err
	}

	wc := &grpcburnerv1.WorkConfig{
		Mode:        mode,
		DurationMs:  int64(opts.WorkDuration / time.Millisecond),
		AllocMb:     alloc32,
		Parallelism: par32,
		IoBytes:     io32,
		LatencyMs:   int64(opts.Latency / time.Millisecond),
		ErrorRate:   opts.ErrorRate,
	}
	return wc, nil
}

func mustInt32(name string, v int) (int32, error) {
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, fmt.Errorf("%s out of int32 range: %d", name, v)
	}
	return int32(v), nil
}

func getenvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
