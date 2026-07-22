package errors

// Common transport categories used across the service.
//
// Specific business errors should wrap one of these sentinels so callers can
// match them with errors.Is while still getting a stable machine-readable code.
var (
	ErrUnauthorized = New("common.unauthorized", "Требуется авторизация", 401)
	ErrForbidden    = New("common.forbidden", "Недостаточно прав", 403)
	ErrInvalidInput = New("common.invalid_input", "Некорректный запрос", 400)
	ErrNotFound     = New("common.not_found", "Сущность не найдена", 404)
	ErrConflict     = New("common.conflict", "Конфликт состояния", 409)
	ErrInternal     = New("common.internal", "Внутренняя ошибка сервиса", 500)
)
