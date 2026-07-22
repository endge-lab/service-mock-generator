package bootstrap

import (
	"context"

	"github.com/endge-lab/service-template-go/internal/config"
	"github.com/endge-lab/service-template-go/internal/platform"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

func InitLogger(lc fx.Lifecycle, cfg *config.Config) *zap.Logger {
	logger := platform.NewLogger(cfg.Logger.Level, cfg.App.Name, cfg.App.Env, cfg.App.Version)
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			_ = logger.Sync()
			return nil
		},
	})

	return logger
}
