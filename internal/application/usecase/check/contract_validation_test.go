package check

import (
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/baft/internal/domain/graph"
)

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

	result := validateContractGraph(nil, nil, "/tmp/BAFT.md", mustLoadContractGraph(t, md))
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

	result := validateContractGraph(nil, nil, "/tmp/BAFT.md", mustLoadContractGraph(t, md))
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

	result := validateContractGraph(nil, nil, "/tmp/BAFT.md", mustLoadContractGraph(t, md))
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

	result := validateContractGraph(nil, nil, "/tmp/BAFT.md", mustLoadContractGraph(t, md))
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

	result := validateContractGraph(nil, nil, "/tmp/BAFT.md", mustLoadContractGraph(t, md))
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

	result := validateContractGraph(nil, nil, "/tmp/BAFT.md", mustLoadContractGraph(t, md))
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

	result := validateContractGraph(nil, nil, "/tmp/BAFT.md", mustLoadContractGraph(t, md))
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
