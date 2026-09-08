package bootstrap

import (
	"github.com/endge-lab/service-kit-go/pkg/grpckit"
	v1 "github.com/endge-lab/service-mock-generator/internal/api/http/v1"

	"go.uber.org/fx"
)

func InvokeModules() fx.Option {
	return fx.Options(
		fx.Invoke(
			v1.SetupRoutes,
			func(_ *grpckit.Server) {},
		),
	)
}
