package generate

import (
	"context"
	"fmt"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"strconv"
	"strings"
)

type step struct {
	key         string
	index       int
	array, wild bool
}
type relationPlan struct {
	definition     v.Relation
	source, target []step
	from, to       [][]string
	unique         []string
}

func appendPath(path []step, s step) []step {
	p := make([]step, len(path)+1)
	copy(p, path)
	p[len(path)] = s
	return p
}
func parseSelector(s string) ([]step, error) {
	if s == "" || s[0] != '$' {
		return nil, fmt.Errorf("selector must begin with $")
	}
	s = s[1:]
	var result []step
	for len(s) > 0 {
		switch s[0] {
		case '.':
			s = s[1:]
			i := 0
			for i < len(s) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') || s[i] == '_' || (i > 0 && s[i] >= '0' && s[i] <= '9')) {
				i++
			}
			if i == 0 {
				return nil, fmt.Errorf("invalid property selector")
			}
			result = append(result, step{key: s[:i]})
			s = s[i:]
		case '[':
			if strings.HasPrefix(s, "['") {
				end := strings.Index(s[2:], "']")
				if end < 0 {
					return nil, fmt.Errorf("invalid quoted property")
				}
				key := s[2 : 2+end]
				if strings.ContainsAny(key, "'\\") {
					return nil, fmt.Errorf("invalid property escaping")
				}
				result = append(result, step{key: key})
				s = s[4+end:]
			} else {
				end := strings.IndexByte(s, ']')
				if end < 0 {
					return nil, fmt.Errorf("unclosed array selector")
				}
				raw := s[1:end]
				a := step{array: true}
				if raw == "*" {
					a.wild = true
				} else {
					i, e := strconv.Atoi(raw)
					if e != nil || i < 0 {
						return nil, fmt.Errorf("invalid array index")
					}
					a.index = i
				}
				result = append(result, a)
				s = s[end+1:]
			}
		default:
			return nil, fmt.Errorf("unsupported selector")
		}
	}
	return result, nil
}
func pointer(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	if s[0] != '/' {
		return nil, fmt.Errorf("invalid JSON Pointer")
	}
	parts := strings.Split(s[1:], "/")
	for i, p := range parts {
		for j := 0; j < len(p); j++ {
			if p[j] == '~' {
				if j+1 == len(p) || (p[j+1] != '0' && p[j+1] != '1') {
					return nil, fmt.Errorf("invalid pointer escape")
				}
				j++
			}
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~")
	}
	return parts, nil
}
func readPointer(x any, p []string) (any, error) {
	for _, k := range p {
		switch val := x.(type) {
		case map[string]any:
			y, ok := val[k]
			if !ok {
				return nil, fmt.Errorf("missing property")
			}
			x = y
		case []any:
			i, e := strconv.Atoi(k)
			if e != nil || i < 0 || i >= len(val) {
				return nil, fmt.Errorf("invalid array index")
			}
			x = val[i]
		default:
			return nil, fmt.Errorf("path is not traversable")
		}
	}
	return x, nil
}
func writePointer(x any, p []string, value any) error {
	if len(p) == 0 {
		return fmt.Errorf("cannot replace target")
	}
	parent, e := readPointer(x, p[:len(p)-1])
	if e != nil {
		return e
	}
	switch n := parent.(type) {
	case map[string]any:
		n[p[len(p)-1]] = value
	case []any:
		i, e := strconv.Atoi(p[len(p)-1])
		if e != nil || i < 0 || i >= len(n) {
			return fmt.Errorf("invalid target index")
		}
		n[i] = value
	default:
		return fmt.Errorf("target is not writable")
	}
	return nil
}
func deref(n *node) *node {
	for n != nil && n.ref != nil {
		n = n.ref
	}
	return n
}
func schemaAt(n *node, path []step) (*node, error) {
	for _, s := range path {
		n = deref(n)
		if n == nil {
			return nil, fmt.Errorf("missing schema")
		}
		if s.array {
			n = n.item
		} else {
			n = n.props[s.key]
		}
	}
	n = deref(n)
	if n == nil {
		return nil, fmt.Errorf("missing schema")
	}
	return n, nil
}
func pointerSteps(n *node, p []string) ([]step, *node, error) {
	var result []step
	for _, k := range p {
		n = deref(n)
		if n == nil {
			return nil, nil, fmt.Errorf("missing schema")
		}
		if n.item != nil {
			i, e := strconv.Atoi(k)
			if e != nil || i < 0 {
				return nil, nil, fmt.Errorf("invalid array pointer")
			}
			result = append(result, step{array: true, index: i})
			n = n.item
		} else {
			result = append(result, step{key: k})
			n = n.props[k]
		}
	}
	n = deref(n)
	if n == nil {
		return nil, nil, fmt.Errorf("missing schema")
	}
	return result, n, nil
}
func overlaps(a, b []step) bool {
	for i := 0; i < min(len(a), len(b)); i++ {
		x, y := a[i], b[i]
		if x.array != y.array {
			return false
		}
		if x.array {
			if !x.wild && !y.wild && x.index != y.index {
				return false
			}
		} else if x.key != y.key {
			return false
		}
	}
	return true
}
func (p *Plan) isDeferred(path []step) bool {
	for _, d := range p.deferred {
		if len(d) == len(path) && overlaps(d, path) {
			return true
		}
	}
	return false
}
func (p *Plan) hasDeferred(path []step) bool {
	for _, d := range p.deferred {
		if len(d) >= len(path) && overlaps(d, path) {
			return true
		}
	}
	return false
}
func (p *Plan) requiresPath(path []step) bool {
	for _, required := range p.requiredPaths {
		if len(required) >= len(path) && overlaps(required, path) {
			return true
		}
	}
	return false
}
func typeSet(n *node) map[string]bool {
	m := map[string]bool{}
	n = deref(n)
	if s, ok := n.raw["type"].(string); ok {
		m[s] = true
	}
	if a, ok := n.raw["type"].([]any); ok {
		for _, s := range a {
			m[s.(string)] = true
		}
	}
	return m
}
func compatible(a, b *node) bool {
	x, y := typeSet(a), typeSet(b)
	if len(x) == 0 || len(y) == 0 {
		return true
	}
	for k := range x {
		if y[k] || (k == "integer" && y["number"]) {
			return true
		}
	}
	return false
}
func (p *Plan) compileRelations(ctx context.Context) error {
	remaining := p.limits.Nodes * p.limits.Attempts
	check := func() error {
		remaining--
		if e := ctx.Err(); e != nil {
			return e
		}
		if remaining < 0 {
			return invalid("relation.limit_exceeded", "", "Relation graph exceeds complexity budget")
		}
		return nil
	}
	ids := map[string]bool{}
	var plans []relationPlan
	var reads, writes [][][]step
	for _, r := range p.options.Relations {
		fail := func(code, msg string) error { return invalid(code, "", msg) }
		if r.ID == "" || ids[r.ID] || len(r.Mappings) == 0 || len(r.Mappings) > p.limits.Mappings {
			return fail("relation.invalid", "Invalid identity or mappings")
		}
		ids[r.ID] = true
		s, err := parseSelector(r.Source.Path)
		if err != nil {
			return fail("relation.path_invalid", err.Error())
		}
		t, err := parseSelector(r.Target.Path)
		if err != nil {
			return fail("relation.path_invalid", err.Error())
		}
		sn, err := schemaAt(p.root, s)
		if err != nil {
			return fail("relation.path_missing", "Missing source schema")
		}
		tn, err := schemaAt(p.root, t)
		if err != nil {
			return fail("relation.path_missing", "Missing target schema")
		}
		if typ := typeSet(tn); len(typ) > 0 && !typ["object"] {
			return fail("relation.type_mismatch", "Target must be an object")
		}
		if r.Selection != "single" && r.Selection != "random" && r.Selection != "round-robin" {
			return fail("relation.invalid", "Invalid selection strategy")
		}
		rp := relationPlan{definition: r, source: s, target: t}
		var rr, ww [][]step
		for _, m := range r.Mappings {
			from, e := pointer(m.From)
			if e != nil {
				return fail("relation.path_invalid", e.Error())
			}
			to, e := pointer(m.To)
			if e != nil || len(to) == 0 {
				return fail("relation.path_invalid", "Invalid target pointer")
			}
			fs, fn, e := pointerSteps(sn, from)
			if e != nil {
				return fail("relation.path_missing", "Missing source mapping")
			}
			ts, tn, e := pointerSteps(tn, to)
			if e != nil {
				return fail("relation.path_missing", "Missing target mapping")
			}
			if !compatible(fn, tn) {
				return fail("relation.type_mismatch", "Incompatible mapping types")
			}
			rp.from = append(rp.from, from)
			rp.to = append(rp.to, to)
			rr = append(rr, append(append([]step{}, s...), fs...))
			write := append(append([]step{}, t...), ts...)
			for _, prev := range p.deferred {
				if e := check(); e != nil {
					return e
				}
				if overlaps(prev, write) {
					return fail("relation.target_conflict", "Overlapping relation targets")
				}
			}
			ww = append(ww, write)
			p.deferred = append(p.deferred, write)
		}
		if r.Source.UniqueBy != "" {
			rp.unique, err = pointer(r.Source.UniqueBy)
			if err != nil {
				return fail("relation.path_invalid", "Invalid uniqueBy pointer")
			}
			us, _, err := pointerSteps(sn, rp.unique)
			if err != nil {
				return fail("relation.path_missing", "Missing uniqueBy schema")
			}
			rr = append(rr, append(append([]step{}, s...), us...))
		}
		p.requiredPaths = append(p.requiredPaths, rr...)
		p.requiredPaths = append(p.requiredPaths, t)
		reads = append(reads, rr)
		writes = append(writes, ww)
		plans = append(plans, rp)
	}
	// A reads B's target => B must run before A, including partial object mappings.
	deps := make([]map[int]bool, len(plans))
	for i := range plans {
		deps[i] = map[int]bool{}
		for j := range plans {
			for _, r := range reads[i] {
				for _, w := range writes[j] {
					if e := check(); e != nil {
						return e
					}
					if overlaps(r, w) {
						deps[i][j] = true
					}
				}
			}
		}
	}
	done := map[int]bool{}
	for len(done) < len(plans) {
		progress := false
		for i, r := range plans {
			if done[i] {
				continue
			}
			ready := true
			for j := range deps[i] {
				if !done[j] {
					ready = false
				}
			}
			if ready {
				p.relations = append(p.relations, r)
				done[i] = true
				progress = true
			}
		}
		if !progress {
			return invalid("relation.cycle", "", "Cyclic relation dependencies")
		}
	}
	return nil
}
func selectNodes(root any, path []step) ([]any, error) {
	nodes := []any{root}
	for _, s := range path {
		var next []any
		for _, x := range nodes {
			if s.array {
				a, ok := x.([]any)
				if !ok {
					return nil, fmt.Errorf("expected array")
				}
				if s.wild {
					next = append(next, a...)
				} else {
					if s.index >= len(a) {
						return nil, fmt.Errorf("array index missing")
					}
					next = append(next, a[s.index])
				}
			} else {
				m, ok := x.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("expected object")
				}
				y, ok := m[s.key]
				if !ok {
					return nil, fmt.Errorf("property missing")
				}
				next = append(next, y)
			}
		}
		nodes = next
	}
	return nodes, nil
}
func (p *Plan) applyRelations(g *generation, root any) error {
	for _, r := range p.relations {
		if err := g.charge(1); err != nil {
			return err
		}
		sources, err := selectNodes(root, r.source)
		if err != nil {
			return invalid("relation.path_missing", "", "Source path does not exist")
		}
		if len(sources) == 0 {
			return invalid("relation.source_empty", "", "Source collection is empty")
		}
		targets, err := selectNodes(root, r.target)
		if err != nil {
			return invalid("relation.path_missing", "", "Target path does not exist")
		}
		if r.definition.Selection == "single" && len(sources) != 1 {
			return invalid("relation.invalid", "", "single requires exactly one source")
		}
		if len(r.unique) > 0 {
			seen := map[string]bool{}
			for _, s := range sources {
				x, e := readPointer(s, r.unique)
				if e != nil {
					return invalid("relation.path_missing", "", "uniqueBy path is missing")
				}
				k := canonical(x)
				if seen[k] {
					return invalid("relation.invalid", "", "Source uniqueBy values are not unique")
				}
				seen[k] = true
			}
		}
		for i, t := range targets {
			index := 0
			switch r.definition.Selection {
			case "random":
				index = g.random.IntN(len(sources))
			case "round-robin":
				index = i % len(sources)
			}
			source := sources[index]
			for j, from := range r.from {
				x, e := readPointer(source, from)
				if e != nil {
					return invalid("relation.path_missing", "", "Source mapping path is missing")
				}
				copy, e := g.clone(x)
				if e != nil {
					return e
				}
				if e = writePointer(t, r.to[j], copy); e != nil {
					return invalid("relation.path_missing", "", "Target mapping is not writable")
				}
			}
		}
	}
	return nil
}
