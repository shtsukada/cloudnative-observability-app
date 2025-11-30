package observability

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	CNOAppRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cno_app_requests_total",
			Help: "Total number of gRPC requests handled by the application.",
		},
		[]string{"mode", "endpoint", "code"},
	)

	CNOAppRequestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cno_app_request_latency_seconds",
			Help:    "Latency of gRPC requests.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"mode", "endpoint", "code"},
	)

	CNOAppRequestsInFlight = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cno_app_requests_in_flight",
			Help: "Number of in-flight gRPC requests being handled.",
		},
		[]string{"mode", "endpoint"},
	)
)

func init() {
	prometheus.MustRegister(CNOAppRequestsTotal)
	prometheus.MustRegister(CNOAppRequestLatency)
	prometheus.MustRegister(CNOAppRequestsInFlight)
}
