package generate

import (
	"context"
	"encoding/json"
	errs "github.com/endge-lab/service-mock-generator/internal/domain/errors"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type node struct {
	raw        map[string]any
	path       string
	ref        *node
	props      map[string]*node
	additional *node
	item       *node
	branches   []*node
	pattern    *pattern
}
type builder struct {
	root   map[string]any
	limits v.Limits
	nodes  map[string]*node
	active map[string]bool
	ctx    context.Context
}

var keywords = map[string]bool{}

func init() {
	for _, k := range strings.Fields("$schema $defs $ref title description default examples deprecated readOnly writeOnly const enum type oneOf properties required additionalProperties minProperties maxProperties items minItems maxItems uniqueItems minLength maxLength format pattern minimum maximum exclusiveMinimum exclusiveMaximum multipleOf") {
		keywords[k] = true
	}
}
func invalid(code, path, message string) error {
	return errs.WithDetails(errs.InvalidInput(errs.Code(code), message), map[string]any{"path": "/schema" + path})
}
func token(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }
func (b *builder) build(raw any, path string, depth int) (*node, error) {
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	if depth > b.limits.Depth {
		return nil, invalid("schema.limit_exceeded", path, "Schema depth exceeds limit")
	}
	if b.active[path] {
		return nil, invalid("schema.ref_cycle", path, "Recursive schema references are unsupported")
	}
	if n := b.nodes[path]; n != nil {
		return n, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, invalid("schema.keyword_unsupported", path, "Expected an object schema")
	}
	if len(b.nodes) >= b.limits.Nodes {
		return nil, invalid("schema.limit_exceeded", path, "Too many schema nodes")
	}
	n := &node{raw: m, path: path, props: map[string]*node{}}
	b.nodes[path] = n
	b.active[path] = true
	defer delete(b.active, path)
	for _, k := range keys(m) {
		if !keywords[k] {
			return nil, invalid("schema.keyword_unsupported", path+"/"+token(k), "Unsupported schema keyword")
		}
	}
	for _, key := range []string{"const", "default"} {
		if value, exists := m[key]; exists {
			if err := valueLimits(b.ctx, value, 0, b.limits); err != nil {
				return nil, err
			}
		}
	}
	for _, key := range []string{"enum", "examples"} {
		if values, ok := m[key].([]any); ok {
			for _, value := range values {
				if err := valueLimits(b.ctx, value, 0, b.limits); err != nil {
					return nil, err
				}
			}
		}
	}
	if dialect, exists := m["$schema"]; exists && dialect != "https://json-schema.org/draft/2020-12/schema" {
		return nil, invalid("schema.dialect_unsupported", path+"/$schema", "Expected Draft 2020-12")
	}
	if defs, ok := m["$defs"].(map[string]any); ok && len(defs) > b.limits.Definitions {
		return nil, invalid("schema.limit_exceeded", path+"/$defs", "Too many definitions")
	}
	if ref, exists := m["$ref"]; exists {
		s, ok := ref.(string)
		if !ok || !strings.HasPrefix(s, "#/$defs/") {
			return nil, invalid("schema.ref_unsupported", path+"/$ref", "Only local definitions are supported")
		}
		parts, err := pointer(strings.TrimPrefix(s, "#"))
		if err != nil {
			return nil, invalid("schema.ref_invalid", path, "Invalid local reference")
		}
		target, err := readPointer(b.root, parts)
		if err != nil {
			return nil, invalid("schema.ref_missing", path, "Reference target does not exist")
		}
		n.ref, err = b.build(target, s[1:], depth+1)
		if err != nil {
			return nil, err
		}
	}
	if t, exists := m["type"]; exists {
		switch a := t.(type) {
		case string:
			if !validType(a) {
				return nil, invalid("schema.type_unsupported", path, "Unsupported type")
			}
		case []any:
			if len(a) != 2 {
				return nil, invalid("schema.type_unsupported", path, "Only nullable type unions are supported")
			}
			x, xok := a[0].(string)
			y, yok := a[1].(string)
			if !xok || !yok || !validType(x) || !validType(y) || (x == "null") == (y == "null") {
				return nil, invalid("schema.type_unsupported", path, "Only nullable type unions are supported")
			}
		default:
			return nil, invalid("schema.type_unsupported", path, "Invalid type")
		}
	}
	if p, ok := m["properties"].(map[string]any); ok {
		for _, k := range keys(p) {
			child, err := b.build(p[k], path+"/properties/"+token(k), depth+1)
			if err != nil {
				return nil, err
			}
			n.props[k] = child
		}
	}
	if ap, exists := m["additionalProperties"]; exists {
		switch ap := ap.(type) {
		case bool:
		case map[string]any:
			var err error
			n.additional, err = b.build(ap, path+"/additionalProperties", depth+1)
			if err != nil {
				return nil, err
			}
		default:
			return nil, invalid("schema.keyword_unsupported", path+"/additionalProperties", "Expected a boolean or object schema")
		}
	}
	if req, ok := m["required"].([]any); ok {
		for _, k := range req {
			key, ok := k.(string)
			if !ok || (n.props[key] == nil && n.additional == nil) {
				return nil, invalid("schema.not_generatable", path, "Required property is not declared")
			}
			if n.props[key] == nil {
				n.props[key] = n.additional
			}
		}
	}
	if raw, exists := m["items"]; exists {
		var err error
		n.item, err = b.build(raw, path+"/items", depth+1)
		if err != nil {
			return nil, err
		}
	}
	if raw, exists := m["oneOf"]; exists {
		list, ok := raw.([]any)
		if !ok || len(list) == 0 {
			return nil, invalid("schema.not_generatable", path, "oneOf must be nonempty")
		}
		for i, r := range list {
			ch, err := b.build(r, path+"/oneOf/"+strconv.Itoa(i), depth+1)
			if err != nil {
				return nil, err
			}
			n.branches = append(n.branches, ch)
		}
	}
	if raw, exists := m["pattern"]; exists {
		s, ok := raw.(string)
		if !ok {
			return nil, invalid("schema.pattern_unsupported", path, "Invalid pattern")
		}
		var err error
		n.pattern, err = parsePattern(s, b.limits.StringLength)
		if err != nil {
			return nil, invalid("schema.pattern_unsupported", path+"/pattern", err.Error())
		}
	}
	if raw, exists := m["format"]; exists {
		s, ok := raw.(string)
		if !ok || !formats[s] {
			return nil, invalid("schema.format_unsupported", path+"/format", "Unsupported format")
		}
	}
	return n, nil
}
func validType(s string) bool {
	return s == "null" || s == "boolean" || s == "integer" || s == "number" || s == "string" || s == "object" || s == "array"
}
func keys[T any](m map[string]T) []string {
	a := make([]string, 0, len(m))
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}
func integer(m map[string]any, k string, fallback int) int {
	if v, ok := m[k].(json.Number); ok {
		n, err := strconv.ParseInt(string(v), 10, 64)
		if err == nil && n >= 0 && n <= 1<<30 {
			return int(n)
		}
		return 1 << 30
	}
	return fallback
}

// walkBudget also covers large literals and unused definitions before library compilation.
func walkBudget(ctx context.Context, x any, depth int, n *int, l v.Limits) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	*n++
	if *n > l.Nodes*8 || depth > l.Depth*3 {
		return invalid("schema.limit_exceeded", "", "JSON structure exceeds resource limits")
	}
	switch x := x.(type) {
	case map[string]any:
		for _, val := range x {
			if err := walkBudget(ctx, val, depth+1, n, l); err != nil {
				return err
			}
		}
	case []any:
		for _, val := range x {
			if err := walkBudget(ctx, val, depth+1, n, l); err != nil {
				return err
			}
		}
	case json.Number:
		if len(x) > 128 {
			return invalid("schema.limit_exceeded", "", "Numeric literal exceeds limit")
		}
		if at := strings.IndexAny(string(x), "eE"); at >= 0 {
			exponent, err := strconv.Atoi(string(x)[at+1:])
			if err != nil || exponent < -100 || exponent > 100 {
				return invalid("schema.limit_exceeded", "", "Numeric exponent exceeds limit")
			}
		}
	}
	return nil
}

// Literal choices and relation-composed roots obey the same structural limits
// as generated candidates. Run before compilation and before final validation.
func valueLimits(ctx context.Context, x any, depth int, l v.Limits) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > l.Depth {
		return invalid("schema.limit_exceeded", "", "Value nesting exceeds limit")
	}
	switch x := x.(type) {
	case string:
		if utf8.RuneCountInString(x) > l.StringLength {
			return invalid("schema.limit_exceeded", "", "Value string length exceeds limit")
		}
	case []any:
		if len(x) > l.ArrayLength {
			return invalid("schema.limit_exceeded", "", "Value array length exceeds limit")
		}
		for _, item := range x {
			if err := valueLimits(ctx, item, depth+1, l); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, item := range x {
			if err := valueLimits(ctx, item, depth+1, l); err != nil {
				return err
			}
		}
	}
	return nil
}
