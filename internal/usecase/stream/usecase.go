package stream

import (
	"context"
	"github.com/endge-lab/service-mock-generator/internal/domain/entities"
	errs "github.com/endge-lab/service-mock-generator/internal/domain/errors"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"github.com/endge-lab/service-mock-generator/internal/usecase/generate"
	"github.com/google/uuid"
	"sync"
	"sync/atomic"
	"time"
)

type session struct {
	mu         sync.Mutex
	work       sync.Mutex
	owner      v.Owner
	info       entities.Stream
	plan       *generate.Plan
	cursor     *generate.Cursor
	ctx        context.Context
	cancel     context.CancelCauseFunc
	wake       chan struct{}
	subscribed bool
}
type UseCase struct {
	mu           sync.Mutex
	sessions     map[string]*session
	pending      map[v.Owner]int
	generator    *generate.UseCase
	limits       v.Limits
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan struct{}
	now          func() time.Time
	buffered     atomic.Int64
	expired      atomic.Int64
	batches      atomic.Int64
	errors       atomic.Int64
	backpressure atomic.Int64
}
type Stats struct {
	Sessions      int   `json:"sessions"`
	Jobs          int64 `json:"jobs"`
	BufferedBytes int64 `json:"bufferedBytes"`
	Expired       int64 `json:"expired"`
	Batches       int64 `json:"batches"`
	Errors        int64 `json:"errors"`
	Backpressure  int64 `json:"backpressure"`
}

func NewUseCase(g *generate.UseCase, l v.Limits) *UseCase {
	ctx, cancel := context.WithCancel(context.Background())
	return &UseCase{sessions: map[string]*session{}, pending: map[v.Owner]int{}, generator: g, limits: l, ctx: ctx, cancel: cancel, done: make(chan struct{}), now: time.Now}
}
func (u *UseCase) Start() {
	go func() {
		defer close(u.done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-u.ctx.Done():
				return
			case <-ticker.C:
				u.Reap()
			}
		}
	}()
}
func (u *UseCase) Close() {
	u.cancel()
	u.mu.Lock()
	items := make([]*session, 0, len(u.sessions))
	for _, s := range u.sessions {
		items = append(items, s)
	}
	u.sessions = map[string]*session{}
	u.mu.Unlock()
	for _, s := range items {
		s.cancel(errs.New("stream.stopped", "Service is shutting down", 503))
	}
}
func (u *UseCase) Stats() Stats {
	u.mu.Lock()
	n := len(u.sessions)
	u.mu.Unlock()
	return Stats{Sessions: n, Jobs: u.generator.ActiveJobs(), BufferedBytes: u.buffered.Load(), Expired: u.expired.Load(), Batches: u.batches.Load(), Errors: u.errors.Load(), Backpressure: u.backpressure.Load()}
}
func (u *UseCase) Create(ctx context.Context, owner v.Owner, r v.StreamRequest) (entities.Stream, error) {
	if owner.ActorID == "" || owner.WorkspaceID == "" {
		return entities.Stream{}, errs.InvalidInput("stream.owner_invalid", "Owner context is required")
	}
	if err := r.Generation.Validate(u.limits, true); err != nil {
		return entities.Stream{}, err
	}
	if err := r.Stream.Validate(u.limits); err != nil {
		return entities.Stream{}, err
	}
	u.mu.Lock()
	total := len(u.sessions)
	owned := 0
	for o, n := range u.pending {
		total += n
		if o.ActorID == owner.ActorID {
			owned += n
		}
	}
	for _, s := range u.sessions {
		if s.owner.ActorID == owner.ActorID {
			owned++
		}
	}
	if u.ctx.Err() != nil {
		u.mu.Unlock()
		return entities.Stream{}, errs.New("stream.unavailable", "Service is shutting down", 503)
	}
	if total >= u.limits.Sessions || owned >= u.limits.SessionsPerOwner {
		u.mu.Unlock()
		return entities.Stream{}, errs.New("stream.limit_exceeded", "Session capacity exhausted", 429)
	}
	u.pending[owner]++
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.pending[owner]--
		if u.pending[owner] == 0 {
			delete(u.pending, owner)
		}
		u.mu.Unlock()
	}()
	plan, err := u.generator.Compile(ctx, r.Schema, r.Generation)
	if err != nil {
		return entities.Stream{}, err
	}
	if err := ctx.Err(); err != nil {
		return entities.Stream{}, err
	}
	c, cancel := context.WithCancelCause(u.ctx)
	info := entities.Stream{ID: uuid.NewString(), Status: "ready", Seed: plan.Seed, Profile: "default-v1", Parameters: r.Stream, ParametersVersion: 1, IdleTimeoutMs: u.limits.IdleTimeout.Milliseconds(), ExpiresAt: u.now().Add(u.limits.ReadyTimeout)}
	s := &session{owner: owner, info: info, plan: plan, cursor: plan.NewCursor(), ctx: c, cancel: cancel, wake: make(chan struct{}, 1)}
	u.mu.Lock()
	if u.ctx.Err() != nil {
		u.mu.Unlock()
		cancel(context.Canceled)
		return entities.Stream{}, context.Canceled
	}
	u.sessions[info.ID] = s
	u.mu.Unlock()
	return info, nil
}
func (u *UseCase) find(owner v.Owner, id string) (*session, error) {
	u.mu.Lock()
	s := u.sessions[id]
	u.mu.Unlock()
	if s == nil || s.owner != owner {
		return nil, errs.NotFound("stream.not_found", "Stream does not exist")
	}
	s.mu.Lock()
	expired := !u.now().Before(s.info.ExpiresAt)
	s.mu.Unlock()
	if expired {
		u.finish(s, errs.New("stream.expired", "Stream lease expired", 404))
		return nil, errs.NotFound("stream.not_found", "Stream does not exist")
	}
	if s.ctx.Err() != nil {
		return nil, errs.NotFound("stream.not_found", "Stream does not exist")
	}
	return s, nil
}
func (u *UseCase) Get(owner v.Owner, id string) (entities.Stream, error) {
	s, err := u.find(owner, id)
	if err != nil {
		return entities.Stream{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info, nil
}
func (u *UseCase) KeepAlive(owner v.Owner, id string) (entities.Stream, error) {
	s, err := u.find(owner, id)
	if err != nil {
		return entities.Stream{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil || !u.now().Before(s.info.ExpiresAt) {
		return entities.Stream{}, errs.NotFound("stream.not_found", "Stream has expired")
	}
	if !s.subscribed {
		return entities.Stream{}, errs.Conflict("stream.invalid_state", "Subscribe before renewing the lease")
	}
	s.info.ExpiresAt = u.now().Add(u.limits.IdleTimeout)
	return s.info, nil
}
func (u *UseCase) Update(ctx context.Context, owner v.Owner, id string, p v.Patch) (entities.Stream, error) {
	s, err := u.find(owner, id)
	if err != nil {
		return entities.Stream{}, err
	}
	if p.IntervalMs == nil && p.ItemsPerMessage == nil && p.Paused == nil {
		return entities.Stream{}, errs.InvalidInput("stream.patch_invalid", "Patch is empty")
	}
	s.work.Lock()
	defer s.work.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entities.Stream{}, err
	}
	if s.ctx.Err() != nil || !u.now().Before(s.info.ExpiresAt) {
		return entities.Stream{}, errs.NotFound("stream.not_found", "Stream has expired")
	}
	params := s.info.Parameters
	if p.IntervalMs != nil {
		params.IntervalMs = *p.IntervalMs
	}
	if p.ItemsPerMessage != nil {
		params.ItemsPerMessage = *p.ItemsPerMessage
	}
	if err := params.Validate(u.limits); err != nil {
		return entities.Stream{}, err
	}
	if p.Paused != nil && !s.subscribed {
		return entities.Stream{}, errs.Conflict("stream.invalid_state", "Subscribe before pausing")
	}
	s.info.Parameters = params
	if p.Paused != nil {
		if *p.Paused {
			s.info.Status = "paused"
		} else {
			s.info.Status = "running"
		}
	}
	s.info.ParametersVersion++
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return s.info, nil
}
func (u *UseCase) Stop(owner v.Owner, id string) error {
	s, err := u.find(owner, id)
	if err != nil {
		return err
	}
	u.finish(s, errs.New("stream.stopped", "Stream stopped", 200))
	return nil
}
func (u *UseCase) finish(s *session, cause error) {
	s.cancel(cause)
	u.mu.Lock()
	if u.sessions[s.info.ID] == s {
		delete(u.sessions, s.info.ID)
		if errs.CodeOf(cause) == "stream.expired" {
			u.expired.Add(1)
		}
	}
	u.mu.Unlock()
}
func (u *UseCase) Reap() {
	u.mu.Lock()
	items := make([]*session, 0, len(u.sessions))
	for _, s := range u.sessions {
		items = append(items, s)
	}
	u.mu.Unlock()
	for _, s := range items {
		s.mu.Lock()
		expired := !u.now().Before(s.info.ExpiresAt)
		if expired {
			s.cancel(errs.New("stream.expired", "Stream lease expired", 404))
		}
		s.mu.Unlock()
		if expired {
			u.finish(s, errs.New("stream.expired", "Stream lease expired", 404))
		}
	}
}
func (u *UseCase) Subscribe(ctx context.Context, owner v.Owner, id string, emit func(context.Context, entities.Event) error) (err error) {
	s, err := u.find(owner, id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.subscribed {
		s.mu.Unlock()
		return errs.Conflict("stream.already_subscribed", "Stream already has a subscriber")
	}
	if s.ctx.Err() != nil || !u.now().Before(s.info.ExpiresAt) {
		s.mu.Unlock()
		return errs.NotFound("stream.not_found", "Stream does not exist")
	}
	s.subscribed = true
	s.info.Status = "running"
	s.info.ExpiresAt = u.now().Add(u.limits.IdleTimeout)
	params := s.info.Parameters
	version := s.info.ParametersVersion
	s.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	off := context.AfterFunc(s.ctx, cancel)
	defer off()
	defer func() {
		u.finish(s, err)
		if err != nil && err != context.Canceled {
			u.errors.Add(1)
		}
	}()
	if err = emit(ctx, entities.Event{Type: "started", StreamID: id, ParametersVersion: version}); err != nil {
		return err
	}
	delay := time.Duration(0)
	if params.EmitImmediately != nil && !*params.EmitImmediately {
		delay = time.Duration(params.IntervalMs) * time.Millisecond
	}
	for {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return u.end(s, ctx)
			case <-s.wake:
				timer.Stop()
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return u.end(s, ctx)
		}
		s.work.Lock()
		s.mu.Lock()
		info := s.info
		s.mu.Unlock()
		if info.Status == "paused" {
			s.work.Unlock()
			select {
			case <-ctx.Done():
				return u.end(s, ctx)
			case <-s.wake:
			}
			delay = 0
			continue
		}
		result, e := u.generator.Batch(ctx, s.plan, s.cursor, info.Parameters.ItemsPerMessage)
		if e != nil {
			s.work.Unlock()
			return e
		}
		size := 0
		for _, item := range result.Items {
			size += len(item)
		}
		if u.buffered.Add(int64(size)) > int64(u.limits.BufferBytes) {
			u.buffered.Add(-int64(size))
			s.work.Unlock()
			u.backpressure.Add(1)
			return errs.New("stream.backpressure", "Global stream buffer budget exceeded", 429)
		}
		s.mu.Lock()
		s.info.Sequence++
		seq := s.info.Sequence
		s.mu.Unlock()
		e = emit(ctx, entities.Event{Type: "data", StreamID: id, Sequence: seq, ParametersVersion: info.ParametersVersion, Items: result.Items})
		u.buffered.Add(-int64(size))
		s.work.Unlock()
		if e != nil {
			u.backpressure.Add(1)
			return e
		}
		u.batches.Add(1)
		delay = time.Duration(info.Parameters.IntervalMs) * time.Millisecond
	}
}
func (u *UseCase) end(s *session, ctx context.Context) error {
	cause := context.Cause(s.ctx)
	if cause != nil {
		return cause
	}
	return ctx.Err()
}
