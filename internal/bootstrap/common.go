package bootstrap

import (
	"github.com/endge-lab/service-mock-generator/internal/config"

	"go.uber.org/fx"
)

func CommonModules() fx.Option {
	return fx.Options(
		fx.Provide(
			newGenerator,
			newStreams,
			newGRPCHandler,
			newGRPCServer,
			config.Load,
			InitLogger,
			InitValidator,
			NewFiber,
			newTelemetryProviders,
			newTracer,
			newMeter,
		),
	)
}
