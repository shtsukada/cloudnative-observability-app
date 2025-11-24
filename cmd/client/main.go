package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	grpcburnerv1 "github.com/shtsukada/cloudnative-observability-proto/gen/go/observability/grpcburner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
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
		return callHealth(conn, opts.Timeout)
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

func callHealth(conn *grpc.ClientConn, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cl := healthpb.NewHealthClient(conn)
	resp, err := cl.Check(ctx, &healthpb.HealthCheckRequest{Service: ""})
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	fmt.Printf("health:%+v\n", resp)
	return nil
}

func getenvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
