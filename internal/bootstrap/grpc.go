package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/endge-lab/service-kit-go/pkg/grpckit"
	serviceoidc "github.com/endge-lab/service-kit-go/pkg/oidc"
	mockdatav1 "github.com/endge-lab/service-mock-generator/api/mockdata/v1"
	grpcv1 "github.com/endge-lab/service-mock-generator/internal/api/grpc/v1"
	"github.com/endge-lab/service-mock-generator/internal/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func newGRPCServer(lifecycle fx.Lifecycle, cfg *config.Config, handler *grpcv1.Server, logger *zap.Logger, shutdown fx.Shutdowner) (*grpckit.Server, error) {
	if !cfg.GRPC.Enabled {
		if cfg.App.IsProduction() {
			return nil, fmt.Errorf("grpc server must be enabled in production")
		}
		return nil, nil
	}
	serverOptions := []grpc.ServerOption{grpc.ChainUnaryInterceptor(grpcv1.RecoverUnary(logger)), grpc.ChainStreamInterceptor(grpcv1.RecoverStream(logger))}
	verifierConfig := cfg.Identity.Verifier
	if verifierConfig.Enabled {
		verifier, err := serviceoidc.NewVerifier(serviceoidc.VerifierConfig{
			Issuer: verifierConfig.Issuer, JWKSURL: verifierConfig.JWKSURL, Audience: verifierConfig.Audience,
			AllowedCallers: verifierConfig.CallerList(), AllowedAlgorithms: verifierConfig.AlgorithmList(),
			CacheTTL: verifierConfig.JWKSCacheTTL, Timeout: verifierConfig.Timeout,
		})
		if err != nil {
			return nil, err
		}
		serverOptions = append(serverOptions,
			grpc.ChainUnaryInterceptor(grpckit.UnaryServerIdentityInterceptor(verifier)),
			grpc.ChainStreamInterceptor(grpckit.StreamServerIdentityInterceptor(verifier)),
		)
	} else if cfg.App.IsProduction() {
		return nil, fmt.Errorf("service identity verification must be enabled in production")
	}
	if cfg.App.IsProduction() && !cfg.TLS.Enabled {
		return nil, fmt.Errorf("grpc TLS must be enabled in production")
	}
	server, err := grpckit.NewServer(grpckit.ServerConfig{
		Address: fmt.Sprintf(":%d", cfg.GRPC.Port), MaxReceiveBytes: cfg.GRPC.MaxReceiveBytes,
		MaxSendBytes: cfg.GRPC.MaxSendBytes, GracefulStopTimeout: cfg.GRPC.GracefulStopTimeout,
		KeepaliveTime: cfg.GRPC.KeepaliveTime, KeepaliveTimeout: cfg.GRPC.KeepaliveTimeout,
		TLS: grpckit.TLSConfig{
			Enabled:  cfg.TLS.Enabled,
			CertFile: cfg.TLS.CertFile,
			KeyFile:  cfg.TLS.KeyFile,
		},
	}, serverOptions...)
	if err != nil {
		return nil, err
	}
	mockdatav1.RegisterMockDataServiceServer(server.GRPC(), handler)
	runContext, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				if err := server.ListenAndServe(runContext); err != nil {
					logger.Error("grpc server stopped", zap.Error(err))
					_ = shutdown.Shutdown(fx.ExitCode(1))
				}
			}()
			logger.Info("grpc server starting", zap.Int("port", cfg.GRPC.Port))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			cancel()
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(cfg.GRPC.GracefulStopTimeout + time.Second):
				return fmt.Errorf("grpc server shutdown timed out")
			}
		},
	})
	return server, nil
}
