package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/endge-lab/service-mock-generator/internal/usecase/ports"
	js "github.com/santhosh-tekuri/jsonschema/v6"
	"sync"
)

type Compiler struct{}

func NewCompiler() *Compiler { return &Compiler{} }

type denyLoader struct{}

func (denyLoader) Load(string) (any, error) {
	return nil, fmt.Errorf("external schema loading is disabled")
}

type validator struct {
	mu       sync.Mutex
	compiler *js.Compiler
	nodes    map[string]*js.Schema
}

const resource = "urn:endge:mock-schema"

func (*Compiler) Compile(raw json.RawMessage) (ports.SchemaValidator, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var doc any
	if err := d.Decode(&doc); err != nil {
		return nil, err
	}
	c := js.NewCompiler()
	c.DefaultDraft(js.Draft2020)
	c.AssertFormat()
	c.UseLoader(denyLoader{})
	if err := c.AddResource(resource, doc); err != nil {
		return nil, err
	}
	root, err := c.Compile(resource)
	if err != nil {
		return nil, err
	}
	return &validator{compiler: c, nodes: map[string]*js.Schema{"": root}}, nil
}
func (v *validator) Validate(path string, value any) error {
	v.mu.Lock()
	s := v.nodes[path]
	if s == nil {
		var err error
		s, err = v.compiler.Compile(resource + "#" + path)
		if err != nil {
			v.mu.Unlock()
			return err
		}
		v.nodes[path] = s
	}
	v.mu.Unlock()
	return s.Validate(value)
}
