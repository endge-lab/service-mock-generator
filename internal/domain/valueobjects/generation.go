package valueobjects

import (
	"bytes"
	"encoding/json"
	errs "github.com/endge-lab/service-mock-generator/internal/domain/errors"
	"io"
	"time"
	"unicode/utf8"
)

type Limits struct {
	RequestBytes      int           `json:"requestBytes"`
	ResultBytes       int           `json:"resultBytes"`
	Nodes             int           `json:"schemaNodes"`
	Depth             int           `json:"depth"`
	Definitions       int           `json:"definitions"`
	ArrayLength       int           `json:"arrayLength"`
	StringLength      int           `json:"stringLength"`
	Relations         int           `json:"relations"`
	Mappings          int           `json:"mappings"`
	Attempts          int           `json:"attempts"`
	Count             int           `json:"count"`
	ItemsPerMessage   int           `json:"itemsPerMessage"`
	MinIntervalMs     int           `json:"minIntervalMs"`
	MaxIntervalMs     int           `json:"maxIntervalMs"`
	Sessions          int           `json:"sessions"`
	SessionsPerOwner  int           `json:"sessionsPerOwner"`
	Jobs              int           `json:"jobs"`
	BufferBytes       int           `json:"bufferBytes"`
	GenerationTimeout time.Duration `json:"-"`
	IdleTimeout       time.Duration `json:"-"`
	ReadyTimeout      time.Duration `json:"-"`
	WriteTimeout      time.Duration `json:"-"`
}

func DefaultLimits() Limits {
	return Limits{RequestBytes: 2 << 20, ResultBytes: 20 << 20, Nodes: 10000, Depth: 32, Definitions: 1000, ArrayLength: 1000, StringLength: 10000, Relations: 1000, Mappings: 100, Attempts: 100, Count: 1000, ItemsPerMessage: 100, MinIntervalMs: 100, MaxIntervalMs: 60000, Sessions: 32, SessionsPerOwner: 5, Jobs: 4, BufferBytes: 64 << 20, GenerationTimeout: 10 * time.Second, IdleTimeout: 180 * time.Second, ReadyTimeout: 30 * time.Second, WriteTimeout: 10 * time.Second}
}

type Selector struct {
	Path     string `json:"path"`
	UniqueBy string `json:"uniqueBy,omitempty"`
}
type Target struct {
	Path string `json:"path"`
}
type Mapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type Relation struct {
	ID        string    `json:"id"`
	Source    Selector  `json:"source"`
	Target    Target    `json:"target"`
	Selection string    `json:"selection"`
	Mappings  []Mapping `json:"mappings"`
}
type Options struct {
	Count                       *int       `json:"count,omitempty"`
	Seed                        *string    `json:"seed,omitempty"`
	Profile                     string     `json:"profile,omitempty"`
	OptionalPropertyProbability *float64   `json:"optionalPropertyProbability,omitempty"`
	DefaultArrayLength          *int       `json:"defaultArrayLength,omitempty"`
	Relations                   []Relation `json:"relations,omitempty"`
}
type Request struct {
	Schema     json.RawMessage `json:"schema"`
	Generation Options         `json:"generation"`
}
type Metadata struct {
	Count   int    `json:"count"`
	Seed    string `json:"seed"`
	Profile string `json:"profile"`
}
type Result struct {
	Items []json.RawMessage `json:"items"`
	Meta  Metadata          `json:"meta"`
}
type Parameters struct {
	IntervalMs      int   `json:"intervalMs"`
	ItemsPerMessage int   `json:"itemsPerMessage"`
	EmitImmediately *bool `json:"emitImmediately,omitempty"`
}
type StreamRequest struct {
	Schema     json.RawMessage `json:"schema"`
	Generation Options         `json:"generation"`
	Stream     Parameters      `json:"stream"`
}
type Patch struct {
	IntervalMs      *int  `json:"intervalMs,omitempty"`
	ItemsPerMessage *int  `json:"itemsPerMessage,omitempty"`
	Paused          *bool `json:"paused,omitempty"`
}
type Owner struct {
	ActorID     string
	WorkspaceID string
}

func Decode(data []byte, into any, max int) error {
	if len(data) > max {
		return errs.New("request.too_large", "Request exceeds the byte limit", 413)
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err := d.Decode(into); err != nil {
		return errs.InvalidInput("request.invalid", "Invalid JSON request")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errs.InvalidInput("request.invalid", "Expected exactly one JSON value")
	}
	return nil
}
func (o Options) Validate(l Limits, stream bool) error {
	if stream && o.Count != nil {
		return errs.InvalidInput("generation.count_invalid", "count is not allowed for streams")
	}
	if !stream && (o.Count == nil || *o.Count < 1 || *o.Count > l.Count) {
		return errs.InvalidInput("generation.count_invalid", "count is out of range")
	}
	if o.Profile != "" && o.Profile != "default-v1" {
		return errs.InvalidInput("generation.profile_unsupported", "Unsupported generation profile")
	}
	if o.Seed != nil && (utf8.RuneCountInString(*o.Seed) < 1 || utf8.RuneCountInString(*o.Seed) > 256) {
		return errs.InvalidInput("generation.seed_invalid", "seed must contain 1 to 256 characters")
	}
	if o.OptionalPropertyProbability != nil && (*o.OptionalPropertyProbability < 0 || *o.OptionalPropertyProbability > 1) {
		return errs.InvalidInput("generation.options_invalid", "Probability must be between zero and one")
	}
	if o.DefaultArrayLength != nil && (*o.DefaultArrayLength < 0 || *o.DefaultArrayLength > 100) {
		return errs.InvalidInput("generation.options_invalid", "Default array length must be between zero and 100")
	}
	if len(o.Relations) > l.Relations {
		return errs.InvalidInput("generation.limit_exceeded", "Too many relations")
	}
	return nil
}
func (p Parameters) Validate(l Limits) error {
	if p.IntervalMs < l.MinIntervalMs || p.IntervalMs > l.MaxIntervalMs || p.ItemsPerMessage < 1 || p.ItemsPerMessage > l.ItemsPerMessage {
		return errs.InvalidInput("stream.rate_invalid", "Stream parameters are out of range")
	}
	return nil
}
