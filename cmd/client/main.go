package main

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "localhost:8080", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	cl := healthpb.NewHealthClient(conn)
	resp, err := cl.Check(ctx, &healthpb.HealthCheckRequest{Service: ""})
	fmt.Println("health:", resp, "err:", err)

}
