package entities

import (
	"encoding/json"
	v "github.com/endge-lab/service-mock-generator/internal/domain/valueobjects"
	"time"
)

type Stream struct {
	ID                string       `json:"id"`
	Status            string       `json:"status"`
	Seed              string       `json:"seed"`
	Profile           string       `json:"profile"`
	Parameters        v.Parameters `json:"parameters"`
	ParametersVersion uint64       `json:"parametersVersion"`
	Sequence          uint64       `json:"sequence"`
	IdleTimeoutMs     int64        `json:"idleTimeoutMs"`
	ExpiresAt         time.Time    `json:"expiresAt"`
}
type Event struct {
	Type              string            `json:"type"`
	StreamID          string            `json:"streamId"`
	Sequence          uint64            `json:"sequence,omitempty"`
	ParametersVersion uint64            `json:"parametersVersion,omitempty"`
	Items             []json.RawMessage `json:"items,omitempty"`
	ErrorCode         string            `json:"errorCode,omitempty"`
	ErrorMessage      string            `json:"errorMessage,omitempty"`
}
