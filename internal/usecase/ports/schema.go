package ports

import "encoding/json"

// SchemaValidator owns library-specific compiled validation state.
type SchemaValidator interface {
	Validate(path string, value any) error
}
type SchemaCompiler interface {
	Compile(document json.RawMessage) (SchemaValidator, error)
}
