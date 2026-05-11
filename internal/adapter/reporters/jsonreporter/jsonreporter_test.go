package jsonreporter

import (
	"encoding/json"
	"testing"

	"github.com/dariushalipour/baft/internal/port"
)

func TestRenderEmpty(t *testing.T) {
	r := &JSONRenderer{}
	out := r.Render(&port.CheckResult{})
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
}

func TestRenderWithCapsules(t *testing.T) {
	r := &JSONRenderer{}
	result := &port.CheckResult{
		Capsules: []port.CapsuleResult{{Label: "mypkg", FilesEncountered: 7, FilesScanned: 5, Nodes: 3, Edges: 4}},
	}
	out := r.Render(result)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if _, ok := parsed["capsules"]; !ok {
		t.Error("expected capsules in output")
	}
}

func TestRenderWithViolations(t *testing.T) {
	r := &JSONRenderer{}
	result := &port.CheckResult{
		Capsules: []port.CapsuleResult{{Label: "mypkg", Violations: []port.Violation{{Message: "violation 1"}}}},
	}
	out := r.Render(result)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if _, ok := parsed["capsules"]; !ok {
		t.Error("expected capsules in output")
	}
}

func TestRenderWithErrors(t *testing.T) {
	r := &JSONRenderer{}
	result := &port.CheckResult{
		Errors: []string{"mypkg: parse failed"},
	}
	out := r.Render(result)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if _, ok := parsed["errors"]; !ok {
		t.Error("expected errors in output")
	}
}
