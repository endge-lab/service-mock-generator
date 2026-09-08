package http

import (
	docs "github.com/endge-lab/service-mock-generator/internal/api/http/v1/docs"
	health "github.com/endge-lab/service-mock-generator/internal/api/http/v1/health"
	transport "github.com/endge-lab/service-mock-generator/internal/api/http/v1/transport"
	"github.com/endge-lab/service-mock-generator/internal/config"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

func SetupRoutes(
	app *fiber.App,
	cfg *config.Config,
	meter metric.Meter,
	logger *zap.Logger,
) {
	setupMiddlewares(app, cfg, meter, logger)

	if !cfg.App.IsProduction() {
		docs.RegisterRoutes(app)
	}

	health.RegisterRoutes(app, health.Config{
		Service: cfg.App.Name,
		Version: cfg.App.Version,
		Env:     cfg.App.Env,
	})

	app.Use(func(c *fiber.Ctx) error {
		return transport.WriteErrorResponse(c, transport.ErrRouteNotFound)
	})
}
