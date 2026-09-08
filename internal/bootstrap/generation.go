package bootstrap

import (
	"context"
	grpcv1 "github.com/endge-lab/service-mock-generator/internal/api/grpc/v1"
	"github.com/endge-lab/service-mock-generator/internal/config"
	"github.com/endge-lab/service-mock-generator/internal/platform/schema"
	"github.com/endge-lab/service-mock-generator/internal/usecase/generate"
	"github.com/endge-lab/service-mock-generator/internal/usecase/stream"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/fx"
)

func newGenerator(cfg *config.Config) *generate.UseCase {
	return generate.NewUseCase(schema.NewCompiler(), cfg.Limits)
}
func newStreams(lc fx.Lifecycle, g *generate.UseCase, cfg *config.Config, meter metric.Meter) (*stream.UseCase, error) {
	s := stream.NewUseCase(g, cfg.Limits)
	active, e := meter.Int64ObservableGauge("mock_stream_sessions_active")
	if e != nil {
		return nil, e
	}
	jobs, e := meter.Int64ObservableGauge("mock_generation_jobs_active")
	if e != nil {
		return nil, e
	}
	bytes, e := meter.Int64ObservableGauge("mock_stream_buffer_bytes")
	if e != nil {
		return nil, e
	}
	batches, e := meter.Int64ObservableCounter("mock_stream_batches_total")
	if e != nil {
		return nil, e
	}
	expired, e := meter.Int64ObservableCounter("mock_stream_expired_total")
	if e != nil {
		return nil, e
	}
	errors, e := meter.Int64ObservableCounter("mock_stream_errors_total")
	if e != nil {
		return nil, e
	}
	backpressure, e := meter.Int64ObservableCounter("mock_stream_backpressure_total")
	if e != nil {
		return nil, e
	}
	duration, e := meter.Int64ObservableCounter("mock_generation_duration_nanoseconds_total")
	if e != nil {
		return nil, e
	}
	size, e := meter.Int64ObservableCounter("mock_generation_bytes_total")
	if e != nil {
		return nil, e
	}
	failures, e := meter.Int64ObservableCounter("mock_generation_failures_total")
	if e != nil {
		return nil, e
	}
	calls, e := meter.Int64ObservableCounter("mock_generation_batches_total")
	if e != nil {
		return nil, e
	}
	registration, e := meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		x := s.Stats()
		gen := g.Stats()
		o.ObserveInt64(duration, gen.DurationNanos)
		o.ObserveInt64(size, gen.ResultBytes)
		o.ObserveInt64(failures, gen.Failures)
		o.ObserveInt64(calls, gen.Calls)
		o.ObserveInt64(active, int64(x.Sessions))
		o.ObserveInt64(jobs, x.Jobs)
		o.ObserveInt64(bytes, x.BufferedBytes)
		o.ObserveInt64(batches, x.Batches)
		o.ObserveInt64(expired, x.Expired)
		o.ObserveInt64(errors, x.Errors)
		o.ObserveInt64(backpressure, x.Backpressure)
		return nil
	}, active, jobs, bytes, batches, expired, errors, backpressure, duration, size, failures, calls)
	if e != nil {
		return nil, e
	}
	lc.Append(fx.Hook{OnStart: func(context.Context) error { s.Start(); return nil }, OnStop: func(context.Context) error { s.Close(); return registration.Unregister() }})
	return s, nil
}
func newGRPCHandler(g *generate.UseCase, s *stream.UseCase, cfg *config.Config) *grpcv1.Server {
	return grpcv1.NewServer(g, s, cfg.Limits, cfg.App.Version, cfg.App.Env)
}
