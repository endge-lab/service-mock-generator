package platform

import (
	servicelogging "github.com/endge-lab/service-kit-go/pkg/logging"

	"go.uber.org/zap"
)

func NewLogger(logLevel string, serviceName string, appEnv string, appVersion string) *zap.Logger {
	logger, err := servicelogging.NewLogger(servicelogging.Config{
		Level:       logLevel,
		ServiceName: serviceName,
		Environment: appEnv,
		Version:     appVersion,
	})
	if err == nil {
		return logger
	}

	logger, _ = servicelogging.NewLogger(servicelogging.Config{
		ServiceName: serviceName,
		Environment: appEnv,
		Version:     appVersion,
	})
	return logger
}
