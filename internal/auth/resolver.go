package auth

import (
	serviceauth "github.com/endge-lab/service-kit-go/pkg/auth"
	"github.com/endge-lab/service-template-go/internal/config"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var ErrUnauthorized = serviceauth.ErrUnauthorized

type Identity = serviceauth.Identity
type Resolver = serviceauth.Resolver

func NewResolver(cfg *config.Config, tracer trace.Tracer, log *zap.Logger) Resolver {
	return serviceauth.NewResolver(serviceauth.Config{
		JWKSURL:          cfg.Auth.JWKSURL(),
		Issuer:           cfg.Auth.Issuer,
		AllowedAudiences: splitCSV(cfg.Auth.AllowedAudiences),
		CacheTTL:         cfg.Auth.JWKSCacheTTL,
		Timeout:          cfg.Auth.Timeout,
	}, tracer, log)
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}

	return serviceauthIdentityList(value)
}

func serviceauthIdentityList(value string) []string {
	values := make([]string, 0)
	current := ""
	for _, char := range value {
		if char == ',' {
			if current != "" {
				values = append(values, current)
			}
			current = ""
			continue
		}
		if char != ' ' && char != '\t' && char != '\n' {
			current += string(char)
		}
	}
	if current != "" {
		values = append(values, current)
	}
	return values
}
