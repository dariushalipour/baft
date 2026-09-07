package check

import (
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	golangLang "github.com/dariushalipour/baft/internal/adapter/languages/golang"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

// A contract mistake is a per-file diagnostic, not a failure of the check
// itself: the editor plugins read CheckResult.Errors as "the run aborted" and
// stop publishing diagnostics when it is non-empty.
func TestRun_ContractErrorsStayOutOfTopLevelErrors(t *testing.T) {
	fsys := memfs.New()
	_ = fsys.WriteFile("/pkg/go.mod", []byte("module example\n\ngo 1.21\n"), 0o644)
	_ = fsys.WriteFile("/pkg/main.go", []byte("package main\n"), 0o644)
	_ = fsys.WriteFile("/pkg/BAFT.md", []byte("```mermaid\nflowchart TD\n  app[\"main/&ast;&ast;\"] --> ghost\n```\n"), 0o644)

	discovery := service.NewCapsuleDiscovery()
	golangLang.Language{}.Register(discovery)
	result := Run(fsys, "/", []port.Language{golangLang.Language{}}, &mermaid.MermaidRepository{}, discovery)

	if len(result.Errors) != 0 {
		t.Fatalf("expected no aborting errors, got %v", result.Errors)
	}
	if len(result.Capsules) != 1 || len(result.Capsules[0].Errors) == 0 {
		t.Fatalf("expected the contract error on the capsule, got %+v", result.Capsules)
	}
	if !result.Failed() {
		t.Fatal("expected a contract error to fail the check")
	}
}
