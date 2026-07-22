package bootstrap

import (
	v1 "github.com/endge-lab/service-template-go/internal/api/http/v1"

	"go.uber.org/fx"
)

func InvokeModules() fx.Option {
	return fx.Options(
		fx.Invoke(
			v1.SetupRoutes,
		),
	)
}
