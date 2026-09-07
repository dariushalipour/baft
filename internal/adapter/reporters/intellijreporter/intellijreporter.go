package intellijreporter

import (
	"encoding/json"

	"github.com/dariushalipour/baft/internal/port"
)

// payload is the wire format read by the editor plugins: per-file diagnostics
// plus the top-level errors that abort a check, so a hard failure is visible
// instead of looking like "no violations".
type payload struct {
	Violations []port.Violation `json:"violations"`
	Errors     []string         `json:"errors"`
}

type IntelliJRenderer struct{}

func (r *IntelliJRenderer) Render(result *port.CheckResult) string {
	out := payload{Violations: []port.Violation{}, Errors: result.Errors}
	if out.Errors == nil {
		out.Errors = []string{}
	}
	for _, c := range result.Capsules {
		out.Violations = append(out.Violations, c.Violations...)
		out.Violations = append(out.Violations, c.Errors...)
	}
	b, _ := json.Marshal(out)
	return string(b) + "\n"
}
