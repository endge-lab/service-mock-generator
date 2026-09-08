package grpcv1_test

import (
	"context"
	"encoding/json"
	"github.com/endge-lab/service-kit-go/pkg/oidc"
	"github.com/endge-lab/service-kit-go/pkg/telemetry"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"github.com/endge-lab/service-mock-generator/internal/platform/schema"
	"github.com/endge-lab/service-mock-generator/internal/usecase/generate"
	metrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	traces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientCredentialsRefreshOutageAndAudienceIsolation(t *testing.T) {
	var offline atomic.Bool
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if offline.Load() {
			w.WriteHeader(503)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": r.Form.Get("audience"), "expires_in": 1})
	}))
	defer server.Close()
	makeProvider := func(audience string) oidc.TokenProvider {
		p, e := oidc.NewClientCredentialsProvider(oidc.ClientCredentialsConfig{TokenURL: server.URL, ClientID: "test", ClientSecret: "synthetic", Audience: audience, Timeout: 100 * time.Millisecond})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	mock, workbench := makeProvider("endge-mock-generator"), makeProvider("endge-ai-workbench")
	for _, p := range []oidc.TokenProvider{mock, workbench, mock} {
		if _, e := p.Token(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("short lived token was not refreshed")
	}
	offline.Store(true)
	if _, e := mock.Token(context.Background()); e == nil {
		t.Fatal("refresh failure accepted")
	}
	offline.Store(false)
	for audience, p := range map[string]oidc.TokenProvider{"endge-mock-generator": mock, "endge-ai-workbench": workbench} {
		if token, e := p.Token(context.Background()); e != nil || token != audience {
			t.Fatal("provider audience cross-talk", e)
		}
	}
}

type failingMetrics struct {
	metrics.UnimplementedMetricsServiceServer
	calls   atomic.Int64
	offline atomic.Bool
}

func (s *failingMetrics) Export(context.Context, *metrics.ExportMetricsServiceRequest) (*metrics.ExportMetricsServiceResponse, error) {
	s.calls.Add(1)
	if s.offline.Load() {
		return nil, status.Error(codes.Unavailable, "synthetic outage")
	}
	return &metrics.ExportMetricsServiceResponse{}, nil
}

type acceptingTraces struct {
	traces.UnimplementedTraceServiceServer
}

func (*acceptingTraces) Export(context.Context, *traces.ExportTraceServiceRequest) (*traces.ExportTraceServiceResponse, error) {
	return &traces.ExportTraceServiceResponse{}, nil
}

func TestTelemetryOutageDoesNotBlockGeneration(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	server := grpc.NewServer()
	collector := &failingMetrics{}
	collector.offline.Store(true)
	metrics.RegisterMetricsServiceServer(server, collector)
	traces.RegisterTraceServiceServer(server, &acceptingTraces{})
	go server.Serve(listener)
	defer server.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	providers, e := telemetry.NewProviders(ctx, telemetry.Config{ServiceName: "mock-outage-test", OTLPEndpoint: listener.Addr().String(), OTLPInsecure: true, MetricsInterval: 10 * time.Millisecond}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = providers.Shutdown(ctx)
	}()
	counter, e := providers.Meter("test").Int64Counter("requests")
	if e != nil {
		t.Fatal(e)
	}
	counter.Add(ctx, 1)
	deadline := time.Now().Add(2 * time.Second)
	for collector.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if collector.calls.Load() == 0 {
		t.Fatal("outage was not exercised")
	}
	g := generate.NewUseCase(schema.NewCompiler(), v.DefaultLimits())
	count, seed := 10, "outage"
	for range 100 {
		_, e = g.Generate(ctx, v.Request{Schema: json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"integer"}`), Generation: v.Options{Count: &count, Seed: &seed}})
		if e != nil {
			t.Fatal(e)
		}
	}
	collector.offline.Store(false)
	if g.ActiveJobs() != 0 {
		t.Fatal("generation jobs retained")
	}
}
