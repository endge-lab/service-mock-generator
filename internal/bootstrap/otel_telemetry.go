package bootstrap

import (
	"context"
	"time"

	servicetelemetry "github.com/endge-lab/service-kit-go/pkg/telemetry"
	"github.com/endge-lab/service-template-go/internal/config"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	traceSampleModeAlways = "always"
	traceSampleModeNever  = "never"
)

func newTelemetryProviders(lc fx.Lifecycle, cfg *config.Config, logger *zap.Logger) (*servicetelemetry.Providers, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	endpoint := ""
	traceSampleMode := traceSampleModeNever
	if cfg.Telemetry.Enabled {
		endpoint = cfg.Telemetry.OTLPEndpoint
		traceSampleMode = traceSampleModeAlways
	}

	providers, err := servicetelemetry.NewProviders(ctx, servicetelemetry.Config{
		ServiceName:     cfg.App.Name,
		ServiceVersion:  cfg.App.Version,
		Environment:     cfg.App.Env,
		OTLPEndpoint:    endpoint,
		OTLPInsecure:    cfg.Telemetry.OTLPInsecure,
		MetricsInterval: 15 * time.Second,
		TraceSampleMode: traceSampleMode,
	}, logger)
	if err != nil {
		logger.Warn("telemetry exporter disabled", zap.Error(err), zap.String("endpoint", endpoint))
		providers, err = servicetelemetry.NewProviders(ctx, servicetelemetry.Config{
			ServiceName:     cfg.App.Name,
			ServiceVersion:  cfg.App.Version,
			Environment:     cfg.App.Env,
			TraceSampleMode: traceSampleModeNever,
		}, logger)
		if err != nil {
			return nil, err
		}
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("shutting down telemetry providers")
			return providers.Shutdown(ctx)
		},
	})

	return providers, nil
}

func newTracer(cfg *config.Config, providers *servicetelemetry.Providers) trace.Tracer {
	return providers.Tracer(cfg.App.Name)
}

func newMeter(cfg *config.Config, providers *servicetelemetry.Providers) metric.Meter {
	return providers.Meter(cfg.App.Name)
}
