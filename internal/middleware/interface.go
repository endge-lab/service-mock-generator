package middleware

import (
	"context"

	"github.com/gofiber/fiber/v2"
)

type AuthMiddleware interface {
	AuthMiddleware() fiber.Handler
	Authenticate(c *fiber.Ctx, allowQueryToken bool) error
}

func IdentityFromContext(ctx context.Context) (RequestIdentity, bool) {
	identity, ok := ctx.Value(identityKey).(RequestIdentity)
	return identity, ok
}