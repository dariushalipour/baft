package jsonreporter

import (
	"encoding/json"

	"github.com/dariushalipour/strata/internal/port"
)

type JSONRenderer struct{}

func (r *JSONRenderer) Render(result *port.CheckResult) string {
	b, _ := json.MarshalIndent(result, "", "  ")
	return string(b) + "\n"
}
