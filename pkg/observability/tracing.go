package observability

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// InitTracerProvider は OpenTelemetry TracerProviderを初期化し、
// gRPCサーバーのトレースが Collector(Tempo) に送信されるように設定する。
// 戻り値の shutdown はアプリ終了時に呼び出す。
func InitTracerProvider(ctx context.Context) (func(context.Context) error, error) {
	// サーバ側用: service.name = "cno-app"
	return initTracerProvider(ctx, "cno-app")
}

// InitClientTracerProvider は gRPCクライアント用のTracerProviderを初期化する。
// 基本設定はサーバー側と揃えつつ、service.Name だけ "cno-app-client"に変える。
func InitClientTracerProvider(ctx context.Context) (func(context.Context) error, error) {
	return initTracerProvider(ctx, "cno-app-client")
}

// initTracerProviderは service.Nameだけを引数で切り替える共通実装。
func initTracerProvider(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	// OTEL_EXPORTER_OTLP_ENDPOINT が未設定ならローカルCollectorを前提にする
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	// "http://localhost:4317" 形式でも動くように scheme を取り除く
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")

	// insecure フラグ(デフォルト true)
	insecure := true
	if v := os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"); v != "" {
		insecure = strings.EqualFold(v, "true")
	}

	// Exporter生成時はタイムタウト付きコンテキストを使う
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exp, err := otlptracegrpc.New(dialCtx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	// Resource: service.* を明示的に設定
	res, err := resource.New(
		ctx,
		resource.WithFromEnv(),
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("service.namespace", "grpc"),
			attribute.String("service.version", serviceVersion()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(
			sdktrace.NewBatchSpanProcessor(exp),
		),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// Shutdown 関数を返す(Exporterの終了も含まれる)
	return func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}, nil
}

// serviceVersion 環境変数からバージョンを取得し、なければ "dev" を返す。
func serviceVersion() string {
	if v := os.Getenv("CNO_APP_VERSION"); v != "" {
		return v
	}
	return "dev"
}
