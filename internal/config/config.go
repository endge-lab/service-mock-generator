package config

import kitconfig "github.com/endge-lab/service-kit-go/config"

type BaseConfig = kitconfig.ServiceConfig
type Config = kitconfig.ServiceConfig

type AppConfig = kitconfig.ServiceAppConfig
type HTTPConfig = kitconfig.ServiceHTTPConfig
type LoggerConfig = kitconfig.ServiceLoggerConfig
type MetricsConfig = kitconfig.ServiceMetricsConfig
type PostgresConfig = kitconfig.ServicePostgresConfig
type AuthConfig = kitconfig.ServiceAuthConfig
type TelemetryConfig = kitconfig.ServiceTelemetryConfig
type RedpandaConfig = kitconfig.ServiceRedpandaConfig
type TLSConfig = kitconfig.ServiceTLSConfig

func Load() (*Config, error) {
	return kitconfig.LoadServiceConfig()
}
