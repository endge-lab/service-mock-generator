package http

import (
	"strings"

	servicefiber "github.com/endge-lab/service-kit-go/pkg/httpkit/fiber"
	"github.com/endge-lab/service-template-go/internal/config"

	otelfiber "github.com/gofiber/contrib/otelfiber/v2"
	"github.com/gofiber/fiber/v2"
	fibercors "github.com/gofiber/fiber/v2/middleware/cors"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

func setupMiddlewares(app *fiber.App, cfg *config.Config, meter metric.Meter, logger *zap.Logger) {
	app.Use(fibercors.New(fibercors.Config{
		AllowCredentials: true,
		AllowMethods:     "GET,POST,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Requested-With, traceparent, tracestate, baggage",
		AllowOriginsFunc: func(origin string) bool {
			return isOriginAllowed(origin, cfg.HTTP.CORSAllowedOrigins)
		},
	}))
	app.Use(otelfiber.Middleware(otelfiber.WithSpanNameFormatter(func(ctx *fiber.Ctx) string {
		return ctx.Method() + " " + routePattern(ctx)
	})))
	app.Use(servicefiber.RequestLoggerMiddleware(logger.With(zap.String("component", "http"))))
	app.Use(mustRequestMetricsMiddleware(meter, logger))
}

func mustRequestMetricsMiddleware(meter metric.Meter, logger *zap.Logger) fiber.Handler {
	handler, err := servicefiber.NewRequestMetricsMiddleware(meter)
	if err != nil {
		logger.Error("failed to create request metrics middleware", zap.Error(err))
		return func(c *fiber.Ctx) error {
			return c.Next()
		}
	}

	return handler
}

func routePattern(c *fiber.Ctx) string {
	if route := c.Route(); route != nil && strings.TrimSpace(route.Path) != "" {
		return route.Path
	}

	return c.Path()
}

func isOriginAllowed(origin string, allowList string) bool {
	normalizedOrigin := strings.TrimSpace(origin)
	if normalizedOrigin == "" {
		return true
	}

	for _, item := range strings.Split(allowList, ",") {
		pattern := strings.TrimSpace(item)
		if pattern == "" {
			continue
		}
		if !strings.Contains(pattern, "*") && strings.EqualFold(pattern, normalizedOrigin) {
			return true
		}
		if strings.HasPrefix(pattern, "https://*.") {
			suffix := strings.TrimPrefix(pattern, "https://*")
			if strings.HasPrefix(normalizedOrigin, "https://") && strings.HasSuffix(normalizedOrigin, suffix) {
				return true
			}
		}
		if strings.HasPrefix(pattern, "http://*.") {
			suffix := strings.TrimPrefix(pattern, "http://*")
			if strings.HasPrefix(normalizedOrigin, "http://") && strings.HasSuffix(normalizedOrigin, suffix) {
				return true
			}
		}
	}

	return false
}
