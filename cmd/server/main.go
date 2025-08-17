package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/shtsukada/cloudnative-observability-app/pkg/observability"
)

const (
	grpcAddr    = ":8080"
	metricsAddr = ":9090"
)

func main() {
	logger := observability.NewLogger()
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	metricsSrv := &http.Server{Addr: metricsAddr, Handler: mux}

	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		logger.Fatal("failed to listen", "addr", grpcAddr, "err", err)
	}
	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(observability.UnaryLoggingInterceptor(logger)),
	)
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	reflection.Register(grpcSrv)

	go func() {
		logger.Info("metrics http starting", "addr", metricsAddr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics http error", "err", err)
		}
	}()
	go func() {
		logger.Info("grpc starting", "addr", grpcAddr)
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
