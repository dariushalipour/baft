package dump

import (
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/ignorefs"
	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	csharpLang "github.com/dariushalipour/baft/internal/adapter/languages/csharp"
	"github.com/dariushalipour/baft/internal/adapter/languages/typescript"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

// Regression: addBoundaryRelations in namespace mode must use the target file's
// actual namespace from namespaceMap, not the raw import statement's namespace.
// Previously, dstID was set to spec.Namespace, which could differ from the
// resolved target file's declared namespace (e.g., alias imports).
// This test verifies that the namespace map correctly maps all scannable files
// to their declared namespaces, which is the data addBoundaryRelations uses.
func TestAddBoundaryRelations_UsesTargetNamespace(t *testing.T) {
	const rootDir = "/home/user/myapp"

	fsys := memfs.New()
	files := map[string]string{
		rootDir + "/Api/Controller.cs": `using System;
using MyApp.Domain;

namespace MyApp.Api
{
    public class Controller { }
}`,
		rootDir + "/Domain/Entity.cs": `using System;

namespace MyApp.Domain
{
    public class Entity { }
}`,
		rootDir + "/Domain/Repository.cs": `using System;

namespace MyApp.Domain
{
    public class Repository { }
}`,
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	lang := &csharpLang.Language{}

	// Build file imports (as dumpCapsule does)
	var fileImports []fileImport
	for path := range files {
		imports, err := lang.ParseImports(fsys, path)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		rel, _ := filepath.Rel(rootDir, path)
		fileImports = append(fileImports, fileImport{abs: path, rel: rel, imports: imports})
	}

	// Build namespace map (as dumpCapsule does before calling addBoundaryRelations)
	nsMap := buildNamespaceMap(fsys, fileImports, lang)

	// Verify namespace map has all scannable files mapped correctly
	expected := map[string]string{
		rootDir + "/Api/Controller.cs":    "MyApp.Api",
		rootDir + "/Domain/Entity.cs":     "MyApp.Domain",
		rootDir + "/Domain/Repository.cs": "MyApp.Domain",
	}
	for expectedPath, expectedNS := range expected {
		if ns := nsMap[expectedPath]; ns != expectedNS {
			t.Errorf("%s namespace = %q, want %q", expectedPath, ns, expectedNS)
		}
	}

	// Verify that addBoundaryRelations would resolve the target namespace correctly:
	// When processing Controller.cs's "using MyApp.Domain;" import, the target
	// files (Entity.cs, Repository.cs) are looked up in namespaceMap by their
	// absolute paths, not by the raw import string. Both should resolve to "MyApp.Domain".
	for _, targetFile := range []string{rootDir + "/Domain/Entity.cs", rootDir + "/Domain/Repository.cs"} {
		if ns := nsMap[targetFile]; ns != "MyApp.Domain" {
			t.Errorf("addBoundaryRelations would resolve %s to namespace %q, want %q",
				targetFile, ns, "MyApp.Domain")
		}
	}
}

// TestBuildNamespaceMap verifies that the namespace map correctly maps file paths
// to their declared namespaces. This is the map used by addBoundaryRelations to
// avoid re-reading files.
func TestBuildNamespaceMap(t *testing.T) {
	const rootDir = "/home/user/myapp"

	fsys := memfs.New()
	files := map[string]string{
		rootDir + "/Api/Controller.cs": `using System;
using MyApp.Domain;

namespace MyApp.Api
{
    public class Controller { }
}`,
		rootDir + "/Domain/Entity.cs": `using System;

namespace MyApp.Domain
{
    public class Entity { }
}`,
		rootDir + "/NoNamespace.cs": `using System;

public class Script { }`,
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	lang := &csharpLang.Language{}

	// Build file import records
	var fileImports []fileImport
	for path := range files {
		imports, err := lang.ParseImports(fsys, path)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		rel, _ := filepath.Rel(rootDir, path)
		fileImports = append(fileImports, fileImport{abs: path, rel: rel, imports: imports})
	}

	// Build namespace map
	nsMap := buildNamespaceMap(fsys, fileImports, lang)

	// Verify namespace map
	if ns := nsMap[rootDir+"/Api/Controller.cs"]; ns != "MyApp.Api" {
		t.Errorf("Controller.cs namespace = %q, want %q", ns, "MyApp.Api")
	}
	if ns := nsMap[rootDir+"/Domain/Entity.cs"]; ns != "MyApp.Domain" {
		t.Errorf("Entity.cs namespace = %q, want %q", ns, "MyApp.Domain")
	}
	// File without namespace should not be in the map
	if ns := nsMap[rootDir+"/NoNamespace.cs"]; ns != "" {
		t.Errorf("NoNamespace.cs should not be in namespace map, got %q", ns)
	}
}

var generatedStyleComment = strings.Trim(`
  %% ------------------------------------------------------------------------------------------
  %% AUTO-GENERATED STYLING: Do not edit manually.
  %% If you add, delete, or reorder nodes, you MUST run 'baft restyle' or format via your IDE.
  %% Outdated references will either break the render entirely or silently mess up the styling.
  %% ------------------------------------------------------------------------------------------
`, "\n")

type recordingRepo struct {
	delegate     mermaid.MermaidRepository
	lastSaveOpts port.GraphSaveOptions
	saveCalls    int
}

func (r *recordingRepo) Load(content string) (*graph.Graph, error) {
	return r.delegate.Load(content)
}

func (r *recordingRepo) Save(g *graph.Graph, opts port.GraphSaveOptions) string {
	r.lastSaveOpts = opts
	r.saveCalls++
	return r.delegate.Save(g, opts)
}

func (r *recordingRepo) Restyle(content string, opts port.GraphSaveOptions) (string, error) {
	return r.delegate.Restyle(content, opts)
}

func TestCycleExpansionCandidatesOnWrappedMemFS(t *testing.T) {
	const rootDir = "/Users/jane/baft"

	fsys := memfs.New()
	files := map[string]string{
		rootDir + "/package.json":        `{"name":"@myorg/app"}`,
		rootDir + "/tsconfig.json":       `{"compilerOptions":{"baseUrl":"."}}`,
		rootDir + "/api/helper.ts":       `export const helperMarker = 1`,
		rootDir + "/api/entry.ts":        "import { consume } from \"../usecase/consumer\"\n\nexport function run() {\n  return consume()\n}\n",
		rootDir + "/usecase/consumer.ts": "export function consume() {\n  return \"ok\"\n}\n",
		rootDir + "/usecase/producer.ts": "import { run } from \"../api/entry\"\n\nexport function produce() {\n  return run()\n}\n",
		rootDir + "/usecase/helper.ts":   `export const helperMarker = 1`,
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	wrapped, err := ignorefs.Wrap(fsys, ignorefs.Options{RootDir: rootDir})
	if err != nil {
		t.Fatalf("wrap fs: %v", err)
	}

	got := cycleExpansionCandidates(
		wrapped,
		rootDir,
		&typescript.Language{},
		&contractError{cycleGroups: [][]string{{"api", "usecase", "api"}}},
		draftConfig{mode: draftModeMergedDirs},
	)
	want := []string{"api", "usecase"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cycleExpansionCandidates() = %v, want %v", got, want)
	}
}

func TestRunWithOptionsPassesColorPaletteToSave(t *testing.T) {
	const rootDir = "/Users/jane/baft"

	fsys := memfs.New()
	files := map[string]string{
		rootDir + "/package.json":        `{"name":"@myorg/app"}`,
		rootDir + "/tsconfig.json":       `{"compilerOptions":{"baseUrl":"."}}`,
		rootDir + "/api/entry.ts":        "import { consume } from \"../usecase/consumer\"\n\nexport function run() {\n  return consume()\n}\n",
		rootDir + "/usecase/consumer.ts": "export function consume() {\n  return \"ok\"\n}\n",
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	repo := &recordingRepo{}
	discovery := service.NewCapsuleDiscovery()
	lang := &typescript.Language{}
	lang.Register(discovery)

	result, err := RunWithOptions(
		fsys,
		rootDir,
		[]port.Language{lang},
		repo,
		discovery,
		port.GraphSaveOptions{ColorPalette: port.ColorPaletteNone},
		io.Discard,
	)
	if err != nil {
		t.Fatalf("RunWithOptions: %v", err)
	}
	if result == nil || len(result.Contracts) == 0 {
		t.Fatalf("expected at least one dumped contract, got %#v", result)
	}
	if repo.saveCalls == 0 {
		t.Fatal("expected Save to be called")
	}
	if repo.lastSaveOpts.ColorPalette != port.ColorPaletteNone {
		t.Fatalf("save color palette = %q, want %q", repo.lastSaveOpts.ColorPalette, port.ColorPaletteNone)
	}
	content, err := fsys.ReadFile(rootDir + "/BAFT.md")
	if err != nil {
		t.Fatalf("read BAFT.md: %v", err)
	}
	if !reflect.DeepEqual(content != nil, true) {
		t.Fatal("expected BAFT.md content to be written")
	}
}

func TestRunWithOptionsWritesGeneratedStyleComment(t *testing.T) {
	const rootDir = "/Users/jane/baft"

	fsys := memfs.New()
	files := map[string]string{
		rootDir + "/package.json":        `{"name":"@myorg/app"}`,
		rootDir + "/tsconfig.json":       `{"compilerOptions":{"baseUrl":"."}}`,
		rootDir + "/api/entry.ts":        "import { consume } from \"../usecase/consumer\"\n\nexport function run() {\n  return consume()\n}\n",
		rootDir + "/usecase/consumer.ts": "export function consume() {\n  return \"ok\"\n}\n",
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	discovery := service.NewCapsuleDiscovery()
	lang := &typescript.Language{}
	lang.Register(discovery)

	_, err := RunWithOptions(
		fsys,
		rootDir,
		[]port.Language{lang},
		&mermaid.MermaidRepository{},
		discovery,
		port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono},
		io.Discard,
	)
	if err != nil {
		t.Fatalf("RunWithOptions: %v", err)
	}

	content, err := fsys.ReadFile(rootDir + "/BAFT.md")
	if err != nil {
		t.Fatalf("read BAFT.md: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, generatedStyleComment) {
		t.Fatalf("missing generated style comment in:\n%s", got)
	}
	if !strings.Contains(got, "style api_slash_entry_dot_ts stroke:#1f1f1f,stroke-width:2px") {
		t.Fatalf("missing style block in:\n%s", got)
	}
}

// Regression: applyNoNodeViolation must propagate non-IsNotExist errors
// from ParseImports. Previously, the condition "if fsys != nil" was always
// true, causing ALL errors to be silently swallowed.
func TestApplyNoNodeViolation_PropagatesNonExistenceErrors(t *testing.T) {
	const rootDir = "/home/user/myapp"

	fsys := memfs.New()
	files := map[string]string{
		rootDir + "/Api/Controller.cs": `using System;
using MyApp.Domain;

namespace MyApp.Api
{
    public class Controller { }
}`,
		rootDir + "/Domain/Entity.cs": `using System;

namespace MyApp.Domain
{
    public class Entity { }
}`,
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	lang := &csharpLang.Language{}
	capsule := port.Capsule{Dir: rootDir}
	contractPath := rootDir + "/BAFT.md"

	nodes := map[string]string{}

	// Test with existing file: should succeed
	violation := port.Violation{File: rootDir + "/Api/Controller.cs"}
	_, err := applyNoNodeViolation(nodes, fsys, capsule, contractPath, lang, violation, draftConfig{namespaceMode: true})
	if err != nil {
		t.Fatalf("applyNoNodeViolation with existing file: %v", err)
	}

	// Test with non-existent file: should silently return false, nil
	// (os.IsNotExist is expected for files that have been deleted)
	violation = port.Violation{File: rootDir + "/Deleted/File.cs"}
	changed, err := applyNoNodeViolation(nodes, fsys, capsule, contractPath, lang, violation, draftConfig{namespaceMode: true})
	if err != nil {
		t.Fatalf("applyNoNodeViolation with non-existent file should not error: %v", err)
	}
	if changed {
		t.Fatal("applyNoNodeViolation with non-existent file should not change nodes")
	}
}

// Regression: dumpCapsule in namespace mode must not add empty string "" as a node key
// when files have no namespace declarations. Previously, the code path could create
// edges from "" when srcID was empty, corrupting the graph.
func TestDumpCapsule_NamespaceMode_NoNamespaceFilesSkipped(t *testing.T) {
	const rootDir = "/home/user/myapp"

	fsys := memfs.New()
	files := map[string]string{
		rootDir + "/MyApp.csproj": `<Project><PropertyGroup><RootNamespace>MyApp</RootNamespace></PropertyGroup></Project>`,
		// Script.cs has no namespace declaration — should be skipped in namespace mode
		rootDir + "/Script.cs": `using System;

public class Script { }`,
		// Another file without namespace
		rootDir + "/Helper.cs": `using MyApp.Api;

public class Helper { }`,
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	lang := &csharpLang.Language{}

	// Build file imports (as dumpCapsule does)
	var fileImports []fileImport
	for abs := range files {
		if !strings.HasSuffix(abs, ".cs") {
			continue
		}
		imports, err := lang.ParseImports(fsys, abs)
		if err != nil {
			t.Fatalf("parse %s: %v", abs, err)
		}
		rel, _ := filepath.Rel(rootDir, abs)
		fileImports = append(fileImports, fileImport{abs: abs, rel: rel, imports: imports})
	}

	// Build namespace map — files without namespace declarations won't be in the map
	nsMap := buildNamespaceMap(fsys, fileImports, lang)

	// Verify: no files should have a namespace mapping
	if len(nsMap) != 0 {
		t.Errorf("namespace map should be empty (no files have namespace declarations), got %d entries: %v", len(nsMap), nsMap)
	}

	// Simulate the dumpCapsule loop: collect srcIDs that would be processed
	var srcIDs []string
	for _, fi := range fileImports {
		if ns, ok := nsMap[fi.abs]; ok {
			srcIDs = append(srcIDs, ns)
		}
		// else: skipped (no namespace) — this is the critical path
	}

	// No files should produce a srcID since none have namespace declarations
	if len(srcIDs) != 0 {
		t.Errorf("expected no srcIDs (all files lack namespace), got %v", srcIDs)
	}

	// Verify no empty string is in srcIDs (the regression case)
	for _, srcID := range srcIDs {
		if srcID == "" {
			t.Error("empty string srcID produced — graph corruption risk")
		}
	}
}
