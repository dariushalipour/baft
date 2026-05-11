package steps

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cucumber/godog"
	"github.com/dariushalipour/strata/internal/adapter/fs/memfs"
	"github.com/dariushalipour/strata/internal/adapter/fs/overlayfs"
	"github.com/dariushalipour/strata/internal/adapter/languages/golang"
	"github.com/dariushalipour/strata/internal/adapter/languages/typescript"
	"github.com/dariushalipour/strata/internal/port"
	"github.com/dariushalipour/strata/pkg/treeview"
)

// Workspace holds the shared state for file-system based test scenarios.
type Workspace struct {
	RootDir      string
	Files        map[string]string
	FSys         port.FileSystem
	Langs        []port.Language
	Errors       []string
	OverlayFiles map[string]string
}

// Reset clears workspace state for a new scenario.
func (w *Workspace) Reset() {
	w.Files = make(map[string]string)
	w.OverlayFiles = make(map[string]string)
	w.FSys = nil
	w.Langs = nil
}

// Initialize registers the shared steps against the given ScenarioContext.
// The caller must ensure the world stored in context provides a GetWorkspace()
// method that returns *Workspace.
func Initialize(sc *godog.ScenarioContext, getter func(context.Context) *Workspace) {
	sc.Step(`^a fresh workspace at "([^"]*)" with this layout:$`,
		func(ctx context.Context, rootDir, doc string) error {
			w := getter(ctx)
			w.Reset()
			w.RootDir = rootDir

			for _, e := range treeview.ParseTree(rootDir, doc) {
				w.Files[filepath.Join(e.BaseDir, e.RelPath)] = ""
			}
			return nil
		})

	sc.Step(`^file "([^"]*)" has content "([^"]*)"$`,
		func(ctx context.Context, fpath, content string) error {
			w := getter(ctx)
			absPath := filepath.Join(w.RootDir, fpath)
			if _, ok := w.Files[absPath]; !ok {
				return fmt.Errorf("file %q was not defined in the workspace layout", absPath)
			}
			w.Files[absPath] = content
			return nil
		})

	sc.Step(`^file "([^"]*)" has content:$`,
		func(ctx context.Context, fpath, content string) error {
			w := getter(ctx)
			absPath := filepath.Join(w.RootDir, fpath)
			if _, ok := w.Files[absPath]; !ok {
				return fmt.Errorf("file %q was not defined in the workspace layout", absPath)
			}
			w.Files[absPath] = content
			return nil
		})

	sc.Step(`^file "([^"]*)" has unsaved content "([^"]*)"$`,
		func(ctx context.Context, fpath, content string) error {
			w := getter(ctx)
			absPath := filepath.Join(w.RootDir, fpath)
			if _, ok := w.Files[absPath]; !ok {
				return fmt.Errorf("file %q was not defined in the workspace layout", absPath)
			}
			w.OverlayFiles[absPath] = content
			return nil
		})

	sc.Step(`^file "([^"]*)" has unsaved content:$`,
		func(ctx context.Context, fpath, content string) error {
			w := getter(ctx)
			absPath := filepath.Join(w.RootDir, fpath)
			if _, ok := w.Files[absPath]; !ok {
				return fmt.Errorf("file %q was not defined in the workspace layout", absPath)
			}
			w.OverlayFiles[absPath] = content
			return nil
		})

	sc.Step(`^the check uses the "([^"]*)" language adapter$`,
		func(ctx context.Context, langName string) error {
			return addLanguage(getter(ctx), langName)
		})

	sc.Step(`^the error(?:s)? (?:is|are):$`,
		func(ctx context.Context, doc *godog.DocString) error {
			w := getter(ctx)
			lines := strings.Split(strings.TrimSpace(doc.Content), "\n")
			var nonBlank []string
			for _, l := range lines {
				if s := strings.TrimSpace(l); s != "" {
					nonBlank = append(nonBlank, s)
				}
			}
			errors := w.Errors
			if len(errors) != len(nonBlank) {
				return fmt.Errorf("expected %d errors, got %d", len(nonBlank), len(errors))
			}
			for i, line := range nonBlank {
				if errors[i] != line {
					return fmt.Errorf("error %d expected %q, got: %s", i+1, line, errors[i])
				}
			}
			return nil
		})
}

func addLanguage(w *Workspace, langName string) error {
	switch langName {
	case "go":
		w.Langs = append(w.Langs, golang.Language{})
	case "typescript":
		w.Langs = append(w.Langs, typescript.Language{})
	}
	return nil
}

// BuildMemFS builds a memfs.FS from the workspace files map.
func BuildMemFS(files map[string]string) *memfs.FS {
	fsys := memfs.New()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, absPath := range paths {
		content := files[absPath]
		_ = fsys.WriteFile(absPath, []byte(content), 0644)
	}
	return fsys
}

func BuildFS(w *Workspace) port.FileSystem {
	fsys := w.FSys
	if fsys == nil {
		fsys = BuildMemFS(w.Files)
	}
	if len(w.OverlayFiles) == 0 {
		return fsys
	}
	overlays := make(map[string][]byte, len(w.OverlayFiles))
	for path, content := range w.OverlayFiles {
		overlays[path] = []byte(content)
	}
	return overlayfs.New(fsys, overlays)
}
