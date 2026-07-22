package bootstrap

import (
	"github.com/endge-lab/service-template-go/internal/config"
	"github.com/endge-lab/service-template-go/internal/platform"

	"go.uber.org/fx"
)

func CommonModules() fx.Option {
	return fx.Options(
		fx.Provide(
			config.Load,
			newPostgres,
			InitLogger,
			InitValidator,
			platform.NewRedpandaClient,
			NewFiber,
			newTelemetryProviders,
			newTracer,
			newMeter,
		),
	)
}
