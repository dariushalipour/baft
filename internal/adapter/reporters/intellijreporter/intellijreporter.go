package intellijreporter

import (
	"encoding/json"

	"github.com/dariushalipour/baft/internal/port"
)

type IntelliJRenderer struct{}

func (r *IntelliJRenderer) Render(result *port.CheckResult) string {
	var diagnostics []port.Violation
	for _, c := range result.Capsules {
		diagnostics = append(diagnostics, c.Violations...)
		diagnostics = append(diagnostics, c.Errors...)
	}
	if diagnostics == nil {
		return "[]\n"
	}
	b, _ := json.Marshal(diagnostics)
	return string(b) + "\n"
}
