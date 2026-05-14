package dump

import (
	"io"
	"reflect"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/ignorefs"
	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/baft/internal/adapter/languages/typescript"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

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
		&contractLoadError{cycleGroups: [][]string{{"api", "usecase", "api"}}},
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
