package main

import (
	"context"
	"fmt"
	"time"

	grpcburnerv1 "github.com/shtsukada/cloudnative-observability-proto/gen/go/observability/grpcburner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	_ = grpcburnerv1.PingRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.NewClient("localhost:8080", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	cl := healthpb.NewHealthClient(conn)
	resp, err := cl.Check(ctx, &healthpb.HealthCheckRequest{Service: ""})
	fmt.Println("health:", resp, "err:", err)

}
