package config

import (
	"fmt"
	kitconfig "github.com/endge-lab/service-kit-go/config"
	"github.com/endge-lab/service-mock-generator/internal/buildinfo"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"os"
	"strconv"
	"time"
)

type Config struct {
	*kitconfig.ServiceConfig
	Limits v.Limits
}

func Load() (*Config, error) {
	base, err := kitconfig.LoadServiceConfig()
	if err != nil {
		return nil, err
	}
	base.App.Name = "service_mock_generator"
	base.App.Version = buildinfo.Resolve()
	l, err := LoadLimits()
	if err != nil {
		return nil, err
	}
	if err := validateIdentity(base, os.Getenv("MOCK_ALLOW_INSECURE_DEVELOPMENT") == "true"); err != nil {
		return nil, err
	}
	return &Config{ServiceConfig: base, Limits: l}, nil
}
func validateIdentity(base *kitconfig.ServiceConfig, allowDevelopment bool) error {
	if !base.Identity.Verifier.Enabled && (base.App.GetEnv() != "development" || !allowDevelopment) {
		return fmt.Errorf("service identity verifier is required; development bypass must be explicit")
	}
	if base.App.IsProduction() && (!base.Identity.Verifier.Enabled || !base.TLS.Enabled || !base.GRPC.Enabled) {
		return fmt.Errorf("production requires gRPC, OIDC service identity and TLS")
	}
	return nil
}

func LoadLimits() (v.Limits, error) {
	l := v.DefaultLimits()
	ints := map[string]*int{"REQUEST_BYTES": &l.RequestBytes, "RESULT_BYTES": &l.ResultBytes, "SCHEMA_NODES": &l.Nodes, "DEPTH": &l.Depth, "DEFINITIONS": &l.Definitions, "ARRAY_LENGTH": &l.ArrayLength, "STRING_LENGTH": &l.StringLength, "RELATIONS": &l.Relations, "MAPPINGS": &l.Mappings, "ATTEMPTS": &l.Attempts, "COUNT": &l.Count, "ITEMS_PER_MESSAGE": &l.ItemsPerMessage, "MIN_INTERVAL_MS": &l.MinIntervalMs, "MAX_INTERVAL_MS": &l.MaxIntervalMs, "SESSIONS": &l.Sessions, "SESSIONS_PER_OWNER": &l.SessionsPerOwner, "JOBS": &l.Jobs, "BUFFER_BYTES": &l.BufferBytes}
	for name, ptr := range ints {
		if s := os.Getenv("MOCK_" + name); s != "" {
			n, e := strconv.Atoi(s)
			if e != nil || n < 1 || n > 1<<30 {
				return l, fmt.Errorf("invalid MOCK_%s", name)
			}
			*ptr = n
		}
	}
	for name, ptr := range map[string]*time.Duration{"GENERATION_TIMEOUT": &l.GenerationTimeout, "IDLE_TIMEOUT": &l.IdleTimeout, "READY_TIMEOUT": &l.ReadyTimeout, "WRITE_TIMEOUT": &l.WriteTimeout} {
		if s := os.Getenv("MOCK_" + name); s != "" {
			d, e := time.ParseDuration(s)
			if e != nil || d <= 0 || d > 24*time.Hour {
				return l, fmt.Errorf("invalid MOCK_%s", name)
			}
			*ptr = d
		}
	}
	if l.MinIntervalMs > l.MaxIntervalMs || l.ItemsPerMessage > l.Count || l.SessionsPerOwner > l.Sessions || l.Depth > 128 || l.Nodes > 100000 || l.Attempts > 1000 || l.BufferBytes < l.ResultBytes {
		return l, fmt.Errorf("inconsistent Mock resource limits")
	}
	return l, nil
}
