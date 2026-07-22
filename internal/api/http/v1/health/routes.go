package http

import (
	transport "github.com/endge-lab/service-template-go/internal/api/http/v1/transport"

	"github.com/gofiber/fiber/v2"
)

type Config struct {
	Service string
	Version string
	Env     string
}

func RegisterRoutes(app *fiber.App, cfg Config) {
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(transport.HealthResponse{
			Status:  "ok",
			Service: cfg.Service,
			Version: cfg.Version,
			Env:     cfg.Env,
		})
	})
	app.Get("/version", func(c *fiber.Ctx) error {
		return c.JSON(transport.VersionResponse{
			Service: cfg.Service,
			Version: cfg.Version,
			Env:     cfg.Env,
		})
	})
}
