package features_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/fs/overlayfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/baft/internal/adapter/languages/golang"
	"github.com/dariushalipour/baft/internal/adapter/languages/typescript"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/application/usecase/check"
	"github.com/dariushalipour/baft/internal/application/usecase/draft"
	"github.com/dariushalipour/baft/internal/port"
	"github.com/dariushalipour/baft/pkg/treeview"
)

// ---------- shared workspace state ----------

type workspace struct {
	RootDir          string
	Files            map[string]string
	FSys             port.FileSystem
	Langs            []port.Language
	Errors           []string
	OverlayFiles     map[string]string
	CapsuleCount     int
	Relations        int
	FilesEncountered int
	FilesScanned     int
	Violations       []string
}

func (w *workspace) reset() {
	w.Files = make(map[string]string)
	w.OverlayFiles = make(map[string]string)
	w.FSys = nil
	w.Langs = nil
	w.Errors = nil
	w.Violations = nil
	w.CapsuleCount = 0
	w.Relations = 0
	w.FilesEncountered = 0
	w.FilesScanned = 0
}

func registerSharedSteps(sc *godog.ScenarioContext, getter func(context.Context) *workspace) {
	sc.Step(`^a fresh workspace at "([^"]*)" with this layout:$`,
		func(ctx context.Context, rootDir, doc string) error {
			w := getter(ctx)
			w.reset()
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

	sc.Step(`^file "([^"]*)" has content '([^']*)'$`,
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
			errs := w.Errors
			if len(errs) != len(nonBlank) {
				return fmt.Errorf("expected %d errors, got %d", len(nonBlank), len(errs))
			}
			for i, line := range nonBlank {
				if errs[i] != line {
					return fmt.Errorf("error %d expected %q, got: %s", i+1, line, errs[i])
				}
			}
			return nil
		})

	sc.Step(`^(\d+) capsules? (?:is|are) (?:discovered|drafted)$`,
		func(ctx context.Context, n int) error {
			w := getter(ctx)
			if w.CapsuleCount != n {
				return fmt.Errorf("expected %d capsules, got %d", n, w.CapsuleCount)
			}
			return nil
		})

	sc.Step(`^(\d+) relations? (?:is|are) examined$`,
		func(ctx context.Context, n int) error {
			w := getter(ctx)
			if w.Relations != n {
				return fmt.Errorf("expected %d relations examined, got %d", n, w.Relations)
			}
			return nil
		})

	sc.Step(`^(\d+) files? (?:is|are) encountered and (\d+) files? (?:is|are) scanned$`,
		func(ctx context.Context, nEncountered int, nScanned int) error {
			w := getter(ctx)
			if w.FilesEncountered != nEncountered {
				return fmt.Errorf("expected %d files encountered, got %d", nEncountered, w.FilesEncountered)
			}
			if w.FilesScanned != nScanned {
				return fmt.Errorf("expected %d files scanned, got %d", nScanned, w.FilesScanned)
			}
			return nil
		})

	sc.Step(`^(\d+) errors? and (\d+) violations? (?:is|are) reported$`,
		func(ctx context.Context, nErrors int, nViolations int) error {
			w := getter(ctx)
			if len(w.Errors) != nErrors {
				return fmt.Errorf("expected %d errors, got %d", nErrors, len(w.Errors))
			}
			if len(w.Violations) != nViolations {
				return fmt.Errorf("expected %d violations, got %d", nViolations, len(w.Violations))
			}
			return nil
		})

	sc.Step(`^the violations? (?:is|are):$`,
		func(ctx context.Context, doc *godog.DocString) error {
			w := getter(ctx)
			lines := strings.Split(strings.TrimSpace(doc.Content), "\n")
			var nonBlank []string
			for _, l := range lines {
				if s := strings.TrimSpace(l); s != "" {
					nonBlank = append(nonBlank, s)
				}
			}
			violations := w.Violations
			if len(violations) != len(nonBlank) {
				return fmt.Errorf("expected %d violations, got %d", len(nonBlank), len(violations))
			}
			for i, line := range nonBlank {
				if violations[i] != line {
					return fmt.Errorf("violation %d expected %q, got: %s", i+1, line, violations[i])
				}
			}
			return nil
		})
}

func addLanguage(w *workspace, langName string) error {
	switch langName {
	case "go":
		w.Langs = append(w.Langs, golang.Language{})
	case "typescript":
		w.Langs = append(w.Langs, &typescript.Language{})
	}
	return nil
}

func buildMemFS(files map[string]string) *memfs.FS {
	fsys := memfs.New()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, absPath := range paths {
		content := files[absPath]
		_ = fsys.WriteFile(absPath, []byte(content), 0o644)
	}
	return fsys
}

func buildFS(w *workspace) port.FileSystem {
	fsys := w.FSys
	if fsys == nil {
		fsys = buildMemFS(w.Files)
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

// ---------- check feature tests ----------

type checkWorld struct {
	ws  workspace
	err error
}

type checkWorldKey struct{}

func cw(ctx context.Context) *checkWorld { return ctx.Value(checkWorldKey{}).(*checkWorld) }

func initializeCheckScenario(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w := &checkWorld{}
		w.ws.reset()
		return context.WithValue(ctx, checkWorldKey{}, w), nil
	})

	registerSharedSteps(sc, func(ctx context.Context) *workspace { return &cw(ctx).ws })

	sc.Step(`^the check runs from "([^"]*)"$`,
		func(ctx context.Context, rootDir string) error {
			w := cw(ctx)
			w.ws.FSys = buildFS(&w.ws)

			discovery := service.NewCapsuleDiscovery()
			for _, lang := range w.ws.Langs {
				lang.Register(discovery)
			}

			result := check.Run(w.ws.FSys, rootDir, w.ws.Langs, &mermaid.MermaidRepository{}, discovery)
			if result == nil {
				w.err = fmt.Errorf("Run returned nil")
				return w.err
			}
			w.ws.CapsuleCount = len(result.Capsules)
			for _, c := range result.Capsules {
				w.ws.Relations += c.Relations
				w.ws.FilesEncountered += c.FilesEncountered
				w.ws.FilesScanned += c.FilesScanned
			}
			w.ws.Violations = result.Violations
			w.ws.Errors = result.Errors
			return nil
		})

	sc.Step(`^the filesystem always returns a walk error$`,
		func(ctx context.Context) error {
			w := cw(ctx)
			mem := memfs.New()
			mem.SetWalkError(w.ws.RootDir, errors.New("simulated walk error"))
			w.ws.FSys = mem
			return nil
		})

	sc.Step(`^the filesystem is not permitted to read "([^"]*)"$`,
		func(ctx context.Context, failPath string) error {
			w := cw(ctx)
			mem := buildMemFS(w.ws.Files)
			mem.SetReadError(filepath.Join(w.ws.RootDir, failPath), &fs.PathError{Op: "read", Path: filepath.Join(w.ws.RootDir, failPath), Err: fs.ErrPermission})
			w.ws.FSys = mem
			return nil
		})
}

func TestCheckFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initializeCheckScenario,
		Options: &godog.Options{
			Strict:   true,
			Format:   "pretty",
			Paths:    []string{"../application/usecase/check/check.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run check feature tests")
	}
}

func TestGolangCheckFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initializeCheckScenario,
		Options: &godog.Options{
			Strict:   true,
			Format:   "pretty",
			Paths:    []string{"../adapter/languages/golang/golang.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run golang check feature tests")
	}
}

func TestTypescriptCheckFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initializeCheckScenario,
		Options: &godog.Options{
			Strict:   true,
			Format:   "pretty",
			Paths:    []string{"../adapter/languages/typescript/typescript.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run typescript check feature tests")
	}
}

// ---------- draft feature tests ----------

type draftWorld struct {
	ws         workspace
	capsules   []draft.CapsuleDraft
	errors     []draft.DraftError
	err        error
	readErrors map[string]string
	logBuf     bytes.Buffer
}

type draftWorldKey struct{}

func dw(ctx context.Context) *draftWorld { return ctx.Value(draftWorldKey{}).(*draftWorld) }

func initializeDraftScenario(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w := &draftWorld{
			readErrors: make(map[string]string),
		}
		w.ws.reset()
		return context.WithValue(ctx, draftWorldKey{}, w), nil
	})

	registerSharedSteps(sc, func(ctx context.Context) *workspace { return &dw(ctx).ws })

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
				lang = &typescript.Language{}
			}
			if lang != nil {
				w.ws.Langs = append(w.ws.Langs, lang)
			}
			return nil
		})

	sc.Step(`^the draft runs from "([^"]*)"$`,
		func(ctx context.Context, rootDir string) error {
			w := dw(ctx)
			w.ws.FSys = buildFS(&w.ws)

			discovery := service.NewCapsuleDiscovery()
			for _, lang := range w.ws.Langs {
				lang.Register(discovery)
			}

			result, runErr := draft.RunWith(w.ws.FSys, rootDir, w.ws.Langs, &mermaid.MermaidRepository{}, discovery, &w.logBuf)
			if runErr != nil {
				w.err = runErr
				return nil
			}
			if result != nil {
				w.capsules = result.Capsules
				w.errors = result.Errors
				w.ws.CapsuleCount = len(result.Capsules)
				var errStrs []string
				for _, e := range result.Errors {
					errStrs = append(errStrs, e.Error())
				}
				w.ws.Errors = errStrs
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
				absPath = filepath.Join(w.ws.RootDir, path)
			}
			content, err := w.ws.FSys.ReadFile(absPath)
			if err != nil {
				return fmt.Errorf("%s not found: %v", path, err)
			}
			if strings.TrimSpace(string(content)) != strings.TrimSpace(expected) {
				return fmt.Errorf("%s content mismatch.\n--- Expected ---\n%s\n--- Got ---\n%s", path, expected, content)
			}
			return nil
		})

	sc.Step(`^"([^"]+)" should not exist$`,
		func(ctx context.Context, path string) error {
			w := dw(ctx)
			absPath := path
			if !filepath.IsAbs(path) {
				absPath = filepath.Join(w.ws.RootDir, path)
			}
			_, err := w.ws.FSys.Stat(absPath)
			if err == nil {
				return fmt.Errorf("expected %s to not exist, but it does", path)
			}
			return nil
		})

	sc.Step(`^BAFT\.md is unchanged$`,
		func(ctx context.Context) error {
			w := dw(ctx)
			original, exists := w.ws.Files[filepath.Join(w.ws.RootDir, "BAFT.md")]
			if !exists {
				return fmt.Errorf("BAFT.md was not in the original files")
			}
			content, err := w.ws.FSys.ReadFile(filepath.Join(w.ws.RootDir, "BAFT.md"))
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
		ScenarioInitializer: initializeDraftScenario,
		Options: &godog.Options{
			Strict:   true,
			Format:   "pretty",
			Paths:    []string{"../application/usecase/draft/draft.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run draft feature tests")
	}
}

// ---------- draft helpers ----------

func wrapLangWithMissingFiles(base port.Language, missingFiles map[string]string) port.Language {
	return &langWithMissingFiles{base: base, missingFiles: missingFiles}
}

type langWithMissingFiles struct {
	base         port.Language
	missingFiles map[string]string
}

func (l *langWithMissingFiles) Name() string { return l.base.Name() }
func (l *langWithMissingFiles) IsScannableFile(rel string) bool {
	return l.base.IsScannableFile(rel)
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
func (l *langWithMissingFiles) Register(d port.CapsuleDiscovery) {
	l.base.Register(d)
}
