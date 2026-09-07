package check

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	golangLang "github.com/dariushalipour/baft/internal/adapter/languages/golang"
	"github.com/dariushalipour/baft/internal/domain/graph"
)

// noWitnesses stands in for the walked file list when a test has no files.
func noWitnesses(string) []string { return nil }

func mustLoadContractGraph(t *testing.T, md string) *graph.Graph {
	t.Helper()

	g, err := (&mermaid.MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return g
}

func TestValidateContractGraph_SimpleCycle(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  app["internal/application/&ast;&ast;"]` + "\n" +
		`  domain["internal/domain/&ast;&ast;"]` + "\n" +
		`  app --> domain` + "\n" +
		`  domain --> app` + "\n" +
		"```\n"

	result := validateContractGraph("/tmp/BAFT.md", mustLoadContractGraph(t, md), noWitnesses)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 cycle error, got %d", len(result.Errors))
	}
	if result.Errors[0].Rule != "circular-dependency" {
		t.Fatalf("expected circular-dependency rule, got %q", result.Errors[0].Rule)
	}
	if !strings.Contains(result.Errors[0].Message, "circular dependency: app → domain → app") {
		t.Fatalf("expected cycle message, got %q", result.Errors[0].Message)
	}
	if result.Errors[0].Line != 6 {
		t.Fatalf("expected cycle line 6, got %d", result.Errors[0].Line)
	}
}

func TestValidateContractGraph_MultipleCycles(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a["a/&ast;&ast;"]` + "\n" +
		`  b["b/&ast;&ast;"]` + "\n" +
		`  c["c/&ast;&ast;"]` + "\n" +
		`  d["d/&ast;&ast;"]` + "\n" +
		`  a --> b` + "\n" +
		`  b --> a` + "\n" +
		`  c --> d` + "\n" +
		`  d --> c` + "\n" +
		"```\n"

	result := validateContractGraph("/tmp/BAFT.md", mustLoadContractGraph(t, md), noWitnesses)
	if len(result.Errors) != 2 {
		t.Fatalf("expected 2 cycle errors, got %d", len(result.Errors))
	}
}

func TestValidateContractGraph_DuplicateCycleNotReportedTwice(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a["a/&ast;&ast;"]` + "\n" +
		`  b["b/&ast;&ast;"]` + "\n" +
		`  a --> b` + "\n" +
		`  b --> a` + "\n" +
		"```\n"

	result := validateContractGraph("/tmp/BAFT.md", mustLoadContractGraph(t, md), noWitnesses)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 cycle error, got %d", len(result.Errors))
	}
	if strings.Count(result.Errors[0].Message, "circular dependency") != 1 {
		t.Fatalf("expected a single cycle message, got %q", result.Errors[0].Message)
	}
}

func TestValidateContractGraph_EmptyGlob(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a[""]` + "\n" +
		"```\n"

	result := validateContractGraph("/tmp/BAFT.md", mustLoadContractGraph(t, md), noWitnesses)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Rule != "empty-node-glob" {
		t.Fatalf("expected empty-node-glob rule, got %q", result.Errors[0].Rule)
	}
	if !strings.Contains(result.Errors[0].Message, `node "a" has empty glob`) {
		t.Fatalf("expected empty glob message, got %q", result.Errors[0].Message)
	}
}

func TestValidateContractGraph_UndefinedEdgeNode(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  app["internal/app/&ast;&ast;"] --> domain` + "\n" +
		"```\n"

	result := validateContractGraph("/tmp/BAFT.md", mustLoadContractGraph(t, md), noWitnesses)
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Rule != "undefined-edge-node" {
		t.Fatalf("expected undefined-edge-node rule, got %q", result.Errors[0].Rule)
	}
	if !strings.Contains(result.Errors[0].Message, `edge references undefined node "domain"`) {
		t.Fatalf("expected undefined node message, got %q", result.Errors[0].Message)
	}
}

func TestValidateContractGraph_DuplicateGlob(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a["internal/x/&ast;&ast;"]` + "\n" +
		`  b["internal/x/&ast;&ast;"]` + "\n" +
		"```\n"

	result := validateContractGraph("/tmp/BAFT.md", mustLoadContractGraph(t, md), noWitnesses)
	if !result.HasDuplicateGlobError {
		t.Fatal("expected duplicate glob flag")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Rule != "duplicate-node-glob" {
		t.Fatalf("expected duplicate-node-glob rule, got %q", result.Errors[0].Rule)
	}
}

func TestValidateContractGraph_InvalidGlob(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a["../domain/&ast;&ast;"]` + "\n" +
		"```\n"

	result := validateContractGraph("/tmp/BAFT.md", mustLoadContractGraph(t, md), noWitnesses)
	if !result.HasInvalidGlobError {
		t.Fatal("expected invalid glob flag")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Rule != "invalid-node-glob" {
		t.Fatalf("expected invalid-node-glob rule, got %q", result.Errors[0].Rule)
	}
}

// TestContractOverlapErrors_NoDuplicates verifies that each overlapping node
// pair produces exactly one overlap error. Before the fix, every worker
// goroutine iterated the entire candidatePairs slice, so with N workers and
// M pairs, N×M results were produced instead of M.
func TestContractOverlapErrors_NoDuplicates(t *testing.T) {
	fsys := memfs.New()

	// Two overlapping pairs with witness files.
	if err := fsys.WriteFile("/tmp/a/b/x.go", []byte("package b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := fsys.WriteFile("/tmp/c/d/x.go", []byte("package d\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	md := "```mermaid\nflowchart TD\n" +
		`  n1["a/&ast;&ast;"]` + "\n" +
		`  n2["a/&ast;"]` + "\n" +
		`  n3["c/&ast;&ast;"]` + "\n" +
		`  n4["c/&ast;"]` + "\n" +
		"```\n"

	g := mustLoadContractGraph(t, md)
	lang := golangLang.Language{}

	result := validateContractGraph("/tmp/BAFT.md", g, func(cfgPath string) []string {
		return collectDirKeys(fsys, lang, filepath.Dir(cfgPath))
	})

	overlapCount := 0
	for _, e := range result.Errors {
		if e.Rule == "node-overlap" {
			overlapCount++
		}
	}

	if overlapCount != 2 {
		t.Fatalf("expected 2 overlap errors, got %d: %v", overlapCount, result.Errors)
	}
}

// Regression: ResolutionStrategy.IsFileGlob must NOT flag dotted namespace patterns
// (e.g., "MyApp.Api") as file globs when using namespaceResolutionStrategy.
// In path mode, "MyApp.Api" would be considered file-shaped because the last
// segment contains a dot. In namespace mode, dots are valid namespace separators.
func TestIsFileGlob_Strategy(t *testing.T) {
	tests := []struct {
		pattern       string
		namespaceMode bool
		wantFileGlob  bool
	}{
		// Path mode: standard file glob detection
		{"src/api/handler.go", false, true}, // has dot in last segment
		{"src/api/**", false, false},        // dir glob
		{"src/api/*.*", false, true},        // file glob with wildcard
		{"MyApp.Api", false, true},          // dot in last segment → file-shaped
		{".", false, false},                 // root

		// Namespace mode: dots are namespace separators, not file extensions
		{"MyApp.Api", true, false},             // valid namespace, NOT file glob
		{"MyApp.Domain.Entities", true, false}, // valid namespace, NOT file glob
		{"MyApp", true, false},                 // single-segment namespace
		{"src/api/**", true, true},             // has "/" → file path glob
		{"Api/*.*", true, true},                // has "/" and "*" → file glob
		{"MyApp.Api.*", true, false},           // namespace wildcard, matched by the graph index
		{".", true, false},                     // root
	}

	pathStrat := &pathResolutionStrategy{}
	nsStrat := &namespaceResolutionStrategy{}

	for _, tc := range tests {
		var got bool
		if tc.namespaceMode {
			got = nsStrat.IsFileGlob(tc.pattern)
		} else {
			got = pathStrat.IsFileGlob(tc.pattern)
		}
		if got != tc.wantFileGlob {
			t.Errorf("IsFileGlob(%q, namespaceMode=%v) = %v, want %v",
				tc.pattern, tc.namespaceMode, got, tc.wantFileGlob)
		}
	}
}
