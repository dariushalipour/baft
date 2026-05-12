package draft

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/baft/internal/adapter/languages/golang"
	"github.com/dariushalipour/baft/internal/adapter/languages/typescript"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/application/steps"
	"github.com/dariushalipour/baft/internal/port"
)

type draftWorld struct {
	Workspace  steps.Workspace
	capsules   []CapsuleDraft
	errors     []DraftError
	err        error
	readErrors map[string]string
	logBuf     bytes.Buffer
}

func (w *draftWorld) GetWorkspace() *steps.Workspace { return &w.Workspace }

type worldKey struct{}

func dw(ctx context.Context) *draftWorld { return ctx.Value(worldKey{}).(*draftWorld) }

func InitializeScenario(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w := &draftWorld{
			readErrors: make(map[string]string),
		}
		w.Workspace.Reset()
		return context.WithValue(ctx, worldKey{}, w), nil
	})

	steps.Initialize(sc, func(ctx context.Context) *steps.Workspace { return dw(ctx).GetWorkspace() })

	sc.Step(`^the "([^"]*)" language adapter cannot read "([^"]*)"$`,
		func(ctx context.Context, _, failFile string) error {
			w := dw(ctx)
			w.readErrors[failFile] = failFile
			return nil
		})

	sc.Step(`^the draft uses the "([^"]*)" language adapter$`,
		func(ctx context.Context, langName string) error {
			w := dw(ctx)
			var lang port.Language
			switch langName {
			case "go":
				lang = golang.Language{}
				if len(w.readErrors) > 0 {
					lang = wrapLangWithMissingFiles(lang, w.readErrors)
				}
			case "typescript":
				lang = typescript.Language{}
			}
			if lang != nil {
				w.Workspace.Langs = append(w.Workspace.Langs, lang)
			}
			return nil
		})

	sc.Step(`^the draft runs from "([^"]*)"$`,
		func(ctx context.Context, rootDir string) error {
			w := dw(ctx)
			w.Workspace.FSys = steps.BuildFS(w.GetWorkspace())

			discovery := service.NewCapsuleDiscovery()
			for _, lang := range w.Workspace.Langs {
				lang.Register(discovery)
			}

			result, runErr := RunWith(w.Workspace.FSys, rootDir, w.Workspace.Langs, &mermaid.MermaidRepository{}, discovery, &w.logBuf)
			if runErr != nil {
				w.err = runErr
				return nil
			}
			if result != nil {
				w.capsules = result.Capsules
				w.errors = result.Errors
				var errStrs []string
				for _, e := range result.Errors {
					errStrs = append(errStrs, e.Error())
				}
				w.Workspace.Errors = errStrs
			}
			return nil
		})

	sc.Step(`^the draft succeeds$`,
		func(ctx context.Context) error {
			w := dw(ctx)
			if w.err != nil {
				return fmt.Errorf("expected no error, got: %v", w.err)
			}
			return nil
		})

	sc.Step(`^the draft errors$`,
		func(ctx context.Context) error {
			w := dw(ctx)
			if w.err == nil {
				return fmt.Errorf("expected error, got none")
			}
			return nil
		})

	sc.Step(`^(\d+) capsules? (?:is|are) drafted$`,
		func(ctx context.Context, n int) error {
			w := dw(ctx)
			if len(w.capsules) != n {
				return fmt.Errorf("expected %d capsules, got %d", n, len(w.capsules))
			}
			return nil
		})

	sc.Step(`^capsule (\d+) has (\d+) files? scanned$`,
		func(ctx context.Context, idx, n int) error {
			w := dw(ctx)
			if idx < 1 || idx > len(w.capsules) {
				return fmt.Errorf("capsule index %d out of range (have %d)", idx, len(w.capsules))
			}
			if w.capsules[idx-1].FilesScanned != n {
				return fmt.Errorf("capsule %d: expected %d files scanned, got %d", idx, n, w.capsules[idx-1].FilesScanned)
			}
			return nil
		})

	sc.Step(`^capsule (\d+) has (\d+) nodes?$`,
		func(ctx context.Context, idx, n int) error {
			w := dw(ctx)
			if idx < 1 || idx > len(w.capsules) {
				return fmt.Errorf("capsule index %d out of range (have %d)", idx, len(w.capsules))
			}
			if w.capsules[idx-1].Nodes != n {
				return fmt.Errorf("capsule %d: expected %d nodes, got %d", idx, n, w.capsules[idx-1].Nodes)
			}
			return nil
		})

	sc.Step(`^capsule (\d+) has (\d+) edges?$`,
		func(ctx context.Context, idx, n int) error {
			w := dw(ctx)
			if idx < 1 || idx > len(w.capsules) {
				return fmt.Errorf("capsule index %d out of range (have %d)", idx, len(w.capsules))
			}
			if w.capsules[idx-1].Edges != n {
				return fmt.Errorf("capsule %d: expected %d edges, got %d", idx, n, w.capsules[idx-1].Edges)
			}
			return nil
		})

	sc.Step(`^"([^"]+)" is expected to have content:$`,
		func(ctx context.Context, path, expected string) error {
			w := dw(ctx)
			absPath := path
			if !filepath.IsAbs(path) {
				absPath = filepath.Join(w.Workspace.RootDir, path)
			}
			content, err := w.Workspace.FSys.ReadFile(absPath)
			if err != nil {
				return fmt.Errorf("%s not found: %v", path, err)
			}
			if strings.TrimSpace(string(content)) != strings.TrimSpace(expected) {
				return fmt.Errorf("%s content mismatch.\n--- Expected ---\n%s\n--- Got ---\n%s", path, expected, content)
			}
			return nil
		})

	sc.Step(`^(\d+) error(?:s)? (?:is|are) reported$`,
		func(ctx context.Context, n int) error {
			w := dw(ctx)
			if len(w.errors) != n {
				return fmt.Errorf("expected %d errors, got %d", n, len(w.errors))
			}
			return nil
		})

	sc.Step(`^"([^"]+)" should not exist$`,
		func(ctx context.Context, path string) error {
			w := dw(ctx)
			absPath := path
			if !filepath.IsAbs(path) {
				absPath = filepath.Join(w.Workspace.RootDir, path)
			}
			_, err := w.Workspace.FSys.Stat(absPath)
			if err == nil {
				return fmt.Errorf("expected %s to not exist, but it does", path)
			}
			return nil
		})

	sc.Step(`^BAFT\.md is unchanged$`,
		func(ctx context.Context) error {
			w := dw(ctx)
			original, exists := w.Workspace.Files[filepath.Join(w.Workspace.RootDir, "BAFT.md")]
			if !exists {
				return fmt.Errorf("BAFT.md was not in the original files")
			}
			content, err := w.Workspace.FSys.ReadFile(filepath.Join(w.Workspace.RootDir, "BAFT.md"))
			if err != nil {
				return fmt.Errorf("BAFT.md not found: %v", err)
			}
			if string(content) != original {
				return fmt.Errorf("BAFT.md was modified, expected:\n%s\ngot:\n%s", original, content)
			}
			return nil
		})
}

func TestDraftFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"draft.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run draft feature tests")
	}
}

// wrapLangWithMissingFiles wraps a language adapter so that ParseImports
// returns fs.ErrNotExist for the configured file basenames.
func wrapLangWithMissingFiles(base port.Language, missingFiles map[string]string) port.Language {
	return &langWithMissingFiles{base: base, missingFiles: missingFiles}
}

type langWithMissingFiles struct {
	base         port.Language
	missingFiles map[string]string
}

func (l *langWithMissingFiles) Name() string { return l.base.Name() }
func (l *langWithMissingFiles) IsGovernedFile(rel string) bool {
	return l.base.IsGovernedFile(rel)
}
func (l *langWithMissingFiles) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	for rel := range l.missingFiles {
		if filepath.Base(absPath) == filepath.Base(rel) {
			return nil, &fs.PathError{Op: "read", Path: absPath, Err: fs.ErrNotExist}
		}
	}
	return l.base.ParseImports(fsys, absPath)
}
func (l *langWithMissingFiles) ResolveInternalTarget(fsys port.FileSystem, spec port.ImportSpec, c port.Capsule, rel string) (string, bool) {
	return l.base.ResolveInternalTarget(fsys, spec, c, rel)
}
func (l *langWithMissingFiles) SupportsFileGlobs() bool { return l.base.SupportsFileGlobs() }
func (l *langWithMissingFiles) SkipDirs() []string      { return l.base.SkipDirs() }
func (l *langWithMissingFiles) Register(d port.CapsuleDiscovery) {
	l.base.Register(d)
}
