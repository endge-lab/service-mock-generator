package generate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	errs "github.com/endge-lab/service-mock-generator/internal/domain/errors"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"github.com/endge-lab/service-mock-generator/internal/platform/schema"
	"github.com/endge-lab/service-mock-generator/internal/usecase/generate"
)

func ptr[T any](x T) *T { return &x }
func raw(s string) json.RawMessage {
	return json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema",` + s + `}`)
}
func engine() *generate.UseCase { return generate.NewUseCase(schema.NewCompiler(), v.DefaultLimits()) }
func request(s string, count int) v.Request {
	return v.Request{Schema: raw(s), Generation: v.Options{Count: ptr(count), Seed: ptr("fixture-seed")}}
}
func TestGenerationScenarios(t *testing.T) {
	cases := map[string]string{
		"null": `"type":"null"`, "boolean": `"type":"boolean"`, "integer": `"type":"integer","minimum":-7,"maximum":13`,
		"decimal":               `"type":"number","minimum":0.1,"maximum":0.9,"multipleOf":0.1`,
		"exclusive":             `"type":"number","exclusiveMinimum":0.1,"exclusiveMaximum":0.2,"multipleOf":0.01`,
		"integer-fraction-step": `"type":"integer","minimum":1,"maximum":20,"multipleOf":1.5`,
		"huge-integer":          `"type":"integer","minimum":9007199254740993,"maximum":9007199254741000`,
		"nullable":              `"type":["string","null"],"minLength":1,"maxLength":5`,
		"string":                `"type":"string","minLength":5,"maxLength":20`,
		"unicode":               `"type":"string","minLength":2,"maxLength":2,"enum":["Москва","東京","аб"]`,
		"const":                 `"const":{"x":[1,true,null]}`, "enum": `"enum":[1,"a",null,{"x":2}]`,
		"examples": `"type":"integer","minimum":2,"examples":["invalid",3]`,
		"default":  `"type":"integer","default":4`, "invalid-default": `"type":"integer","default":"bad"`,
		"object":            `"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"boolean"},"c":{"type":"string"}},"required":["a"],"minProperties":2,"maxProperties":2,"additionalProperties":false`,
		"array":             `"type":"array","items":{"type":"integer"},"minItems":2,"maxItems":8`,
		"unique":            `"type":"array","items":{"type":"integer","minimum":0,"maximum":100},"minItems":10,"maxItems":10,"uniqueItems":true`,
		"empty-array":       `"type":"array","maxItems":0`,
		"reference":         `"$defs":{"a/b":{"type":"object","properties":{"id":{"type":"string","format":"uuid"}},"required":["id"]}},"$ref":"#/$defs/a~1b"`,
		"unused-definition": `"$defs":{"Unused":{"$ref":"https://invalid.test/no-network"}},"type":"integer"`,
		"oneOf":             `"oneOf":[{"type":"string","const":"a"},{"type":"integer","minimum":0}]`,
		"ref-sibling":       `"$defs":{"N":{"type":"integer","minimum":0,"maximum":3}},"$ref":"#/$defs/N","minimum":2`,
	}
	for _, pattern := range []string{`^SU[0-9]{3,4}$`, `^[A-Z]{3}$`, `^(SU|FV)[0-9]{4}$`, `^[A-Z]{2}-[0-9]{2}$`, `^ITEM-\d{6}$`} {
		b, _ := json.Marshal(pattern)
		cases[pattern] = `"type":"string","pattern":` + string(b)
	}
	for _, format := range []string{"date", "time", "date-time", "duration", "uuid", "email", "hostname", "ipv4", "ipv6", "uri"} {
		cases[format] = `"type":"string","format":"` + format + `"`
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			u := engine()
			for i := 0; i < 10; i++ {
				r := request(s, 10)
				r.Generation.Seed = ptr(fmt.Sprint(i))
				result, err := u.Generate(context.Background(), r)
				if err != nil {
					t.Fatalf("seed %d: %v", i, err)
				}
				if len(result.Items) != 10 {
					t.Fatal("partial result")
				}
				again, err := u.Generate(context.Background(), r)
				if err != nil || !reflect.DeepEqual(result, again) {
					t.Fatal("nondeterministic generation")
				}
			}
		})
	}
}
func TestInvalidSchemas(t *testing.T) {
	cases := []string{
		`"type":"integer","minimum":2,"maximum":1`,
		`"type":"array","items":{"const":1},"minItems":2,"maxItems":2,"uniqueItems":true`,
		`"oneOf":[{"type":"integer"},{"type":"integer"}]`,
		`"type":"string","pattern":"^(a+)+$"`, `"type":"string","format":"unknown"`,
		`"$ref":"file:///etc/passwd"`, `"$ref":"https://example.com/schema"`,
		`"$defs":{"A":{"$ref":"#/$defs/A"}},"$ref":"#/$defs/A"`,
		`"$ref":"#/$defs/Missing"`, `"type":["integer","string"]`,
		`"type":"object","required":["missing"]`, `"type":"object","additionalProperties":{"type":"integer"}`,
		`"anyOf":[{"type":"string"}]`, `"type":"array","minItems":1000000,"items":{"type":"integer"}`,
		`"type":"string","minLength":1000000`, `"type":"integer","maximum":1e99999`,
		`"type":"object","properties":{"x":true}`, `"const":2,"type":"string"`,
	}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			r, e := engine().Generate(context.Background(), request(s, 2))
			if e == nil || len(r.Items) > 0 {
				t.Fatalf("expected controlled error, got %s", r.Items)
			}
		})
	}
}
func TestRelations(t *testing.T) {
	schemaText := `"type":"object","properties":{"sources":{"type":"array","minItems":2,"maxItems":2,"items":{"type":"object","properties":{"id":{"type":"string","format":"uuid"},"code":{"type":"string"}},"required":["id","code"]}},"targets":{"type":"array","minItems":5,"maxItems":5,"items":{"type":"object","properties":{"sourceId":{"type":"string"},"sourceCode":{"type":"string"}},"required":["sourceId","sourceCode"]}}},"required":["sources","targets"]`
	for _, selection := range []string{"random", "round-robin"} {
		r := request(schemaText, 20)
		r.Generation.Relations = []v.Relation{{ID: "r", Source: v.Selector{Path: "$.sources[*]", UniqueBy: "/id"}, Target: v.Target{Path: "$.targets[*]"}, Selection: selection, Mappings: []v.Mapping{{From: "/id", To: "/sourceId"}, {From: "/code", To: "/sourceCode"}}}}
		result, err := engine().Generate(context.Background(), r)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range result.Items {
			var root struct {
				Sources []struct{ ID, Code string }
				Targets []struct{ SourceID, SourceCode string }
			}
			if e := json.Unmarshal(item, &root); e != nil {
				t.Fatal(e)
			}
			for i, target := range root.Targets {
				found := false
				for _, source := range root.Sources {
					if target.SourceID == source.ID && target.SourceCode == source.Code {
						found = true
					}
				}
				if !found {
					t.Fatal("broken relation")
				}
				if selection == "round-robin" && target.SourceID != root.Sources[i%2].ID {
					t.Fatal("wrong round-robin")
				}
			}
		}
	}
}
func TestBatchGroupingAndCancellation(t *testing.T) {
	u := engine()
	r := request(`"type":"object","properties":{"x":{"type":"integer"},"s":{"type":"string"}},"required":["x","s"]`, 20)
	p, e := u.Compile(context.Background(), r.Schema, r.Generation)
	if e != nil {
		t.Fatal(e)
	}
	a, e := u.Batch(context.Background(), p, p.NewCursor(), 20)
	if e != nil {
		t.Fatal(e)
	}
	c := p.NewCursor()
	var items []json.RawMessage
	for range 20 {
		b, e := u.Batch(context.Background(), p, c, 1)
		if e != nil {
			t.Fatal(e)
		}
		items = append(items, b.Items...)
	}
	if !reflect.DeepEqual(a.Items, items) {
		t.Fatal("grouping changes sequence")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := u.Generate(ctx, r); e == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestResourceBudget(t *testing.T) {
	l := v.DefaultLimits()
	l.ResultBytes = 1024
	u := generate.NewUseCase(schema.NewCompiler(), l)
	r, e := u.Generate(context.Background(), request(`"type":"array","minItems":1000,"items":{"type":"string","minLength":10000}`, 1))
	if e == nil || len(r.Items) != 0 {
		t.Fatal("unbounded generation")
	}
	if errs.HTTPStatusOf(e) != 413 {
		t.Fatal(e)
	}
	if _, e = u.Generate(context.Background(), request(`"type":"integer"`, 1)); e != nil {
		t.Fatal("does not recover after oversized request")
	}
}
func FuzzSchema(f *testing.F) {
	for _, s := range []string{`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"integer"}`, `{}`, `null`, `{"$ref":"#/$defs/A"}`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 8192 {
			return
		}
		l := v.DefaultLimits()
		l.Nodes = 100
		l.Depth = 8
		l.ResultBytes = 8192
		l.Attempts = 5
		u := generate.NewUseCase(schema.NewCompiler(), l)
		r := v.Request{Schema: json.RawMessage(s), Generation: v.Options{Count: ptr(1), Seed: ptr("fuzz")}}
		result, e := u.Generate(context.Background(), r)
		if e == nil && (len(result.Items) != 1 || !json.Valid(result.Items[0])) {
			t.Fatal("invalid accepted output")
		}
	})
}
func TestDecode(t *testing.T) {
	for _, s := range []string{`{"schema":{},"generation":{"count":1},"ownerId":"x"}`, `{"schema":{},"generation":{"count":1,"oops":2}}`, `{} {}`, strings.Repeat("x", 200)} {
		var r v.Request
		if e := v.Decode([]byte(s), &r, 100); e == nil {
			t.Fatal("accepted invalid envelope")
		}
	}
}

func TestExtremeNegativeExponentRejectedBeforeCompilation(t *testing.T) {
	u := generate.NewUseCase(schema.NewCompiler(), v.DefaultLimits())
	count := 1
	for _, number := range []string{"1e-999999999", "1e999999999"} {
		_, err := u.Generate(context.Background(), v.Request{Schema: json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","const":` + number + `}`), Generation: v.Options{Count: &count}})
		if err == nil {
			t.Fatal("unbounded exponent accepted")
		}
	}
}

func TestOptionalRelationSourceIsGeneratedBeforeTarget(t *testing.T) {
	r := request(`"type":"object","properties":{"source":{"type":"object","properties":{"id":{"type":"integer","const":42}}},"target":{"type":"object","properties":{"ref":{"type":"integer"}}}}`, 1)
	r.Generation.OptionalPropertyProbability = ptr(0.0)
	r.Generation.Relations = []v.Relation{{ID: "optional", Source: v.Selector{Path: "$.source"}, Target: v.Target{Path: "$.target"}, Selection: "single", Mappings: []v.Mapping{{From: "/id", To: "/ref"}}}}
	result, err := engine().Generate(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Items[0]) != `{"source":{"id":42},"target":{"ref":42}}` {
		t.Fatal(string(result.Items[0]))
	}
}

func TestLiteralValuesObeyPublishedStructuralLimits(t *testing.T) {
	for _, keyword := range []string{"const", "enum", "examples", "default"} {
		for _, literal := range []any{strings.Repeat("x", 10001), make([]any, 1001)} {
			schema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", keyword: literal}
			if keyword == "enum" || keyword == "examples" {
				schema[keyword] = []any{literal}
			}
			raw, _ := json.Marshal(schema)
			if _, e := engine().Compile(context.Background(), raw, v.Options{}); e == nil {
				t.Fatalf("%s exceeds structural limits but compiled", keyword)
			}
		}
	}
}
