package observability

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// NewLoggerはサーバー/クライアント共通で利用するJSON形式のzapロガーを返す。
// 戻り値はSugaredLoggerにしておき、呼び出し側はInfow/Errorwなどで利用する想定
func NewLogger() *zap.SugaredLogger {
	cfg := zap.NewProductionConfig()
	cfg.Encoding = "json"
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.MessageKey = "msg"
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.CallerKey = "caller"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	base, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return base.Sugar()
}

// UnaryLoggingInterceptorはgRPC Unary RPCのログを出力するインターセプター
// 以下のフィールドをJSONログとして出力する
//
//   - grpc_method : フルメソッド名(/package/Service/Method)
//   - trace_id : TODO(Otel導入時に span context から取得)
//   - request_id : metadata x-request-id から取得。なければ生成してcontextに埋め込む
//   - mode : TODO(metadata や RPCリクエストから取得予定、今は空文字)
//   - bytes_in : リクエストメッセージのバイトサイズ
//   - bytes_out : レスポンスメッセージのバイトサイズ
//   - latency_ms : 処理時間(ミリ秒)
//   - code : gRPCのステータスコード(OK,INVALID_ARGUMENT,...)
//   - error : エラー時のみ出力
func UnaryLoggingInterceptor(logger *zap.SugaredLogger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// request_idの取得/生成
		md, _ := metadata.FromIncomingContext(ctx)
		requestID := ""
		if md != nil {
			if vals := md.Get("x-request-id"); len(vals) > 0 && vals[0] != "" {
				requestID = vals[0]
			}
		}
		if requestID == "" {
			requestID = uuid.New().String()
			if md == nil {
				md = metadata.New(nil)
			}
			md.Set("x-request-id", requestID)
			ctx = metadata.NewIncomingContext(ctx, md)
		}
		// TODO: modeはmetadataやリクエストメッセージから取得する(後続ブランチ)
		mode := ""

		// bytes_in:リクエストサイズ
		bytesIn := 0
		if msg, ok := req.(proto.Message); ok {
			if b, err := proto.Marshal(msg); err == nil {
				bytesIn = len(b)
			}
		}

		// 実処理
		resp, err := handler(ctx, req)

		// bytes_out: レスポンスサイズ
		bytesOut := 0
		if msg, ok := resp.(proto.Message); ok {
			if b, err := proto.Marshal(msg); err == nil {
				bytesOut = len(b)
			}
		}

		latencyMs := time.Since(start).Milliseconds()

		st, _ := status.FromError(err)
		code := "OK"
		if st != nil {
			code = st.Code().String()
		}

		// TODO: trace_idはOtel導入時にspan contextから取得
		traceID := ""

		fields := []any{
			"grpc_method", info.FullMethod,
			"trace_id", traceID,
			"request_id", requestID,
			"mode", mode,
			"bytes_in", bytesIn,
			"bytes_out", bytesOut,
			"latency_ms", latencyMs,
			"code", code,
		}

		if err != nil {
			fields = append(fields, "error", err)
			logger.Errorw("grpc server unary", fields...)
		} else {
			logger.Infow("grpc server unary", fields...)
		}
		return resp, err
	}
}
