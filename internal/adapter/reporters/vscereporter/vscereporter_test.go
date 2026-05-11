package vscereporter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/port"
)

func TestRenderEmpty(t *testing.T) {
	r := &VSCERenderer{}
	out := r.Render(&port.CheckResult{})
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("expected [], got: %q", out)
	}
}

func TestRenderWithViolations(t *testing.T) {
	r := &VSCERenderer{}
	result := &port.CheckResult{
		Capsules: []port.CapsuleResult{
			{
				Label: "auth",
				Violations: []port.Violation{
					{
						Rule:      "import-not-allowed",
						Severity:  "error",
						Source:    "baft",
						Message:   "auth cannot import billing",
						File:      "/repo/auth/service.go",
						Line:      12,
						Column:    5,
						ColumnEnd: 20,
					},
				},
			},
		},
	}
	out := r.Render(result)

	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON array, got error: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(parsed))
	}
	v := parsed[0]
	checks := map[string]string{
		"rule":     "import-not-allowed",
		"severity": "error",
		"source":   "baft",
		"message":  "auth cannot import billing",
		"file":     "/repo/auth/service.go",
	}
	for field, want := range checks {
		if got, ok := v[field].(string); !ok || got != want {
			t.Errorf("field %q: want %q, got %v", field, want, v[field])
		}
	}
	if line, ok := v["line"].(float64); !ok || int(line) != 12 {
		t.Errorf("field line: want 12, got %v", v["line"])
	}
	if col, ok := v["column"].(float64); !ok || int(col) != 5 {
		t.Errorf("field column: want 5, got %v", v["column"])
	}
	if colEnd, ok := v["columnEnd"].(float64); !ok || int(colEnd) != 20 {
		t.Errorf("field columnEnd: want 20, got %v", v["columnEnd"])
	}
}

func TestRenderMultipleCapsules(t *testing.T) {
	r := &VSCERenderer{}
	result := &port.CheckResult{
		Capsules: []port.CapsuleResult{
			{Label: "auth", Violations: []port.Violation{{Message: "v1"}}},
			{Label: "billing", Violations: []port.Violation{{Message: "v2"}, {Message: "v3"}}},
		},
	}
	out := r.Render(result)

	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON array, got error: %v", err)
	}
	if len(parsed) != 3 {
		t.Errorf("expected 3 violations flattened, got %d", len(parsed))
	}
}

func TestRenderColumnEndOmittedWhenZero(t *testing.T) {
	r := &VSCERenderer{}
	result := &port.CheckResult{
		Capsules: []port.CapsuleResult{
			{Violations: []port.Violation{{Message: "v1", Column: 3}}},
		},
	}
	out := r.Render(result)

	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if _, present := parsed[0]["columnEnd"]; present {
		t.Errorf("expected columnEnd to be absent when zero, but it was present")
	}
}

func TestRenderIncludesErrors(t *testing.T) {
	r := &VSCERenderer{}
	result := &port.CheckResult{
		Capsules: []port.CapsuleResult{
			{
				Label:      "auth",
				Violations: []port.Violation{{Rule: "import-not-allowed", Message: "v1"}},
				Errors:     []port.Violation{{Rule: "file-glob-unsupported", Message: "e1", File: "/repo/BAFT.md", Line: 3}},
			},
		},
	}
	out := r.Render(result)

	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON array, got error: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 diagnostics (1 violation + 1 error), got %d", len(parsed))
	}
	rules := map[string]bool{}
	for _, d := range parsed {
		if r, ok := d["rule"].(string); ok {
			rules[r] = true
		}
	}
	if !rules["import-not-allowed"] || !rules["file-glob-unsupported"] {
		t.Errorf("expected both rules in output, got: %v", rules)
	}
}
