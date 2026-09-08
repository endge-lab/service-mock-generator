package grpcv1

import (
	"context"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Recovery contains unexpected failures at the transport boundary, without logging inputs.
func RecoverUnary(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (reply any, err error) {
		defer func() {
			if recover() != nil {
				logger.Error("unexpected unary panic", zap.String("method", info.FullMethod))
				reply = nil
				err = status.Error(codes.Internal, "Internal generator error")
			}
		}()
		return handler(ctx, req)
	}
}
func RecoverStream(logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if recover() != nil {
				logger.Error("unexpected streaming panic", zap.String("method", info.FullMethod))
				err = status.Error(codes.Internal, "Internal generator error")
			}
		}()
		return handler(srv, stream)
	}
}
