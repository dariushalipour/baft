package jsonreporter

import (
	"encoding/json"

	"github.com/dariushalipour/baft/internal/port"
)

// SchemaVersion is bumped whenever the shape of the emitted document changes.
const SchemaVersion = 1

type document struct {
	SchemaVersion int `json:"schemaVersion"`
	*port.CheckResult
}

type JSONRenderer struct{}

func (r *JSONRenderer) Render(result *port.CheckResult) string {
	b, _ := json.MarshalIndent(document{SchemaVersion, result}, "", "  ")
	return string(b) + "\n"
}
