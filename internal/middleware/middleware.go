package middleware

import (
	"context"
	"strings"

	kitfiberauth "github.com/endge-lab/service-kit-go/pkg/auth/fiber"
	"github.com/endge-lab/service-template-go/internal/auth"
	domainerrors "github.com/endge-lab/service-template-go/internal/domain/errors"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type contextKey string

const (
	userIDKey    contextKey = "user_id"
	sessionIDKey contextKey = "session_id"
	identityKey  contextKey = "identity"
)

type RequestIdentity struct {
	AuthUserID  string
	Username    string
	DisplayName string
	Role        string
	SessionID   string
	App         string
	Platform    string
	Scope       []string
	ExpiresAt   string
}

type middleware struct {
	delegate *kitfiberauth.Middleware
}

type errorResponse struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func NewAuthMiddleware(authResolver auth.Resolver, log *zap.Logger) AuthMiddleware {
	return &middleware{
		delegate: kitfiberauth.NewMiddleware(authResolver, log),
	}
}

func (m *middleware) AuthMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := m.Authenticate(c, false); err != nil {
			return err
		}
		return c.Next()
	}
}

func (m *middleware) Authenticate(c *fiber.Ctx, allowQueryToken bool) error {
	if err := m.delegate.AuthenticateRequest(c, allowQueryToken); err != nil {
		return respondJSONError(c, err)
	}

	identity, ok := kitfiberauth.GetIdentity(c.UserContext())
	if !ok || strings.TrimSpace(identity.AuthUserID) == "" {
		return respondJSONError(c, domainerrors.Unauthorized("auth.identity_missing", "В токене отсутствует идентификатор пользователя"))
	}

	userID := strings.TrimSpace(identity.AuthUserID)
	ctx := context.WithValue(c.UserContext(), userIDKey, userID)
	ctx = context.WithValue(ctx, identityKey, RequestIdentity{
		AuthUserID:  userID,
		Username:    strings.TrimSpace(identity.Username),
		DisplayName: strings.TrimSpace(identity.DisplayName),
		Role:        strings.TrimSpace(identity.Role),
		SessionID:   strings.TrimSpace(identity.SessionID),
		App:         strings.TrimSpace(identity.App),
		Platform:    strings.TrimSpace(identity.Platform),
		Scope:       identity.Scope,
		ExpiresAt:   strings.TrimSpace(identity.ExpiresAt),
	})

	sessionID := strings.TrimSpace(identity.SessionID)
	if sessionID != "" {
		ctx = context.WithValue(ctx, sessionIDKey, sessionID)
		c.Locals(string(sessionIDKey), sessionID)
	}

	c.SetUserContext(ctx)
	c.Locals(string(userIDKey), userID)
	c.Locals(string(identityKey), identity)

	return nil
}

func GetUserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok
}

func GetSessionID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(sessionIDKey).(string)
	return id, ok
}

func respondJSONError(c *fiber.Ctx, err error) error {
	return c.Status(domainerrors.HTTPStatusOf(err)).JSON(errorResponse{
		Code:    domainerrors.CodeOf(err),
		Message: domainerrors.SafeMessageOf(err),
		Details: domainerrors.DetailsOf(err),
	})
}
