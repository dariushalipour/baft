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

// TextRenderer renders human-readable output. Color must be enabled by the
// caller: it stays off when stdout is not a terminal or NO_COLOR is set.
type TextRenderer struct{ Color bool }

func (r *TextRenderer) paint(color, msg string) string {
	if !r.Color {
		return msg
	}
	return color + msg + colorReset
}

func (r *TextRenderer) Render(result *port.CheckResult) string {
	var out strings.Builder
	// Capsule-scoped entries are printed under their capsule below.
	scoped := make(map[string]bool)
	for _, c := range result.Capsules {
		for _, v := range c.Errors {
			scoped[c.Label+": "+v.Message] = true
		}
		for _, v := range c.Violations {
			scoped[c.Label+": "+v.Message] = true
		}
	}

	for _, e := range result.Errors {
		if !scoped[e] {
			writeLine(&out, r.paint(colorRed, "✗ "+e))
		}
	}

	for _, w := range result.Warnings {
		if !scoped[w] {
			writeLine(&out, r.paint(colorYellow, "⚠ "+w))
		}
	}

	for _, c := range result.Capsules {
		line := c.Label + formatCapsuleStats(c)
		violations, warnings := splitBySeverity(c.Violations)
		switch {
		case len(violations) > 0 || len(c.Errors) > 0:
			writeLine(&out, r.paint(colorRed, "✗ "+line))
		case len(warnings) > 0:
			writeLine(&out, r.paint(colorYellow, "⚠ "+line))
		default:
			writeLine(&out, r.paint(colorGreen, "✓ "+line))
		}
		for _, v := range violations {
			writeLine(&out, formatDetail("violation", v))
		}
		for _, w := range warnings {
			writeLine(&out, r.paint(colorYellow, formatDetail("warning", w)))
		}
		for _, e := range c.Errors {
			writeLine(&out, formatDetail("error", e))
		}
	}

	return out.String()
}

// splitBySeverity partitions violations into failing ones and warnings.
func splitBySeverity(all []port.Violation) (violations, warnings []port.Violation) {
	for _, v := range all {
		if v.Severity == "warning" {
			warnings = append(warnings, v)
		} else {
			violations = append(violations, v)
		}
	}
	return violations, warnings
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
