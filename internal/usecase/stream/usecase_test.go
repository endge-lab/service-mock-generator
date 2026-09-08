package stream

import (
	"context"
	"encoding/json"
	"github.com/endge-lab/service-mock-generator/internal/domain/entities"
	errs "github.com/endge-lab/service-mock-generator/internal/domain/errors"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"github.com/endge-lab/service-mock-generator/internal/platform/schema"
	"github.com/endge-lab/service-mock-generator/internal/usecase/generate"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func ptr[T any](x T) *T { return &x }

var testOwner = v.Owner{ActorID: "a", WorkspaceID: "w"}

func request() v.StreamRequest {
	return v.StreamRequest{Schema: json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"integer"}`), Generation: v.Options{Seed: ptr("fixture")}, Stream: v.Parameters{IntervalMs: 100, ItemsPerMessage: 1}}
}
func setup(t *testing.T) (*UseCase, *atomic.Int64) {
	t.Helper()
	l := v.DefaultLimits()
	u := NewUseCase(generate.NewUseCase(schema.NewCompiler(), l), l)
	var clock atomic.Int64
	origin := time.Now()
	u.now = func() time.Time { return origin.Add(time.Duration(clock.Load())) }
	t.Cleanup(u.Close)
	return u, &clock
}
func TestReadyExpiryAndOwnership(t *testing.T) {
	u, clock := setup(t)
	s, e := u.Create(context.Background(), testOwner, request())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = u.Get(v.Owner{ActorID: "b", WorkspaceID: "w"}, s.ID); errs.HTTPStatusOf(e) != 404 {
		t.Fatal("foreign access")
	}
	if _, e = u.KeepAlive(testOwner, s.ID); errs.HTTPStatusOf(e) != 409 {
		t.Fatal("renewed ready session")
	}
	clock.Store(int64(30 * time.Second))
	if _, e = u.Get(testOwner, s.ID); errs.HTTPStatusOf(e) != 404 {
		t.Fatal("revived expired session")
	}
	if u.Stats().Sessions != 0 {
		t.Fatal("session leaked")
	}
}

func TestStartedUsesPreparedParametersVersion(t *testing.T) {
	u, _ := setup(t)
	s, e := u.Create(context.Background(), testOwner, request())
	if e != nil {
		t.Fatal(e)
	}
	updated, e := u.Update(context.Background(), testOwner, s.ID, v.Patch{ItemsPerMessage: ptr(2)})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = u.Subscribe(ctx, testOwner, s.ID, func(_ context.Context, event entities.Event) error {
		if event.Type == "started" {
			if event.ParametersVersion != updated.ParametersVersion {
				t.Errorf("acknowledgement version %d; want %d", event.ParametersVersion, updated.ParametersVersion)
			}
			cancel()
		}
		return nil
	})
}

func TestPausePreservesCursorAcrossBatchSizeChange(t *testing.T) {
	u, _ := setup(t)
	r := request()
	s, e := u.Create(context.Background(), testOwner, r)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan entities.Event, 20)
	done := make(chan error, 1)
	go func() {
		done <- u.Subscribe(ctx, testOwner, s.ID, func(ctx context.Context, e entities.Event) error {
			select {
			case events <- e:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	<-events
	first := <-events
	if _, e = u.Update(context.Background(), testOwner, s.ID, v.Patch{Paused: ptr(true)}); e != nil {
		t.Fatal(e)
	}
	before := u.generator.Stats().Calls
	time.Sleep(250 * time.Millisecond)
	if u.generator.Stats().Calls != before {
		t.Fatal("paused stream consumed generation work")
	}
	updated, e := u.Update(context.Background(), testOwner, s.ID, v.Patch{Paused: ptr(false), ItemsPerMessage: ptr(2)})
	if e != nil {
		t.Fatal(e)
	}
	items := append([]json.RawMessage{}, first.Items...)
	for {
		select {
		case event := <-events:
			items = append(items, event.Items...)
			if event.ParametersVersion == updated.ParametersVersion {
				if len(event.Items) != 2 {
					t.Fatal("wrong updated batch size")
				}
				goto received
			}
		case <-time.After(time.Second):
			t.Fatal("resume stalled")
		}
	}
received:
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup stalled")
	}
	count := len(items)
	expected, e := u.generator.Generate(context.Background(), v.Request{Schema: r.Schema, Generation: v.Options{Count: &count, Seed: r.Generation.Seed}})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(items, expected.Items) {
		t.Fatal("pause or batch size change altered deterministic sequence")
	}
}
func TestLeaseIgnoresDataAndCommands(t *testing.T) {
	u, clock := setup(t)
	s, e := u.Create(context.Background(), testOwner, request())
	if e != nil {
		t.Fatal(e)
	}
	events := make(chan entities.Event, 10)
	done := make(chan error, 1)
	go func() {
		done <- u.Subscribe(context.Background(), testOwner, s.ID, func(ctx context.Context, e entities.Event) error {
			select {
			case events <- e:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	<-events
	if e = u.Subscribe(context.Background(), testOwner, s.ID, func(context.Context, entities.Event) error { return nil }); errs.HTTPStatusOf(e) != 409 {
		t.Fatal("duplicate subscription accepted")
	}
	clock.Store(int64(150 * time.Second))
	if _, e = u.Update(context.Background(), testOwner, s.ID, v.Patch{IntervalMs: ptr(200), Paused: ptr(true)}); e != nil {
		t.Fatal(e)
	}
	clock.Store(int64(180 * time.Second))
	u.Reap()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expired stream did not stop")
	}
	if u.Stats().Sessions != 0 {
		t.Fatal("session leaked")
	}
}
func TestKeepAliveAndInvalidPatch(t *testing.T) {
	u, clock := setup(t)
	s, e := u.Create(context.Background(), testOwner, request())
	if e != nil {
		t.Fatal(e)
	}
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- u.Subscribe(context.Background(), testOwner, s.ID, func(ctx context.Context, e entities.Event) error {
			if e.Type == "started" {
				close(ready)
			}
			return nil
		})
	}()
	<-ready
	clock.Store(int64(150 * time.Second))
	info, e := u.KeepAlive(testOwner, s.ID)
	if e != nil {
		t.Fatal(e)
	}
	if info.ExpiresAt.Sub(u.now()) != 180*time.Second {
		t.Fatal("wrong lease")
	}
	if _, e = u.Update(context.Background(), testOwner, s.ID, v.Patch{ItemsPerMessage: ptr(0)}); e == nil {
		t.Fatal("invalid update accepted")
	}
	after, e := u.Get(testOwner, s.ID)
	if e != nil || after.ParametersVersion != 1 {
		t.Fatal("invalid patch mutated session")
	}
	clock.Store(int64(200 * time.Second))
	u.Reap()
	if _, e = u.Get(testOwner, s.ID); e != nil {
		t.Fatal("keepalive ineffective")
	}
	_ = u.Stop(testOwner, s.ID)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop leaked")
	}
}
func TestConcurrentSessionLimit(t *testing.T) {
	u, _ := setup(t)
	var wg sync.WaitGroup
	var accepted atomic.Int64
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := u.Create(context.Background(), testOwner, request()); e == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() > 5 || u.Stats().Sessions > 5 {
		t.Fatal("quota race")
	}
}
func TestDisconnectDuringWrite(t *testing.T) {
	u, _ := setup(t)
	s, e := u.Create(context.Background(), testOwner, request())
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- u.Subscribe(ctx, testOwner, s.ID, func(ctx context.Context, e entities.Event) error {
			if e.Type == "started" {
				close(ready)
				return nil
			}
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-ready
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("disconnect leaked")
	}
	if stats := u.Stats(); stats.Sessions != 0 || stats.BufferedBytes != 0 {
		t.Fatal(stats)
	}
}
func FuzzCommands(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4})
	f.Add([]byte{4, 4, 0})
	f.Fuzz(func(t *testing.T, commands []byte) {
		if len(commands) > 100 {
			return
		}
		u, clock := setup(t)
		s, e := u.Create(context.Background(), testOwner, request())
		if e != nil {
			t.Fatal(e)
		}

		var done chan error
		for _, c := range commands {
			switch c % 8 {
			case 0:
				_, _ = u.Get(testOwner, s.ID)
			case 1:
				_, _ = u.KeepAlive(testOwner, s.ID)
			case 2:
				_, _ = u.Update(context.Background(), testOwner, s.ID, v.Patch{IntervalMs: ptr(int(c))})
			case 3:
				clock.Add(int64(time.Duration(c) * time.Second))
				u.Reap()
			case 4:
				_ = u.Stop(testOwner, s.ID)
			case 5:
				if done == nil {
					done = make(chan error, 1)
					ack := make(chan struct{})
					var once sync.Once
					go func() {
						defer once.Do(func() { close(ack) })
						done <- u.Subscribe(context.Background(), testOwner, s.ID, func(context.Context, entities.Event) error { once.Do(func() { close(ack) }); return nil })
					}()
					select {
					case <-ack:
					case <-time.After(time.Second):
						t.Fatal("subscription did not acknowledge")
					}
				}
			case 6:
				_, _ = u.Update(context.Background(), testOwner, s.ID, v.Patch{Paused: ptr(c%2 == 0)})
			case 7:
				_, _ = u.Update(context.Background(), testOwner, s.ID, v.Patch{IntervalMs: ptr(100 + int(c)), ItemsPerMessage: ptr(1 + int(c)%100)})
			}
		}
		u.Close()
		if done != nil {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("subscription cleanup stuck")
			}
		}
		if u.Stats().Sessions != 0 {
			t.Fatal("cleanup leaked")
		}
	})
}
