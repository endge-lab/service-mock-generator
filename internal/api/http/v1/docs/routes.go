package docs

import (
	contractdocs "github.com/endge-lab/service-template-go/docs"

	"github.com/gofiber/fiber/v2"
)

func RegisterRoutes(app *fiber.App) {
	app.Get("/swagger/openapi3.yaml", handleOpenAPISpec)
	app.Get("/swagger", handleSwaggerUI)
}

func handleOpenAPISpec(c *fiber.Ctx) error {
	c.Set("Content-Type", "application/yaml; charset=utf-8")
	return c.Send(contractdocs.OpenAPI3YAML())
}

func handleSwaggerUI(c *fiber.Ctx) error {
	return c.Type("html").SendString(`<!doctype html>
<html lang="ru">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Endge Service Template Scalar</title>
    <style>
      body { margin: 0; }
    </style>
  </head>
  <body>
    <script
      id="api-reference"
      data-url="/swagger/openapi3.yaml"
      data-configuration='{"theme":"blue","layout":"modern","showSidebar":true,"persistAuth":true,"defaultOpenAllTags":false}'></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.28.5"></script>
  </body>
</html>`)
}
