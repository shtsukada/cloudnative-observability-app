package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/shtsukada/cloudnative-observability-app/pkg/observability"
	grpcburnerv1 "github.com/shtsukada/cloudnative-observability-proto/gen/go/observability/grpcburner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type options struct {
	Addr    string
	Timeout time.Duration
	Mode    string
	Payload string
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

	// NOTE:今はHealthを叩くだけだが、PingRequest型をimportしておき、
	// 今後Dowork/Ping呼び出しに差し替えるまで「proto依存」にしておく
	_ = grpcburnerv1.PingRequest{}

	conn, err := grpc.NewClient(
		opts.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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
	mode := fs.String("mode", modeDefault, "client mode (health, do-work, ...)")
	payload := fs.String("payload", payloadDefault, "optional payload for future DoWork RPC")

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

	return &options{
		Addr:    *addr,
		Timeout: dur,
		Mode:    *mode,
		Payload: *payload,
	}, nil
}

// callHealthはHealthチェックRPCを実行し、
// request_id 生成+metadata伝播/終了ログを出力する
func callHealth(conn *grpc.ClientConn, opts *options) error {
	logger := observability.NewLogger()
	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	// request_id 生成&metadataへ設定
	requestID := uuid.New().String()

	md := metadata.New(map[string]string{
		"x-request-id": requestID,
		// TODO: modeをmetadataで伝播する場合は後続ブランチで利用
		// "x-mode": opts.Mode,
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	// 送信リクエスト(HealthCheck)
	req := &healthpb.HealthCheckRequest{Service: ""}

	// bytes_out:リクエストサイズ
	reqBytes, _ := proto.Marshal(req)

	start := time.Now()

	// リクエスト開始ログ
	logger.Infow("client request start",
		"mode", opts.Mode,
		"addr", opts.Addr,
		"request_id", requestID,
	)

	cl := healthpb.NewHealthClient(conn)
	resp, err := cl.Check(ctx, req)

	latencyMs := time.Since(start).Milliseconds()

	// bytes_in: レスポンスサイズ
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

func getenvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
