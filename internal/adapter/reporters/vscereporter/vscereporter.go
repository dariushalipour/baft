package vscereporter

import (
	"encoding/json"

	"github.com/dariushalipour/strata/internal/port"
)

type VSCERenderer struct{}

func (r *VSCERenderer) Render(result *port.CheckResult) string {
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
