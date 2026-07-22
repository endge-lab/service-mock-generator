package http

// ErrorResponse is the standard external transport envelope for HTTP handlers.
type ErrorResponse struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// HealthResponse is returned by technical health-check endpoints.
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
	Env     string `json:"env"`
}

// VersionResponse returns the public runtime version of the service.
type VersionResponse struct {
	Service string `json:"service"`
	Version string `json:"version"`
	Env     string `json:"env"`
}
