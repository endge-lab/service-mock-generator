package generate_test

import (
	"bytes"
	"context"
	"encoding/json"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultV1Golden(t *testing.T) {
	schemas := []string{
		`"type":"object","properties":{"id":{"type":"string","format":"uuid"},"at":{"type":"string","format":"date-time"},"n":{"type":"number","multipleOf":0.25},"code":{"type":"string","pattern":"^(SU|FV)[0-9]{4}$"},"optional":{"type":["boolean","null"]}},"required":["id","at","n","code"]`,
		`"type":"array","minItems":2,"maxItems":5,"uniqueItems":true,"items":{"type":"integer","minimum":1,"maximum":100}`,
		`"oneOf":[{"type":"string","enum":["東京","Москва"]},{"type":"number","exclusiveMinimum":0.1,"exclusiveMaximum":0.11}]`,
		`"$defs":{"T":{"type":"object","properties":{"x":{"type":"integer","default":7},"y":{"type":"boolean","examples":[true]}}}},"$ref":"#/$defs/T"`,
	}
	var got []v.Result
	for _, s := range schemas {
		result, e := engine().Generate(context.Background(), request(s, 5))
		if e != nil {
			t.Fatal(e)
		}
		got = append(got, result)
	}
	raw, e := json.MarshalIndent(got, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	raw = append(raw, '\n')
	path := filepath.Join("testdata", "default-v1.json")
	if os.Getenv("UPDATE_MOCK_GOLDEN") == "1" {
		if e = os.WriteFile(path, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	expected, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(raw, expected) {
		t.Fatal("default-v1 sequence changed; compatible profiles must preserve golden values")
	}
}
