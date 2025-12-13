package server

import (
	// "context"

	"context"
	"fmt"
	"io"
	"time"

	"github.com/shtsukada/cloudnative-observability-app/pkg/load"
	grpcburnerv1 "github.com/shtsukada/cloudnative-observability-proto/gen/go/observability/grpcburner/v1"
)

// GrpcBurnerServer は grpcburnerv1.GrpcBurnerServerを実装する
type GrpcBurnerServer struct {
	grpcburnerv1.UnimplementedBurnerServer
}

// NewGrpcBurnerServer はアプリの gRPCサーバー実装を返す
// 後続ブランチで logger や設定を差し込む際に拡張しやすいよう、コンストラクタ関数とする
func NewGrpcBurnerServer() *GrpcBurnerServer {
	return &GrpcBurnerServer{}
}

// Pingは軽量な到達確認用 RPC
func (s *GrpcBurnerServer) Ping(
	ctx context.Context,
	_ *grpcburnerv1.PingRequest,
) (*grpcburnerv1.PingReply, error) {
	_ = ctx
	return &grpcburnerv1.PingReply{Message: "pong"}, nil
}

// DoWorkは Unary 型の負荷実行 RPC
func (s *GrpcBurnerServer) DoWork(
	ctx context.Context,
	req *grpcburnerv1.DoWorkRequest,
) (*grpcburnerv1.DoWorkResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	cfg, err := workConfigFromProto(req.GetConfig())
	if err != nil {
		return &grpcburnerv1.DoWorkResponse{
			RequestId:    req.GetRequestId(),
			Ok:           false,
			ErrorMessage: fmt.Sprintf("invalid config: %v", err),
		}, nil
	}

	if err := load.Run(ctx, cfg); err != nil {
		return &grpcburnerv1.DoWorkResponse{
			RequestId:    req.GetRequestId(),
			Ok:           false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &grpcburnerv1.DoWorkResponse{
		RequestId: req.GetRequestId(),
		Ok:        true,
	}, nil
}

// DoWorkServerStreaming は同じ config を repeat回実行し、その結果をストリームで返す
func (s *GrpcBurnerServer) DoWorkServerStreaming(
	req *grpcburnerv1.DoWorkServerStreamingRequest,
	stream grpcburnerv1.Burner_DoWorkServerStreamingServer,
) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if req.GetRepeat() <= 0 {
		return fmt.Errorf("repeat must be > 0")
	}

	cfg, err := workConfigFromProto(req.GetConfig())
	if err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	ctx := stream.Context()

	for i := int32(0); i < req.GetRepeat(); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		runErr := load.Run(ctx, cfg)
		resp := &grpcburnerv1.DoWorkResponse{
			RequestId: req.GetRequestId(),
			Ok:        runErr == nil,
		}
		if runErr != nil {
			resp.ErrorMessage = runErr.Error()
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return nil
}

// DoWorkClientStreaming は複数の DoWorkRequestを受信し、左マリを1回返す
func (s *GrpcBurnerServer) DoWorkClientStreaming(
	stream grpcburnerv1.Burner_DoWorkClientStreamingServer,
) error {
	var (
		total      int32
		success    int32
		failed     int32
		summaryReq string
	)

	ctx := stream.Context()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		total++
		if summaryReq == "" {
			summaryReq = req.GetRequestId()
		}

		cfg, cfgErr := workConfigFromProto(req.GetConfig())
		if cfgErr != nil {
			failed++
			continue
		}

		if err := load.Run(ctx, cfg); err != nil {
			failed++
		} else {
			success++
		}
	}

	summary := &grpcburnerv1.DoWorkSummary{
		RequestId: summaryReq,
		Total:     total,
		Success:   success,
		Failed:    failed,
	}
	return stream.SendAndClose(summary)
}

// DoWorkBidiStreamingはリクエストごとにDoWorkを実行し、その結果を逐次返す。
func (s *GrpcBurnerServer) DoWorkBidiStreaming(
	stream grpcburnerv1.Burner_DoWorkBidiStreamingServer,
) error {
	ctx := stream.Context()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		cfg, cfgErr := workConfigFromProto(req.GetConfig())
		resp := &grpcburnerv1.DoWorkResponse{
			RequestId: req.GetRequestId(),
			Ok:        cfgErr == nil,
		}

		if cfgErr == nil {
			if err := load.Run(ctx, cfg); err != nil {
				resp.Ok = false
				resp.ErrorMessage = err.Error()
			}
		} else {
			resp.ErrorMessage = cfgErr.Error()
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

func workConfigFromProto(pc *grpcburnerv1.WorkConfig) (load.Config, error) {
	if pc == nil {
		return load.Config{}, fmt.Errorf("config is required")
	}

	var mode load.Mode
	switch pc.GetMode() {
	case grpcburnerv1.LoadMode_LOAD_MODE_CPU:
		mode = load.ModeCPU
	case grpcburnerv1.LoadMode_LOAD_MODE_MEM:
		mode = load.ModeMem
	case grpcburnerv1.LoadMode_LOAD_MODE_CPU_MEM:
		mode = load.ModeCPUMem
	case grpcburnerv1.LoadMode_LOAD_MODE_IO:
		mode = load.ModeIO
	default:
		return load.Config{}, fmt.Errorf("unsupported mode: %v", pc.GetMode())
	}

	cfg := load.Config{
		Mode:        mode,
		Duration:    time.Duration(pc.GetDurationMs()) * time.Millisecond,
		AllocMB:     int(pc.GetAllocMb()),
		Parallelism: int(pc.GetParallelism()),
		IOBytes:     int(pc.GetIoBytes()),
		Latency:     time.Duration(pc.GetLatencyMs()) * time.Millisecond,
		ErrorRate:   pc.GetErrorRate(),
	}

	return cfg, nil
}
