package server

import (
	// "context"

	grpcburnerv1 "github.com/shtsukada/cloudnative-observability-proto/gen/go/observability/grpcburner/v1"
)

// GrpcBurnerServer は grpcburnerv1.GrpcBurnerServerを実装する
type GrpcBurnerServer struct {
	grpcburnerv1.UnimplementedBurnerServer
}

// NewGrpcBurnerServer はアプリの gRPCサーバー実装を返す
// 後続ブランチで logger や設定を差し込む際に拡張しやすいよう、コンストラクタ関数とする
func NewGrpcBurnerServer() *GrpcBurnerServer {
	return &GrpcBurnerServer{}
}

// DoWork は proto側で定義する予定の Unary RPC の仮実装
// NOTE: まだ protoに DoWorkRequest / DoWorkResponseが存在しないため、実装はcloudnative-observability-protoを整備するブランチで追加する
// func (s *GrpcBurnerServer) DoWork(
// 	ctx context.Context,
// 	req *grpcburnerv1.DoWorkRequest,
// ) (*grpcburnerv1.DoWorkResponse, error) {
//     // TODO(cno-app): cloudnative-observability-proto で DoWork* を定義したあとに実装する。
//     //   - 負荷モード / エラーレート / レイテンシ などを proto のフィールドから受け取り、
//     //     実際の負荷生成ロジックにつなげる。
// 	return &grpcburnerv1.DoWorkResponse{}, nil
// }
