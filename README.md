# cloudnative-observability-app

gRPC サーバ・クライアントを実装した負荷生成アプリケーションです。

## 成果物
- gRPC サーバ & クライアント
- 負荷モード: CPU / Memory / Latency / Errors / Streaming
- Prometheus メトリクス
- zap JSON ログ（trace_id, request_id 含む）
- OTLP トレース送信

## 契約
- readiness/liveness: /healthz
- buildx (linux/amd64) + cosign 署名 + syft SBOM
- values から image/tag/env を切替可能

## Quickstart
```bash
docker run --rm ghcr.io/YOUR_ORG/grpc-burner:TAG --mode=cpu
```

## MVP
- gRPC (Unary/Streaming)
- 負荷モード
- Prometheus metrics
- zap logs
- OTLP traces
- readiness/liveness

## Plus
- 設定再読込（SIGHUP）
- 動的に遅延/エラー率を変更

## 受け入れ基準チェックリスト
- [ ] Unary/Streaming 全モードでレスポンス成功
- [ ] 各負荷モードでリソース変化確認
- [ ] /metrics が期待値を返す
- [ ] zap ログに trace_id が含まれる
- [ ] Tempo にトレースが到達

## スコープ外
- Operator/監視スタック自体の実装

## ライセンス
MIT License