package check

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/application/steps"
)

type checkWorld struct {
	Workspace        steps.Workspace
	capsules         int
	relations        int
	filesEncountered int
	filesScanned     int
	violations       []string
	errors           []string
	err              error
}

func (w *checkWorld) GetWorkspace() *steps.Workspace { return &w.Workspace }

func cw(ctx context.Context) *checkWorld { return ctx.Value(worldKey{}).(*checkWorld) }

func InitializeScenario(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w := &checkWorld{}
		w.Workspace.Reset()
		return context.WithValue(ctx, worldKey{}, w), nil
	})

	steps.Initialize(sc, func(ctx context.Context) *steps.Workspace { return cw(ctx).GetWorkspace() })

	sc.Step(`^the check runs from "([^"]*)"$`,
		func(ctx context.Context, rootDir string) error {
			w := cw(ctx)
			w.Workspace.FSys = steps.BuildFS(w.GetWorkspace())

			discovery := service.NewCapsuleDiscovery()
			for _, lang := range w.Workspace.Langs {
				lang.Register(discovery)
			}

			result := Run(w.Workspace.FSys, rootDir, w.Workspace.Langs, &mermaid.MermaidRepository{}, discovery)
			if result == nil {
				w.err = fmt.Errorf("Run returned nil")
				return w.err
			}
			w.capsules = len(result.Capsules)
			for _, c := range result.Capsules {
				w.relations += c.Relations
				w.filesEncountered += c.FilesEncountered
				w.filesScanned += c.FilesScanned
			}
			w.violations = result.Violations
			w.errors = result.Errors
			w.Workspace.Errors = result.Errors
			return nil
		})

	sc.Step(`^the filesystem always returns a walk error$`,
		func(ctx context.Context) error {
			w := cw(ctx)
			mem := memfs.New()
			mem.SetWalkError(w.Workspace.RootDir, errors.New("simulated walk error"))
			w.Workspace.FSys = mem
			return nil
		})

	sc.Step(`^the filesystem is not permitted to read "([^"]*)"$`,
		func(ctx context.Context, failPath string) error {
			w := cw(ctx)
			mem := steps.BuildMemFS(w.Workspace.Files)
			mem.SetReadError(filepath.Join(w.Workspace.RootDir, failPath), &fs.PathError{Op: "read", Path: filepath.Join(w.Workspace.RootDir, failPath), Err: fs.ErrPermission})
			w.Workspace.FSys = mem
			return nil
		})

	sc.Step(`^(\d+) capsules? (?:is|are) discovered$`,
		func(ctx context.Context, n int) error {
			w := cw(ctx)
			if w.capsules != n {
				return fmt.Errorf("expected %d capsules, got %d", n, w.capsules)
			}
			return nil
		})

	sc.Step(`^(\d+) violations? (?:is|are) reported$`,
		func(ctx context.Context, n int) error {
			w := cw(ctx)
			if len(w.violations) != n {
				return fmt.Errorf("expected %d violations, got %d", n, len(w.violations))
			}
			return nil
		})

	sc.Step(`^(\d+) errors? (?:is|are) reported$`,
		func(ctx context.Context, n int) error {
			w := cw(ctx)
			if len(w.errors) != n {
				return fmt.Errorf("expected %d errors, got %d", n, len(w.errors))
			}
			return nil
		})

	sc.Step(`^(\d+) relations? (?:is|are) examined$`,
		func(ctx context.Context, n int) error {
			w := cw(ctx)
			if w.relations != n {
				return fmt.Errorf("expected %d relations examined, got %d", n, w.relations)
			}
			return nil
		})

	sc.Step(`^(\d+) files? (?:is|are) encountered$`,
		func(ctx context.Context, n int) error {
			w := cw(ctx)
			if w.filesEncountered != n {
				return fmt.Errorf("expected %d files encountered, got %d", n, w.filesEncountered)
			}
			return nil
		})

	sc.Step(`^(\d+) files? (?:is|are) scanned$`,
		func(ctx context.Context, n int) error {
			w := cw(ctx)
			if w.filesScanned != n {
				return fmt.Errorf("expected %d files scanned, got %d", n, w.filesScanned)
			}
			return nil
		})

	sc.Step(`^the violations? (?:is|are):$`,
		func(ctx context.Context, doc *godog.DocString) error {
			w := cw(ctx)
			lines := strings.Split(strings.TrimSpace(doc.Content), "\n")
			var nonBlank []string
			for _, l := range lines {
				if s := strings.TrimSpace(l); s != "" {
					nonBlank = append(nonBlank, s)
				}
			}
			if len(w.violations) != len(nonBlank) {
				return fmt.Errorf("expected %d violations, got %d", len(nonBlank), len(w.violations))
			}
			for i, line := range nonBlank {
				if w.violations[i] != line {
					return fmt.Errorf("violation %d expected %q, got: %s", i+1, line, w.violations[i])
				}
			}
			return nil
		})

}

func TestCheckFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"check.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run check feature tests")
	}
}

type worldKey struct{}
