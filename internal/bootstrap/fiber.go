package bootstrap

import (
	"fmt"

	servicefiber "github.com/endge-lab/service-kit-go/pkg/httpkit/fiber"
	"github.com/endge-lab/service-template-go/internal/config"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func NewFiber(
	lc fx.Lifecycle,
	cfg *config.Config,
	logger *zap.Logger,
) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:   cfg.App.Name,
		BodyLimit: 16 * 1024 * 1024,
	})

	servicefiber.RegisterLifecycle(lc, app, fmt.Sprintf(":%s", cfg.HTTP.Port), logger)

	return app
}
