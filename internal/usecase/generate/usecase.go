package generate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	errs "github.com/endge-lab/service-mock-generator/internal/domain/errors"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"github.com/endge-lab/service-mock-generator/internal/usecase/ports"
	"math/big"
	prng "math/rand/v2"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type UseCase struct {
	compiler    ports.SchemaCompiler
	limits      v.Limits
	jobs        chan struct{}
	active      atomic.Int64
	calls       atomic.Int64
	failures    atomic.Int64
	duration    atomic.Int64
	items       atomic.Int64
	resultBytes atomic.Int64
}
type Plan struct {
	root          *node
	validator     ports.SchemaValidator
	options       v.Options
	relations     []relationPlan
	deferred      [][]step
	requiredPaths [][]step
	limits        v.Limits
	Seed          string
}
type Cursor struct{ random *prng.Rand }

func NewUseCase(c ports.SchemaCompiler, l v.Limits) *UseCase {
	return &UseCase{compiler: c, limits: l, jobs: make(chan struct{}, l.Jobs)}
}

type Stats struct{ Calls, Failures, DurationNanos, Items, ResultBytes int64 }

func (u *UseCase) Stats() Stats {
	return Stats{u.calls.Load(), u.failures.Load(), u.duration.Load(), u.items.Load(), u.resultBytes.Load()}
}
func (u *UseCase) ActiveJobs() int64 { return u.active.Load() }
func (u *UseCase) acquire() error {
	select {
	case u.jobs <- struct{}{}:
		u.active.Add(1)
		return nil
	default:
		return errs.New("generation.busy", "Generation capacity exhausted", 429)
	}
}
func (u *UseCase) release() { <-u.jobs; u.active.Add(-1) }
func (u *UseCase) Compile(ctx context.Context, raw json.RawMessage, options v.Options) (*Plan, error) {
	if err := u.acquire(); err != nil {
		return nil, err
	}
	defer u.release()
	ctx, cancel := context.WithTimeout(ctx, u.limits.GenerationTimeout)
	defer cancel()
	var root map[string]any
	if err := v.Decode(raw, &root, u.limits.RequestBytes); err != nil {
		return nil, err
	}
	if root == nil {
		return nil, invalid("schema.invalid", "", "Schema is required")
	}
	if root["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return nil, invalid("schema.dialect_unsupported", "", "Draft 2020-12 declaration is required")
	}
	count := 0
	if err := walkBudget(ctx, root, 0, &count, u.limits); err != nil {
		return nil, err
	}
	b := builder{root: root, limits: u.limits, nodes: map[string]*node{}, active: map[string]bool{}, ctx: ctx}
	n, err := b.build(root, "", 0)
	if err != nil {
		return nil, err
	}
	// Strip unreachable definitions before validation: they do not affect this generation.
	cleaned := make(map[string]any, len(root))
	for k, val := range root {
		cleaned[k] = val
	}
	if defs, ok := root["$defs"].(map[string]any); ok {
		reachable := map[string]any{}
		for k, val := range defs {
			prefix := "/$defs/" + token(k)
			for path := range b.nodes {
				if path == prefix || strings.HasPrefix(path, prefix+"/") {
					reachable[k] = val
					break
				}
			}
		}
		cleaned["$defs"] = reachable
	}
	raw, err = json.Marshal(cleaned)
	if err != nil {
		return nil, invalid("schema.invalid", "", "Invalid schema")
	}
	validator, err := u.compiler.Compile(raw)
	if err != nil {
		return nil, invalid("schema.invalid", "", "Schema fails Draft 2020-12 validation")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	seed := ""
	if options.Seed != nil {
		seed = *options.Seed
	} else {
		data := make([]byte, 16)
		if _, err = rand.Read(data); err != nil {
			return nil, err
		}
		seed = hex.EncodeToString(data)
	}
	p := &Plan{root: n, validator: validator, options: options, limits: u.limits, Seed: seed}
	if err := p.compileRelations(ctx); err != nil {
		return nil, err
	}
	return p, nil
}
func (p *Plan) NewCursor() *Cursor {
	hash := sha256.Sum256([]byte(p.Seed))
	return &Cursor{random: prng.New(prng.NewPCG(binary.BigEndian.Uint64(hash[:8]), binary.BigEndian.Uint64(hash[8:16])))}
}
func (u *UseCase) Generate(ctx context.Context, req v.Request) (v.Result, error) {
	if err := req.Generation.Validate(u.limits, false); err != nil {
		return v.Result{}, err
	}
	p, err := u.Compile(ctx, req.Schema, req.Generation)
	if err != nil {
		return v.Result{}, err
	}
	return u.Batch(ctx, p, p.NewCursor(), *req.Generation.Count)
}
func (u *UseCase) Batch(ctx context.Context, p *Plan, c *Cursor, count int) (result v.Result, err error) {
	started := time.Now()
	u.calls.Add(1)
	defer func() {
		u.duration.Add(time.Since(started).Nanoseconds())
		if err != nil {
			u.failures.Add(1)
		} else {
			u.items.Add(int64(len(result.Items)))
			for _, raw := range result.Items {
				u.resultBytes.Add(int64(len(raw)))
			}
		}
	}()
	if count < 1 || count > u.limits.Count {
		return result, errs.InvalidInput("generation.count_invalid", "Count out of range")
	}
	if err = u.acquire(); err != nil {
		return result, err
	}
	defer u.release()
	ctx, cancel := context.WithTimeout(ctx, u.limits.GenerationTimeout)
	defer cancel()
	result = v.Result{Items: make([]json.RawMessage, 0, count), Meta: v.Metadata{Count: count, Seed: p.Seed, Profile: "default-v1"}}
	metadataBytes, _ := json.Marshal(result.Meta)
	bytesUsed := 512 + len(metadataBytes)
	for range count {
		g := generation{plan: p, random: c.random, ctx: ctx, remaining: u.limits.ResultBytes - bytesUsed, steps: u.limits.Nodes * u.limits.Attempts * 2}
		value, e := g.value(p.root, nil, 0)
		if e != nil {
			return v.Result{}, e
		}
		if e = p.applyRelations(&g, value); e != nil {
			return v.Result{}, e
		}
		if e = valueLimits(ctx, value, 0, p.limits); e != nil {
			return v.Result{}, e
		}
		if e = p.validator.Validate("", value); e != nil {
			return v.Result{}, invalid("schema.not_generatable", "", "Generated root does not satisfy all constraints")
		}
		raw, e := json.Marshal(value)
		if e != nil {
			return v.Result{}, errs.Internal("generation.internal", "Cannot encode generated value")
		}
		bytesUsed += len(raw) + 1
		if bytesUsed > u.limits.ResultBytes {
			return v.Result{}, errs.New("generation.limit_exceeded", "Result exceeds byte limit", 413)
		}
		result.Items = append(result.Items, raw)
	}
	if err = ctx.Err(); err != nil {
		return v.Result{}, err
	}
	return result, nil
}

type generation struct {
	plan             *Plan
	random           *prng.Rand
	ctx              context.Context
	remaining, steps int
}

func (g *generation) charge(n int) error {
	g.steps--
	g.remaining -= n
	if err := g.ctx.Err(); err != nil {
		return err
	}
	if g.remaining < 0 || g.steps < 0 {
		return errs.New("generation.limit_exceeded", "Generation resource budget exceeded", 413)
	}
	return nil
}
func (g *generation) value(n *node, path []step, depth int) (any, error) {
	if depth > g.plan.limits.Depth {
		return nil, invalid("schema.limit_exceeded", n.path, "Generated nesting exceeds limit")
	}
	if err := g.charge(8); err != nil {
		return nil, err
	}
	if g.plan.isDeferred(path) {
		return nil, nil
	}
	for attempt := 0; attempt < g.plan.limits.Attempts; attempt++ {
		x, err := g.candidate(n, path, depth)
		if err != nil {
			return nil, err
		}
		if g.plan.hasDeferred(path) || g.plan.validator.Validate(n.path, x) == nil {
			return x, nil
		}
	}
	return nil, invalid("schema.not_generatable", n.path, "Cannot satisfy constraints within attempt budget")
}
func (g *generation) candidate(n *node, path []step, depth int) (any, error) {
	if err := g.charge(1); err != nil {
		return nil, err
	}
	m := n.raw
	if n.ref != nil {
		return g.value(n.ref, path, depth+1)
	}
	if x, ok := m["const"]; ok {
		return g.clone(x)
	}
	if a, ok := m["enum"].([]any); ok {
		if len(a) == 0 {
			return nil, invalid("schema.not_generatable", n.path, "Empty enum")
		}
		return g.clone(a[g.random.IntN(len(a))])
	}
	if a, ok := m["examples"].([]any); ok && len(a) > 0 {
		start := g.random.IntN(len(a))
		for i := range a {
			x := a[(start+i)%len(a)]
			if g.plan.validator.Validate(n.path, x) == nil {
				return g.clone(x)
			}
		}
	}
	if x, ok := m["default"]; ok && g.plan.validator.Validate(n.path, x) == nil {
		return g.clone(x)
	}
	if len(n.branches) > 0 {
		return g.value(n.branches[g.random.IntN(len(n.branches))], path, depth+1)
	}
	typ, _ := m["type"].(string)
	if types, ok := m["type"].([]any); ok {
		typ = types[g.random.IntN(len(types))].(string)
	}
	if typ == "" {
		return nil, invalid("schema.not_generatable", n.path, "A generatable type or value is required")
	}
	switch typ {
	case "null":
		return nil, nil
	case "boolean":
		return g.random.IntN(2) == 1, nil
	case "integer", "number":
		return g.number(n, typ == "integer")
	case "string":
		return g.text(n)
	case "array":
		low, high := integer(m, "minItems", 0), integer(m, "maxItems", g.plan.limits.ArrayLength)
		if low > high || low > g.plan.limits.ArrayLength {
			return nil, invalid("schema.not_generatable", n.path, "Array bounds exceed limits")
		}
		if high > g.plan.limits.ArrayLength {
			high = g.plan.limits.ArrayLength
		}
		length := 3
		if g.plan.options.DefaultArrayLength != nil {
			length = *g.plan.options.DefaultArrayLength
		}
		_, hasMin := m["minItems"]
		_, hasMax := m["maxItems"]
		if hasMin && hasMax {
			length = low + g.random.IntN(high-low+1)
		} else {
			length = max(low, min(length, high))
		}
		if length > 0 && n.item == nil {
			return nil, invalid("schema.not_generatable", n.path, "Array items schema is required")
		}
		if err := g.charge(length * 8); err != nil {
			return nil, err
		}
		a := make([]any, 0, length)
		seen := map[string]bool{}
		unique, _ := m["uniqueItems"].(bool)
		for i := 0; i < length; i++ {
			accepted := false
			for attempt := 0; attempt < g.plan.limits.Attempts; attempt++ {
				x, err := g.value(n.item, appendPath(path, step{index: i, array: true}), depth+1)
				if err != nil {
					return nil, err
				}
				key := canonical(x)
				if unique && seen[key] {
					continue
				}
				seen[key] = true
				a = append(a, x)
				accepted = true
				break
			}
			if !accepted {
				return nil, invalid("schema.not_generatable", n.path, "Cannot generate enough unique items")
			}
		}
		return a, nil
	case "object":
		required := map[string]bool{}
		if a, ok := m["required"].([]any); ok {
			for _, k := range a {
				required[k.(string)] = true
			}
		}
		names := keys(n.props)
		prob := 0.7
		if g.plan.options.OptionalPropertyProbability != nil {
			prob = *g.plan.options.OptionalPropertyProbability
		}
		low := integer(m, "minProperties", 0)
		capacity := len(names)
		if n.additional != nil {
			capacity = max(capacity, low)
			if len(names) == 0 {
				capacity = max(capacity, 3)
			}
		}
		high := integer(m, "maxProperties", capacity)
		if low > capacity || low > high || len(required) > high {
			return nil, invalid("schema.not_generatable", n.path, "Object property bounds are impossible")
		}
		chosen := map[string]bool{}
		for k := range required {
			chosen[k] = true
		}
		for _, k := range names {
			p := appendPath(path, step{key: k})
			if g.plan.hasDeferred(p) || g.plan.requiresPath(p) {
				chosen[k] = true
			}
		}
		if len(chosen) > high {
			return nil, invalid("schema.not_generatable", n.path, "Relation target exceeds property limits")
		}
		for _, k := range names {
			if !chosen[k] && len(chosen) < high && g.random.Float64() < prob {
				chosen[k] = true
			}
		}
		for _, k := range names {
			if len(chosen) >= low {
				break
			}
			chosen[k] = true
		}
		// У словаря без именованных полей создаём до трёх ключей; min/max имеют приоритет.
		target := low
		if n.additional != nil && len(names) == 0 {
			target = max(low, min(3, high))
		}
		for i := 1; len(chosen) < target; i++ {
			if err := g.charge(8); err != nil {
				return nil, err
			}
			key := "key_" + strconv.Itoa(i)
			if n.props[key] != nil {
				continue
			}
			chosen[key] = true
			names = append(names, key)
		}
		result := map[string]any{}
		for _, k := range names {
			if !chosen[k] {
				continue
			}
			if err := g.charge(len(k) + 4); err != nil {
				return nil, err
			}
			child := n.props[k]
			if child == nil {
				child = n.additional
			}
			x, err := g.value(child, appendPath(path, step{key: k}), depth+1)
			if err != nil {
				return nil, err
			}
			result[k] = x
		}
		return result, nil
	}
	return nil, invalid("schema.not_generatable", n.path, "Unsupported schema")
}
func canonical(x any) string {
	switch x := x.(type) {
	case json.Number:
		r, ok := new(big.Rat).SetString(string(x))
		if ok {
			return "number:" + r.RatString()
		}
	case []any:
		var b strings.Builder
		b.WriteByte('[')
		for _, v := range x {
			b.WriteString(canonical(v))
			b.WriteByte(',')
		}
		b.WriteByte(']')
		return b.String()
	case map[string]any:
		var b strings.Builder
		b.WriteByte('{')
		for _, k := range keys(x) {
			key, _ := json.Marshal(k)
			b.Write(key)
			b.WriteByte(':')
			b.WriteString(canonical(x[k]))
			b.WriteByte(',')
		}
		b.WriteByte('}')
		return b.String()
	}
	b, _ := json.Marshal(x)
	return string(b)
}

func (g *generation) clone(x any) (any, error) {
	switch x := x.(type) {
	case map[string]any:
		if err := g.charge(len(x) * 8); err != nil {
			return nil, err
		}
		r := map[string]any{}
		for _, k := range keys(x) {
			if err := g.charge(len(k) + 4); err != nil {
				return nil, err
			}
			y, err := g.clone(x[k])
			if err != nil {
				return nil, err
			}
			r[k] = y
		}
		return r, nil
	case []any:
		if err := g.charge(len(x) * 8); err != nil {
			return nil, err
		}
		r := make([]any, len(x))
		for i, y := range x {
			z, err := g.clone(y)
			if err != nil {
				return nil, err
			}
			r[i] = z
		}
		return r, nil
	case string:
		if err := g.charge(len(x)*6 + 2); err != nil {
			return nil, err
		}
		return x, nil
	default:
		if err := g.charge(32); err != nil {
			return nil, err
		}
		return x, nil
	}
}
