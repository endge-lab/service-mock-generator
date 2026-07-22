package docs

import _ "embed"

//go:embed openapi3.yaml
var openAPI3YAML []byte

func OpenAPI3YAML() []byte {
	return openAPI3YAML
}
