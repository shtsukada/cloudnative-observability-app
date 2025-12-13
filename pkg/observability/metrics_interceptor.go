package observability

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func UnaryMetricsInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {

	start := time.Now()

	mode := ""
	endpoint := info.FullMethod

	// in_flight++
	CNOAppRequestsInFlight.WithLabelValues(mode, endpoint).Inc()
	defer CNOAppRequestsInFlight.WithLabelValues(mode, endpoint).Dec()

	// handler実行
	resp, err := handler(ctx, req)

	// status code
	st, _ := status.FromError(err)
	code := "OK"
	if st != nil {
		code = st.Code().String()
	}

	// Counter
	CNOAppRequestsTotal.WithLabelValues(mode, endpoint, code).Inc()

	// Histogram
	latency := time.Since(start).Seconds()
	CNOAppRequestLatency.WithLabelValues(mode, endpoint, code).Observe(latency)

	return resp, err
}

func StreamMetricsInterceptor(
	srv any,
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	start := time.Now()

	mode := ""
	endpoint := info.FullMethod

	CNOAppRequestsInFlight.WithLabelValues(mode, endpoint).Inc()
	defer CNOAppRequestsInFlight.WithLabelValues(mode, endpoint).Dec()

	err := handler(srv, ss)

	st, _ := status.FromError(err)
	code := "OK"
	if st != nil {
		code = st.Code().String()
	}

	CNOAppRequestsTotal.WithLabelValues(mode, endpoint, code).Inc()

	latency := time.Since(start).Seconds()
	CNOAppRequestLatency.WithLabelValues(mode, endpoint, code).Observe(latency)

	return err
}
