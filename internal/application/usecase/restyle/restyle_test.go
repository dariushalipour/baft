package restyle

import (
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/baft/internal/port"
)

func TestRunRestylesAllContractsUnderRoot(t *testing.T) {
	const rootDir = "/Users/jane/baft"

	fsys := memfs.New()
	files := map[string]string{
		rootDir + "/BAFT.md":        "```mermaid\nflowchart TD\n  alpha[\"alpha\"]\n  beta[\"beta\"]\n\n  alpha --> beta\n```\n",
		rootDir + "/nested/BAFT.md": "```mermaid\nflowchart TD\n  gamma[\"gamma\"]:::endophobic\n```\n",
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	result, err := Run(fsys, rootDir, &mermaid.MermaidRepository{}, port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", result.Errors)
	}
	if len(result.Contracts) != 2 {
		t.Fatalf("contracts = %d, want 2", len(result.Contracts))
	}
	for _, contract := range result.Contracts {
		if !contract.Changed {
			t.Fatalf("expected %s to be restyled", contract.ContractPath)
		}
	}

	rootContent, err := fsys.ReadFile(rootDir + "/BAFT.md")
	if err != nil {
		t.Fatalf("read root BAFT.md: %v", err)
	}
	if !strings.Contains(string(rootContent), "style alpha stroke:#1f1f1f,stroke-width:2px") {
		t.Fatalf("missing root node style in:\n%s", rootContent)
	}
	if !strings.Contains(string(rootContent), "linkStyle 0 stroke:#1f1f1f,stroke-width:2px") {
		t.Fatalf("missing root link style in:\n%s", rootContent)
	}

	nestedContent, err := fsys.ReadFile(rootDir + "/nested/BAFT.md")
	if err != nil {
		t.Fatalf("read nested BAFT.md: %v", err)
	}
	if !strings.Contains(string(nestedContent), "style gamma stroke:#1f1f1f,stroke-width:2px,stroke-dasharray:5 5") {
		t.Fatalf("missing nested endophobic style in:\n%s", nestedContent)
	}
}

func TestRunWithNoneOnlyStylesEndophobicNodes(t *testing.T) {
	const rootDir = "/Users/jane/baft"

	fsys := memfs.New()
	content := "```mermaid\nflowchart TD\n  alpha[\"alpha\"]:::endophobic\n  beta[\"beta\"]\n\n  alpha --> beta\n```\n"
	if err := fsys.WriteFile(rootDir+"/BAFT.md", []byte(content), 0o644); err != nil {
		t.Fatalf("write BAFT.md: %v", err)
	}

	result, err := Run(fsys, rootDir, &mermaid.MermaidRepository{}, port.GraphSaveOptions{ColorPalette: port.ColorPaletteNone})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", result.Errors)
	}

	restyled, err := fsys.ReadFile(rootDir + "/BAFT.md")
	if err != nil {
		t.Fatalf("read BAFT.md: %v", err)
	}
	got := string(restyled)
	if !strings.Contains(got, "style alpha stroke-width:2px,stroke-dasharray:5 5") {
		t.Fatalf("missing endophobic style in:\n%s", got)
	}
	if strings.Contains(got, "style beta") {
		t.Fatalf("unexpected beta style in:\n%s", got)
	}
	if strings.Contains(got, "linkStyle ") {
		t.Fatalf("unexpected linkStyle in:\n%s", got)
	}
}

func TestRunRestylesCircularContract(t *testing.T) {
	const rootDir = "/Users/jane/baft"

	fsys := memfs.New()
	content := "```mermaid\nflowchart TD\n  alpha[\"alpha\"]\n  beta[\"beta\"]\n\n  alpha --> beta\n  beta --> alpha\n```\n"
	if err := fsys.WriteFile(rootDir+"/BAFT.md", []byte(content), 0o644); err != nil {
		t.Fatalf("write BAFT.md: %v", err)
	}

	result, err := Run(fsys, rootDir, &mermaid.MermaidRepository{}, port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", result.Errors)
	}
	if len(result.Contracts) != 1 {
		t.Fatalf("contracts = %d, want 1", len(result.Contracts))
	}
	if !result.Contracts[0].Changed {
		t.Fatal("expected circular contract to be restyled")
	}

	restyled, err := fsys.ReadFile(rootDir + "/BAFT.md")
	if err != nil {
		t.Fatalf("read BAFT.md: %v", err)
	}
	got := string(restyled)
	if !strings.Contains(got, "alpha --> beta") || !strings.Contains(got, "beta --> alpha") {
		t.Fatalf("expected cycle edges to be preserved in:\n%s", got)
	}
	if !strings.Contains(got, "style alpha stroke:#1f1f1f,stroke-width:2px") {
		t.Fatalf("missing alpha style in:\n%s", got)
	}
	if !strings.Contains(got, "style beta stroke:#2a2a2a,stroke-width:2px") {
		t.Fatalf("missing beta style in:\n%s", got)
	}
	if !strings.Contains(got, "linkStyle 0 stroke:#1f1f1f,stroke-width:2px") {
		t.Fatalf("missing first link style in:\n%s", got)
	}
	if !strings.Contains(got, "linkStyle 1 stroke:#2a2a2a,stroke-width:2px") {
		t.Fatalf("missing second link style in:\n%s", got)
	}
}

func TestRestyleContractReportsUnchangedOutput(t *testing.T) {
	repo := &mermaid.MermaidRepository{}
	content := "```mermaid\nflowchart TD\n  alpha[\"alpha\"]\n```\n"

	restyled, changed, err := RestyleContract(content, repo, port.GraphSaveOptions{ColorPalette: port.ColorPaletteVibrant})
	if err != nil {
		t.Fatalf("RestyleContract: %v", err)
	}
	if !changed {
		t.Fatalf("expected styling to be added")
	}

	restyledAgain, changedAgain, err := RestyleContract(restyled, repo, port.GraphSaveOptions{ColorPalette: port.ColorPaletteVibrant})
	if err != nil {
		t.Fatalf("RestyleContract second pass: %v", err)
	}
	if changedAgain {
		t.Fatalf("expected second pass to be unchanged")
	}
	if restyledAgain != restyled {
		t.Fatalf("second pass changed content:\n%s", restyledAgain)
	}
}

func TestRestyleContractReturnsParseError(t *testing.T) {
	repo := &mermaid.MermaidRepository{}

	_, _, err := RestyleContract("not mermaid", repo, port.GraphSaveOptions{ColorPalette: port.ColorPaletteVibrant})
	if err == nil {
		t.Fatal("expected parse error")
	}
}
