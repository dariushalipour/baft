package textreporter

import (
	"github.com/dariushalipour/baft/internal/port"
)

const (
	colorReset = "\033[0m"
	colorRed   = "\033[0;31m"
	colorGreen = "\033[0;32m"
)

func red(msg string) string {
	return colorRed + msg + colorReset
}

func green(msg string) string {
	return colorGreen + msg + colorReset
}

type TextRenderer struct{}

func (r *TextRenderer) Render(result *port.CheckResult) string {
	var out string
	capsuleErrors := make(map[string]bool)
	for _, c := range result.Capsules {
		for _, e := range c.Errors {
			capsuleErrors[c.Label+": "+e.Message] = true
		}
	}

	for _, e := range result.Errors {
		if capsuleErrors[e] {
			continue
		}
		out += red("✗ "+e) + "\n"
	}

	for _, c := range result.Capsules {
		if len(c.Violations) > 0 || len(c.Errors) > 0 {
			out += red("✗ "+c.Label) + "\n"
			for _, v := range c.Violations {
				out += "    " + v.Message + "\n"
			}
			for _, e := range c.Errors {
				out += "    " + e.Message + "\n"
			}
		} else {
			out += green("✓ "+c.Label) + "\n"
		}
	}

	return out
}
