package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/shtsukada/cloudnative-observability-app/pkg/observability"
	appserver "github.com/shtsukada/cloudnative-observability-app/pkg/server"
	grpcburnerv1 "github.com/shtsukada/cloudnative-observability-proto/gen/go/observability/grpcburner/v1"
)

const (
	grpcAddr    = ":8080"
	metricsAddr = ":9090"
)

// newGRPCServer は interceptor やオプションを差し込みやすいよう、
// grpc.NewServer のラッパーとして定義しておく
// func newGRPCServer() *grpc.Server {
// 	// 後続ブランチ (logging / metrics / tracing) で ServerOption を追加する予定
// 	return grpc.NewServer()
// }

// registerGRPCServices は gRPC サーバーに標準サービスとアプリケーションサービスを登録する
func registerGRPCServices(s *grpc.Server) {
	// HealthCheck
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(s, healthServer)
	// デフォルトサービス名 "" を SERVINGにしておく
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	// アプリケーションのgRPCサービス
	burner := appserver.NewGrpcBurnerServer()
	grpcburnerv1.RegisterBurnerServer(s, burner)

	// Reflection
	reflection.Register(s)
}

func newHTTPMux() http.Handler {
	mux := http.NewServeMux()

	// Prometheusメトリクス
	mux.Handle("/metrics", promhttp.Handler())

	// シンプルなヘルスチェック
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		// 将来的に gRPC health の状態を見に行く実装に差し替えても良い
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func newHTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           newHTTPMux(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func main() {
	logger := observability.NewLogger()

	ctx := context.Background()
	tracingShutdown, err := observability.InitTracerProvider(ctx)
	if err != nil {
		logger.Fatal("failed to init tracing", "err", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracingShutdown(shutdownCtx); err != nil {
			logger.Error("failed to shutdown tracer provider", "err", err)
		}
	}()

	metricsSrv := newHTTPServer(metricsAddr)

	grpcLis, err := net.Listen("tcp", grpcAddr) //nolint:gosec
	if err != nil {
		logger.Fatal("failed to listen", "addr", grpcAddr, "err", err)
	}

	otelHandler := otelgrpc.NewServerHandler(
		otelgrpc.WithTracerProvider(otel.GetTracerProvider()),
	)

	grpcSrv := grpc.NewServer(
		grpc.StatsHandler(otelHandler),
		grpc.ChainUnaryInterceptor(
			grpc_prometheus.UnaryServerInterceptor,
			observability.UnaryMetricsInterceptor,
			observability.UnaryLoggingInterceptor(logger),
		),
		grpc.ChainStreamInterceptor(
			grpc_prometheus.StreamServerInterceptor,
			observability.StreamMetricsInterceptor,
		),
	)
	grpc_prometheus.Register(grpcSrv)
	registerGRPCServices(grpcSrv)

	go func() {
		logger.Infow("metrics http starting", "addr", metricsAddr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics http error", "err", err)
		}
	}()
	go func() {
		logger.Infow("grpc starting", "addr", grpcAddr)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			logger.Error("grpc serve error", "err", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	logger.Info("shutting down...")

	grpcSrv.GracefulStop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = metricsSrv.Shutdown(ctx)

	logger.Info("bye")
}
