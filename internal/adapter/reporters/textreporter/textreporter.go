package textreporter

import (
	"fmt"
	"strings"

	"github.com/dariushalipour/baft/internal/port"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[0;33m"
)

func red(msg string) string {
	return colorRed + msg + colorReset
}

func green(msg string) string {
	return colorGreen + msg + colorReset
}

func yellow(msg string) string {
	return colorYellow + msg + colorReset
}

type TextRenderer struct{}

func (r *TextRenderer) Render(result *port.CheckResult) string {
	var out strings.Builder
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
		writeLine(&out, red("✗ "+e))
	}

	for _, w := range result.Warnings {
		writeLine(&out, yellow("⚠ "+w))
	}

	for _, c := range result.Capsules {
		line := c.Label + formatCapsuleStats(c)
		if len(c.Violations) > 0 || len(c.Errors) > 0 {
			writeLine(&out, red("✗ "+line))
			for _, v := range c.Violations {
				writeLine(&out, formatDetail("violation", v))
			}
			for _, e := range c.Errors {
				writeLine(&out, formatDetail("error", e))
			}
		} else {
			writeLine(&out, green("✓ "+line))
		}
	}

	return out.String()
}

func writeLine(out *strings.Builder, line string) {
	out.WriteString(line)
	out.WriteByte('\n')
}

func formatCapsuleStats(c port.CapsuleResult) string {
	var parts []string
	if c.FilesScanned > 0 || c.FilesEncountered > 0 {
		parts = append(parts, formatFilesScanned(c.FilesScanned, c.FilesEncountered))
	}
	if c.Relations > 0 {
		parts = append(parts, fmt.Sprintf("%d internal %s checked", c.Relations, pluralize(c.Relations, "import", "imports")))
	}
	if c.Nodes > 0 || c.Edges > 0 {
		parts = append(parts, fmt.Sprintf("graph: %d %s, %d %s", c.Nodes, pluralize(c.Nodes, "node", "nodes"), c.Edges, pluralize(c.Edges, "edge", "edges")))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func formatFilesScanned(scanned, encountered int) string {
	if scanned == encountered {
		return fmt.Sprintf("%d %s scanned", scanned, pluralize(scanned, "file", "files"))
	}
	return fmt.Sprintf("%d of %d %s scanned", scanned, encountered, pluralize(encountered, "file", "files"))
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func formatDetail(kind string, violation port.Violation) string {
	line := "    " + kind
	if violation.Rule != "" {
		line += " [" + violation.Rule + "]"
	}
	return line + ": " + violation.Message
}
