package dump

import (
	"reflect"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/ignorefs"
	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/languages/typescript"
)

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
