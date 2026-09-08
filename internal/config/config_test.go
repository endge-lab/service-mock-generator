package config

import (
	kit "github.com/endge-lab/service-kit-go/config"
	"testing"
)

func TestIdentityIsRequiredOutsideExplicitDevelopment(t *testing.T) {
	for _, env := range []string{"production", "staging", "development"} {
		for _, allow := range []bool{false, true} {
			c := &kit.ServiceConfig{}
			c.App.Env = env
			e := validateIdentity(c, allow)
			if (e == nil) != (env == "development" && allow) {
				t.Fatal(env, allow, e)
			}
		}
	}
	c := &kit.ServiceConfig{}
	c.App.Env = "production"
	c.Identity.Verifier.Enabled = true
	c.GRPC.Enabled = true
	if validateIdentity(c, false) == nil {
		t.Fatal("production accepted without TLS")
	}
	c.TLS.Enabled = true
	if e := validateIdentity(c, false); e != nil {
		t.Fatal(e)
	}
}

func TestRejectInvalidConfiguredLimits(t *testing.T) {
	for name, value := range map[string]string{"MOCK_DEPTH": "129", "MOCK_JOBS": "0", "MOCK_GENERATION_TIMEOUT": "0s", "MOCK_ITEMS_PER_MESSAGE": "1001", "MOCK_SESSIONS_PER_OWNER": "33", "MOCK_BUFFER_BYTES": "1"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if _, e := LoadLimits(); e == nil {
				t.Fatal("invalid limit accepted")
			}
		})
	}
}
