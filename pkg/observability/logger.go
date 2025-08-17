package observability

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func NewLogger() *zap.SugaredLogger {
	l, _ := zap.NewProduction()
	return l.Sugar()
}

func UnaryLoggingInterceptor(log *zap.SugaredLogger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		log.Infow("grpc",
			"method", info.FullMethod,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return resp, err
	}
}
