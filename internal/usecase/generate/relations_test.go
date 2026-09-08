package generate_test

import (
	"context"
	"encoding/json"
	"fmt"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"runtime"
	"testing"
	"time"
)

func relation(id, from, to string) v.Relation {
	return v.Relation{ID: id, Source: v.Selector{Path: from}, Target: v.Target{Path: to}, Selection: "single", Mappings: []v.Mapping{{From: "/id", To: "/id"}}}
}
func TestRelationDependenciesAndFailures(t *testing.T) {
	text := `"type":"object","properties":{"a":{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]},"b":{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]},"c":{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}},"required":["a","b","c"]`
	for seed := range 20 {
		r := request(text, 5)
		r.Generation.Seed = ptr(fmt.Sprint(seed))
		r.Generation.Relations = []v.Relation{relation("second", "$.b", "$.c"), relation("first", "$.a", "$.b")}
		result, e := engine().Generate(context.Background(), r)
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range result.Items {
			var x map[string]map[string]int
			json.Unmarshal(item, &x)
			if x["a"]["id"] != x["b"]["id"] || x["a"]["id"] != x["c"]["id"] {
				t.Fatal("dependency order violated", string(item))
			}
		}
	}
	for name, relations := range map[string][]v.Relation{
		"cycle":     {relation("one", "$.a", "$.b"), relation("two", "$.b", "$.a")},
		"self":      {relation("one", "$.a", "$.a")},
		"conflict":  {relation("one", "$.a", "$.b"), relation("two", "$.c", "$.b")},
		"missing":   {relation("one", "$.missing", "$.b")},
		"duplicate": {relation("same", "$.a", "$.b"), relation("same", "$.a", "$.c")},
	} {
		t.Run(name, func(t *testing.T) {
			r := request(text, 1)
			r.Generation.Relations = relations
			if x, e := engine().Generate(context.Background(), r); e == nil || len(x.Items) != 0 {
				t.Fatal("invalid relation produced output")
			}
		})
	}
}

func TestCancelDuringActiveGeneration(t *testing.T) {
	u := engine()
	r := request(`"type":"array","minItems":1000,"maxItems":1000,"items":{"type":"array","minItems":1000,"maxItems":1000,"items":{"type":"string","minLength":100,"maxLength":100}}`, 1)
	plan, e := u.Compile(context.Background(), r.Schema, r.Generation)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		x, e := u.Batch(ctx, plan, plan.NewCursor(), 1)
		if len(x.Items) != 0 {
			done <- fmt.Errorf("partial output after cancel")
			return
		}
		done <- e
	}()
	deadline := time.Now().Add(time.Second)
	for u.ActiveJobs() == 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if u.ActiveJobs() == 0 {
		t.Fatal("generation did not start")
	}
	cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancel ignored")
		}
	case <-time.After(time.Second):
		t.Fatal("generation did not cancel")
	}
	if u.ActiveJobs() != 0 {
		t.Fatal("job leaked")
	}
	if _, e = u.Generate(context.Background(), request(`"type":"integer"`, 1)); e != nil {
		t.Fatal("capacity not restored", e)
	}
}
