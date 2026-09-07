package textreporter

import (
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/port"
)

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
	out := r.Render(result)
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
	out := r.Render(result)
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
			Errors:     []port.Violation{{Rule: "contract-load-error", Message: "parse failed"}},
		}},
	}
	out := r.Render(result)
	expected := strings.Join([]string{
		"✗ mypkg",
		"    violation [import-not-allowed]: violation 1",
		"    violation: violation 2",
		"    error [contract-load-error]: parse failed",
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
	out := r.Render(result)
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
			Errors: []port.Violation{{Rule: "contract-load-error", Message: "parse failed"}},
		}},
	}
	out := r.Render(result)
	expected := strings.Join([]string{
		"✗ mypkg",
		"    error [contract-load-error]: parse failed",
		"",
	}, "\n")
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestRenderColorized(t *testing.T) {
	r := &TextRenderer{Color: true}
	result := &port.CheckResult{Capsules: []port.CapsuleResult{{Label: "mypkg"}}}
	if out := r.Render(result); out != colorGreen+"✓ mypkg"+colorReset+"\n" {
		t.Fatalf("expected colorized output, got %q", out)
	}
}

func TestRenderPlainByDefault(t *testing.T) {
	r := &TextRenderer{}
	result := &port.CheckResult{Capsules: []port.CapsuleResult{{Label: "mypkg"}}}
	if out := r.Render(result); strings.Contains(out, "\033") {
		t.Fatalf("expected no ANSI escapes, got %q", out)
	}
}
