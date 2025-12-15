#!/usr/bin/env bash
set -euo pipefail

: "${IMAGE_TAG:?IMAGE_TAG is required}"
: "${NAMESPACE:=cno}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MANIFEST="${ROOT_DIR}/hack/ci/kind-manifest.yaml"

cleanup() {
  set +e
  if [[ -n "${PF_GRPC_PID:-}" ]]; then kill "${PF_GRPC_PID}" >/dev/null 2>&1 || true; fi
  if [[ -n "${PF_METRICS_PID:-}" ]]; then kill "${PF_METRICS_PID}" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT

echo "[smoke] apply manifest (ns=${NAMESPACE})"
kubectl apply -f "${MANIFEST}"

echo "[smoke] set image to ${IMAGE_TAG}"
kubectl -n "${NAMESPACE}" set image deployment/grpc-burner server="${IMAGE_TAG}"

echo "[smoke] wait for deployment ready"
kubectl -n "${NAMESPACE}" rollout status deploy/grpc-burner --timeout=180s

echo "[smoke] port-forward grpc(:8080) + metrics(:9090)"
kubectl -n "${NAMESPACE}" port-forward svc/grpc-burner 15051:8080 >/tmp/pf-grpc.log 2>&1 &
PF_GRPC_PID=$!
kubectl -n "${NAMESPACE}" port-forward svc/grpc-burner-metrics 18080:9090 >/tmp/pf-metrics.log 2>&1 &
PF_METRICS_PID=$!

sleep 2

echo "[smoke] install grpc health probe (pinned)"
go install github.com/grpc-ecosystem/grpc-health-probe@v0.4.40
PROBE_BIN="$(go env GOPATH)/bin/grpc-health-probe"

echo "[smoke] grpc health check"
"${PROBE_BIN}" -addr 127.0.0.1:15051

echo "[smoke] check /metrics contains expected metrics"
curl -sf "http://127.0.0.1:18080/metrics" | grep -q "cno_app_requests_total"
curl -sf "http://127.0.0.1:18080/metrics" | grep -q "cno_app_requests_in_flight"

echo "[smoke] OK"
