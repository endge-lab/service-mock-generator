package bootstrap

import (
	"github.com/endge-lab/service-template-go/internal/auth"
	"github.com/endge-lab/service-template-go/internal/middleware"

	"go.uber.org/fx"
)

func HandlerModules() fx.Option {
	return fx.Options(
		fx.Provide(
			auth.NewResolver,
			fx.Annotate(middleware.NewAuthMiddleware, fx.As(new(middleware.AuthMiddleware))),
		),
	)
}
