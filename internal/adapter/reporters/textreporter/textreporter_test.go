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
		Capsules: []port.CapsuleResult{{Label: "mypkg", FilesEncountered: 7, FilesScanned: 5, Nodes: 3, Edges: 4}},
	}
	out := r.Render(result)
	if !strings.Contains(out, "✓") {
		t.Error("expected checkmark in output")
	}
	if !strings.Contains(out, "mypkg") {
		t.Error("expected label in output")
	}
}

func TestRenderWithViolations(t *testing.T) {
	r := &TextRenderer{}
	result := &port.CheckResult{
		Capsules: []port.CapsuleResult{{Label: "mypkg", Violations: []port.Violation{{Message: "violation 1"}, {Message: "violation 2"}}}},
	}
	out := r.Render(result)
	if !strings.Contains(out, "✗") {
		t.Error("expected fail mark in output")
	}
	if !strings.Contains(out, "violation 1") {
		t.Error("expected violation 1 in output")
	}
	if !strings.Contains(out, "violation 2") {
		t.Error("expected violation 2 in output")
	}
}

func TestRenderWithErrors(t *testing.T) {
	r := &TextRenderer{}
	result := &port.CheckResult{
		Errors: []string{"mypkg: parse failed"},
	}
	out := r.Render(result)
	if !strings.Contains(out, "parse failed") {
		t.Error("expected error message in output")
	}
	if !strings.Contains(out, "mypkg") {
		t.Error("expected label in error output")
	}
}

func TestRenderDoesNotDuplicateCapsuleErrors(t *testing.T) {
	r := &TextRenderer{}
	result := &port.CheckResult{
		Errors: []string{"mypkg: parse failed"},
		Capsules: []port.CapsuleResult{{
			Label:  "mypkg",
			Errors: []port.Violation{{Message: "parse failed"}},
		}},
	}
	out := r.Render(result)
	if strings.Count(out, "parse failed") != 1 {
		t.Fatalf("expected parse failed once, got output: %q", out)
	}
	if !strings.Contains(out, "✗ mypkg") {
		t.Error("expected capsule section in output")
	}
}
