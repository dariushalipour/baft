package check

import (
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	golangLang "github.com/dariushalipour/baft/internal/adapter/languages/golang"
	"github.com/dariushalipour/baft/internal/port"
)

func capsuleFS(t *testing.T, contract string) port.FileSystem {
	t.Helper()

	fsys := memfs.New()
	files := map[string]string{
		"/pkg/go.mod":  "module example\n\ngo 1.21\n",
		"/pkg/BAFT.md": contract,
		"/pkg/a/a.go":  "package a\n\nimport \"example/b\"\n\nvar _ = b.V\n",
		"/pkg/b/b.go":  "package b\n\nvar V = 1\n",
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return fsys
}

func runOnePkgCapsule(t *testing.T, contract string) *port.CapsuleResult {
	t.Helper()

	res, err := RunCapsule(
		capsuleFS(t, contract),
		"/pkg",
		port.Capsule{Dir: "/pkg", CapsuleID: "example"},
		golangLang.Language{},
		&mermaid.MermaidRepository{},
		nil,
	)
	if err != nil {
		t.Fatalf("RunCapsule: %v", err)
	}
	if res == nil {
		t.Fatal("RunCapsule returned no result")
	}
	return res
}

// RunCapsule checks one known capsule without discovery, and reports the same
// per-file violations the whole-tree Run does.
func TestRunCapsule_ReportsViolationsWithoutDiscovery(t *testing.T) {
	res := runOnePkgCapsule(t, "```mermaid\nflowchart TD\n  a[\"a\"]\n  b[\"b\"]\n```\n")

	if res.Label != "/pkg" || len(res.Violations) != 1 {
		t.Fatalf("expected one violation on /pkg, got %+v", res)
	}
	if res.Violations[0].Rule != "import-not-allowed" {
		t.Fatalf("expected import-not-allowed, got %q", res.Violations[0].Rule)
	}
}

// A contract RunCapsule cannot parse is an error on the capsule, not a silent
// empty result: dump turns it into a dump error instead of amending blindly.
func TestRunCapsule_SurfacesContractLoadErrors(t *testing.T) {
	res := runOnePkgCapsule(t, "```mermaid\nflowchart TD\n  a[\"a\"] ~~> b\n```\n")

	if len(res.Errors) == 0 || res.Errors[0].Rule != "contract-load-error" {
		t.Fatalf("expected a contract-load-error, got %+v", res.Errors)
	}
}

// A node-overlap whose only witness file sits in a nested capsule is still
// reported: witnesses come from everything the walk saw, not only the files
// this capsule checks.
func TestRunCapsule_OverlapWitnessInNestedCapsule(t *testing.T) {
	fsys := memfs.New()
	for path, content := range map[string]string{
		"/pkg/go.mod":           "module example\n\ngo 1.21\n",
		"/pkg/BAFT.md":          "```mermaid\nflowchart TD\n  x[\"nested/deep\"]\n  y[\"nested/&ast;\"]\n```\n",
		"/pkg/nested/go.mod":    "module nested\n\ngo 1.21\n",
		"/pkg/nested/deep/d.go": "package deep\n",
	} {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	res, err := RunCapsule(fsys, "/pkg", port.Capsule{Dir: "/pkg", CapsuleID: "example"}, golangLang.Language{}, &mermaid.MermaidRepository{}, []string{"/pkg/nested"})
	if err != nil {
		t.Fatalf("RunCapsule: %v", err)
	}
	if res == nil || len(res.Errors) == 0 || res.Errors[0].Rule != "node-overlap" {
		t.Fatalf("expected a node-overlap error, got %+v", res)
	}
}
