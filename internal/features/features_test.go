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
	"github.com/dariushalipour/baft/internal/application/usecase/dump"
	"github.com/dariushalipour/baft/internal/port"
	"github.com/dariushalipour/baft/pkg/treeview"
)

// ---------- shared workspace state ----------

type contractReport struct {
	contractPath string
	nodes        int
	edges        int
	isNew        bool
	amendNodes   int
	amendEdges   int
	hasAmendDiff bool
}

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
	Reports          []contractReport
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
	w.Reports = nil
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

	sc.Step(`^(\d+) capsules? (?:is|are) (?:discovered|dumped)$`,
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

type fileSnapshot struct {
	exists  bool
	content string
}

func snapshotFiles(fsys port.FileSystem, rootDir string) (map[string]fileSnapshot, error) {
	snapshots := make(map[string]fileSnapshot)
	err := fsys.WalkDir(rootDir, func(abs string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		content, err := fsys.ReadFile(abs)
		if err != nil {
			return err
		}
		snapshots[abs] = fileSnapshot{exists: true, content: string(content)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshots, nil
}

func resolveScenarioPath(rootDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(rootDir, path)
}

func assertFileContent(fsys port.FileSystem, rootDir, path, expected string) error {
	absPath := resolveScenarioPath(rootDir, path)
	content, err := fsys.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("%s not found: %v", path, err)
	}
	if strings.TrimSpace(string(content)) != strings.TrimSpace(expected) {
		return fmt.Errorf("%s content mismatch.\n--- Expected ---\n%s\n--- Got ---\n%s", path, expected, content)
	}
	return nil
}

func assertFileNotExists(fsys port.FileSystem, rootDir, path string) error {
	absPath := resolveScenarioPath(rootDir, path)
	_, err := fsys.Stat(absPath)
	if err == nil {
		return fmt.Errorf("expected %s to not exist, but it does", path)
	}
	return nil
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

// ---------- dump feature tests ----------

type dumpWorld struct {
	ws          workspace
	beforeFiles map[string]fileSnapshot
	errors      []dump.DumpError
	err         error
	readErrors  map[string]string
	logBuf      bytes.Buffer
}

type dumpWorldKey struct{}

func dw(ctx context.Context) *dumpWorld { return ctx.Value(dumpWorldKey{}).(*dumpWorld) }

func initializeDumpScenario(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w := &dumpWorld{
			readErrors: make(map[string]string),
		}
		w.ws.reset()
		return context.WithValue(ctx, dumpWorldKey{}, w), nil
	})

	registerSharedSteps(sc, func(ctx context.Context) *workspace { return &dw(ctx).ws })

	sc.Step(`^the "([^"]*)" language adapter cannot read "([^"]*)"$`,
		func(ctx context.Context, _, failFile string) error {
			w := dw(ctx)
			w.readErrors[failFile] = failFile
			return nil
		})

	sc.Step(`^the dump uses the "([^"]*)" language adapter$`,
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

	sc.Step(`^Contract at "([^"]+)" has (\d+) nodes? and (\d+) edges?$`,
		func(ctx context.Context, path string, nodes, edges int) error {
			w := dw(ctx)
			for _, r := range w.ws.Reports {
				rp := r.contractPath
				if filepath.IsAbs(rp) {
					rp, _ = filepath.Rel(w.ws.RootDir, rp)
				}
				rp = filepath.ToSlash(rp)
				if rp == path {
					if r.nodes != nodes {
						return fmt.Errorf("contract at %s: expected %d nodes, got %d", path, nodes, r.nodes)
					}
					if r.edges != edges {
						return fmt.Errorf("contract at %s: expected %d edges, got %d", path, edges, r.edges)
					}
					return nil
				}
			}
			return fmt.Errorf("no contract report found for %s", path)
		})

	sc.Step(`^Contract at "([^"]+)" is (new|an amendment)$`,
		func(ctx context.Context, path string, state string) error {
			w := dw(ctx)
			for _, r := range w.ws.Reports {
				rp := r.contractPath
				if filepath.IsAbs(rp) {
					rp, _ = filepath.Rel(w.ws.RootDir, rp)
				}
				rp = filepath.ToSlash(rp)
				if rp == path {
					if state == "new" && !r.isNew {
						return fmt.Errorf("contract at %s: expected new, got amendment", path)
					}
					if state == "an amendment" && r.isNew {
						return fmt.Errorf("contract at %s: expected amendment, got new", path)
					}
					return nil
				}
			}
			return fmt.Errorf("no contract report found for %s", path)
		})

	sc.Step(`^Contract at "([^"]+)" added (\d+) nodes and (\d+) edges$`,
		func(ctx context.Context, path string, nodes, edges int) error {
			w := dw(ctx)
			for _, r := range w.ws.Reports {
				rp := r.contractPath
				if filepath.IsAbs(rp) {
					rp, _ = filepath.Rel(w.ws.RootDir, rp)
				}
				rp = filepath.ToSlash(rp)
				if rp == path {
					if !r.hasAmendDiff {
						return fmt.Errorf("contract at %s: expected amend diff, got none", path)
					}
					if r.amendNodes != nodes {
						return fmt.Errorf("contract at %s: expected %d amended nodes, got %d", path, nodes, r.amendNodes)
					}
					if r.amendEdges != edges {
						return fmt.Errorf("contract at %s: expected %d amended edges, got %d", path, edges, r.amendEdges)
					}
					return nil
				}
			}
			return fmt.Errorf("no contract report found for %s", path)
		})

	sc.Step(`^the dump runs from "([^"]*)"$`,
		func(ctx context.Context, rootDir string) error {
			w := dw(ctx)
			w.ws.FSys = buildFS(&w.ws)
			beforeFiles, snapshotErr := snapshotFiles(w.ws.FSys, w.ws.RootDir)
			if snapshotErr != nil {
				return snapshotErr
			}
			w.beforeFiles = beforeFiles

			discovery := service.NewCapsuleDiscovery()
			for _, lang := range w.ws.Langs {
				lang.Register(discovery)
			}

			result, runErr := dump.RunWith(w.ws.FSys, rootDir, w.ws.Langs, &mermaid.MermaidRepository{}, discovery, &w.logBuf)
			if runErr != nil {
				w.err = runErr
				return nil
			}
			if result != nil {
				w.errors = result.Errors
				w.ws.CapsuleCount = len(result.Contracts)
				var errStrs []string
				for _, e := range result.Errors {
					errStrs = append(errStrs, e.Error())
				}
				w.ws.Errors = errStrs
				for _, c := range result.Contracts {
					r := contractReport{
						contractPath: c.ContractPath,
						nodes:        c.Nodes,
						edges:        c.Edges,
						isNew:        c.IsNew,
					}
					if c.AmendDiff != nil {
						r.hasAmendDiff = true
						r.amendNodes = c.AmendDiff.Nodes
						r.amendEdges = c.AmendDiff.Edges
					} else {
						r.amendNodes = -1
						r.amendEdges = -1
					}
					w.ws.Reports = append(w.ws.Reports, r)
				}
			}
			return nil
		})

	sc.Step(`^the dump errors$`,
		func(ctx context.Context) error {
			w := dw(ctx)
			if w.err == nil {
				return fmt.Errorf("expected error, got none")
			}
			return nil
		})

	sc.Step(`^file "([^"]+)" should be:$`,
		func(ctx context.Context, path, expected string) error {
			w := dw(ctx)
			return assertFileContent(w.ws.FSys, w.ws.RootDir, path, expected)
		})

	sc.Step(`^file "([^"]+)" should not exist$`,
		func(ctx context.Context, path string) error {
			w := dw(ctx)
			return assertFileNotExists(w.ws.FSys, w.ws.RootDir, path)
		})

	sc.Step(`^file "([^"]+)" should stay the same$`,
		func(ctx context.Context, path string) error {
			w := dw(ctx)
			absPath := resolveScenarioPath(w.ws.RootDir, path)
			original, exists := w.beforeFiles[absPath]
			if !exists || !original.exists {
				return fmt.Errorf("%s did not exist before the dump run", path)
			}
			content, err := w.ws.FSys.ReadFile(absPath)
			if err != nil {
				return fmt.Errorf("%s not found: %v", path, err)
			}
			if string(content) != original.content {
				return fmt.Errorf("%s changed.\n--- Before ---\n%s\n--- After ---\n%s", path, original.content, content)
			}
			return nil
		})
}

func TestDumpFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initializeDumpScenario,
		Options: &godog.Options{
			Strict:   true,
			Format:   "pretty",
			Paths:    []string{"../application/usecase/dump/dump.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run dump feature tests")
	}
}

// ---------- dump helpers ----------

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
