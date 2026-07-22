package errors

import (
	"errors"
	"fmt"
)

type Code string

// AppError is the canonical machine-readable transport contract for the service.
//
// Rules:
//   - return sentinels like ErrInvalidInput / ErrNotFound for common cases;
//   - create service-specific errors by wrapping those sentinels with InvalidInput,
//     NotFound, Conflict and similar constructors;
//   - HTTP, logs and traces should read code/status/message from this type instead
//     of hardcoding their own mappings.
type AppError struct {
	code       Code
	message    string
	httpStatus int
	details    map[string]any
	cause      error
}

func New(code Code, message string, httpStatus int) *AppError {
	return &AppError{
		code:       code,
		message:    message,
		httpStatus: httpStatus,
	}
}

func Wrap(cause error, code Code, message string, httpStatus int) *AppError {
	return &AppError{
		code:       code,
		message:    message,
		httpStatus: httpStatus,
		cause:      cause,
	}
}

func InvalidInput(code Code, message string) *AppError {
	return Wrap(ErrInvalidInput, code, message, 400)
}

func Unauthorized(code Code, message string) *AppError {
	return Wrap(ErrUnauthorized, code, message, 401)
}

func Forbidden(code Code, message string) *AppError {
	return Wrap(ErrForbidden, code, message, 403)
}

func NotFound(code Code, message string) *AppError {
	return Wrap(ErrNotFound, code, message, 404)
}

func Conflict(code Code, message string) *AppError {
	return Wrap(ErrConflict, code, message, 409)
}

func Internal(code Code, message string) *AppError {
	return Wrap(ErrInternal, code, message, 500)
}

func WithDetails(err error, details map[string]any) error {
	if len(details) == 0 {
		return err
	}

	var appErr *AppError
	if errors.As(err, &appErr) {
		cloned := *appErr
		cloned.details = cloneDetails(details)
		return &cloned
	}

	internalErr := Internal("common.internal", ErrInternal.SafeMessage())
	internalErr.cause = err
	internalErr.details = cloneDetails(details)
	return internalErr
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.code, e.cause)
	}
	return string(e.code)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *AppError) Code() string {
	if e == nil {
		return ""
	}
	return string(e.code)
}

func (e *AppError) SafeMessage() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *AppError) HTTPStatus() int {
	if e == nil {
		return 0
	}
	return e.httpStatus
}

func (e *AppError) Details() map[string]any {
	if e == nil {
		return nil
	}
	return cloneDetails(e.details)
}

func (e *AppError) Is(target error) bool {
	other, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.code != "" && e.code == other.code
}

func CodeOf(err error) string {
	var appErr interface{ Code() string }
	if errors.As(err, &appErr) {
		return appErr.Code()
	}
	return ErrInternal.Code()
}

func SafeMessageOf(err error) string {
	var appErr interface{ SafeMessage() string }
	if errors.As(err, &appErr) {
		return appErr.SafeMessage()
	}
	return ErrInternal.SafeMessage()
}

func HTTPStatusOf(err error) int {
	var appErr interface{ HTTPStatus() int }
	if errors.As(err, &appErr) && appErr.HTTPStatus() > 0 {
		return appErr.HTTPStatus()
	}
	return ErrInternal.HTTPStatus()
}

func DetailsOf(err error) map[string]any {
	var appErr interface{ Details() map[string]any }
	if errors.As(err, &appErr) {
		return appErr.Details()
	}
	return nil
}

func cloneDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}

	cloned := make(map[string]any, len(details))
	for key, value := range details {
		cloned[key] = value
	}

	return cloned
}
