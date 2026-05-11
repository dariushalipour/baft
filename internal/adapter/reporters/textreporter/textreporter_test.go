package textreporter

import (
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/port"
)

func stripANSI(s string) string {
	s = strings.ReplaceAll(s, colorRed, "")
	s = strings.ReplaceAll(s, colorGreen, "")
	return strings.ReplaceAll(s, colorReset, "")
}

func TestRenderEmpty(t *testing.T) {
	r := &TextRenderer{}
	out := r.Render(&port.CheckResult{})
	if len(strings.TrimSpace(out)) != 0 {
		t.Errorf("expected empty output, got: %q", out)
	}
}

func TestRenderNoViolations(t *testing.T) {
	r := &TextRenderer{}
	result := &port.CheckResult{
		Capsules: []port.CapsuleResult{{Label: "mypkg", FilesEncountered: 7, FilesScanned: 5, Nodes: 3, Edges: 4, Relations: 9}},
	}
	out := stripANSI(r.Render(result))
	expected := "✓ mypkg (5 of 7 files scanned, 9 internal imports checked, graph: 3 nodes, 4 edges)\n"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestRenderPluralizesStats(t *testing.T) {
	r := &TextRenderer{}
	result := &port.CheckResult{
		Capsules: []port.CapsuleResult{{Label: "mypkg", FilesEncountered: 1, FilesScanned: 1, Nodes: 1, Edges: 1, Relations: 1}},
	}
	out := stripANSI(r.Render(result))
	expected := "✓ mypkg (1 file scanned, 1 internal import checked, graph: 1 node, 1 edge)\n"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestRenderWithViolations(t *testing.T) {
	r := &TextRenderer{}
	result := &port.CheckResult{
		Capsules: []port.CapsuleResult{{
			Label:      "mypkg",
			Violations: []port.Violation{{Rule: "import-not-allowed", Message: "violation 1"}, {Message: "violation 2"}},
			Errors:     []port.Violation{{Rule: "config-load-error", Message: "parse failed"}},
		}},
	}
	out := stripANSI(r.Render(result))
	expected := strings.Join([]string{
		"✗ mypkg",
		"    violation [import-not-allowed]: violation 1",
		"    violation: violation 2",
		"    error [config-load-error]: parse failed",
		"",
	}, "\n")
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestRenderWithErrors(t *testing.T) {
	r := &TextRenderer{}
	result := &port.CheckResult{
		Errors: []string{"mypkg: parse failed"},
	}
	out := stripANSI(r.Render(result))
	expected := "✗ mypkg: parse failed\n"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestRenderDoesNotDuplicateCapsuleErrors(t *testing.T) {
	r := &TextRenderer{}
	result := &port.CheckResult{
		Errors: []string{"mypkg: parse failed"},
		Capsules: []port.CapsuleResult{{
			Label:  "mypkg",
			Errors: []port.Violation{{Rule: "config-load-error", Message: "parse failed"}},
		}},
	}
	out := stripANSI(r.Render(result))
	expected := strings.Join([]string{
		"✗ mypkg",
		"    error [config-load-error]: parse failed",
		"",
	}, "\n")
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}
