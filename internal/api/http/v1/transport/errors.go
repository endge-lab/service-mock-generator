package http

import (
	domainerrors "github.com/endge-lab/service-template-go/internal/domain/errors"

	"github.com/gofiber/fiber/v2"
)

var (
	ErrRouteNotFound   = domainerrors.NotFound("http.route_not_found", "Маршрут не найден")
	ErrInvalidBody     = domainerrors.InvalidInput("http.invalid_body", "Некорректное тело запроса")
	ErrValidationError = domainerrors.InvalidInput("http.validation_failed", "Некорректные поля запроса")
	ErrInvalidToken    = domainerrors.Unauthorized("auth.invalid_access_token", "Access token недействителен или просрочен")
	ErrMissingToken    = domainerrors.Unauthorized("auth.access_token_required", "Требуется access token")
	ErrMissingIdentity = domainerrors.Unauthorized("auth.identity_missing", "В токене отсутствует идентификатор пользователя")
)

func WriteErrorResponse(c *fiber.Ctx, err error) error {
	return c.Status(domainerrors.HTTPStatusOf(err)).JSON(ErrorResponse{
		Code:    domainerrors.CodeOf(err),
		Message: domainerrors.SafeMessageOf(err),
		Details: domainerrors.DetailsOf(err),
	})
}
