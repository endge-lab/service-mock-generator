package generate

import (
	"context"
	"encoding/json"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"github.com/endge-lab/service-mock-generator/internal/platform/schema"
	"math/rand/v2"
	"testing"
)

func FuzzPattern(f *testing.F) {
	for _, s := range []string{`^SU[0-9]{3}$`, `^(A|B)?$`, `^\d{1,5}$`, `^a*$`, `[]{}`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1000 {
			return
		}
		p, e := parsePattern(s, 1000)
		if e != nil {
			return
		}
		r := rand.New(rand.NewPCG(1, 2))
		for range 10 {
			v := p.generate(r)
			if !p.re.MatchString(v) {
				t.Fatal("pattern generated a nonmatching value")
			}
		}
	})
}
func FuzzRelations(f *testing.F) {
	for _, s := range []string{"$.source", "$", "$.x[*]", "$['x-y'][0]", "$..x", "["} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1000 {
			return
		}
		_, _ = parseSelector(s)
		_, _ = pointer(s)
		l := v.DefaultLimits()
		l.Nodes = 100
		l.Attempts = 5
		u := NewUseCase(schema.NewCompiler(), l)
		seed := "relations-fuzz"
		options := v.Options{Seed: &seed, Relations: []v.Relation{{ID: "fuzz", Source: v.Selector{Path: s}, Target: v.Target{Path: "$.target"}, Selection: "single", Mappings: []v.Mapping{{From: "/id", To: "/id"}}}}}
		plan, err := u.Compile(context.Background(), json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"source":{"type":"object","properties":{"id":{"type":"integer"}}},"target":{"type":"object","properties":{"id":{"type":"integer"}}}}}`), options)
		if err == nil {
			result, err := u.Batch(context.Background(), plan, plan.NewCursor(), 1)
			if err == nil && (len(result.Items) != 1 || !json.Valid(result.Items[0])) {
				t.Fatal("invalid relation result")
			}
		}

	})
}
