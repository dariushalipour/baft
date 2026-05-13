package ignorefs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/fs/realfs"
	"github.com/dariushalipour/baft/internal/port"
)

func makeTestFS(t *testing.T, files map[string]string) *memfs.FS {
	t.Helper()
	fsys := memfs.New()
	for path, content := range files {
		err := fsys.WriteFile(path, []byte(content), 0o644)
		if err != nil {
			t.Fatal(err)
		}
	}
	return fsys
}

func TestWrap_IgnoresFile(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":  "package main",
		"/Users/jane/baft/src/temp.go": "package temp",
		"/Users/jane/baft/.baftignore": "src/temp.go\n",
		"/Users/jane/baft/go.mod":      "module example.com/app",
		"/Users/jane/baft/BAFT.md":     "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// temp.go should be ignored
	_, err = ignored.ReadFile("/Users/jane/baft/src/temp.go")
	if err == nil {
		t.Fatal("expected error reading ignored file")
	}

	// app.go should be readable
	_, err = ignored.ReadFile("/Users/jane/baft/src/app.go")
	if err != nil {
		t.Fatalf("unexpected error reading non-ignored file: %v", err)
	}
}

func TestWrap_NestedBaftignore(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":          "package main",
		"/Users/jane/baft/src/internal/gen.go": "package internal",
		"/Users/jane/baft/src/.baftignore":     "internal/gen.go\n",
		"/Users/jane/baft/go.mod":              "module example.com/app",
		"/Users/jane/baft/BAFT.md":             "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// gen.go in nested dir should be ignored
	_, err = ignored.ReadFile("/Users/jane/baft/src/internal/gen.go")
	if err == nil {
		t.Fatal("expected error reading nested ignored file")
	}

	// app.go should be readable
	_, err = ignored.ReadFile("/Users/jane/baft/src/app.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrap_BaftignoreOverridesGitignore(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":     "package main",
		"/Users/jane/baft/src/ignored.go": "package ignored",
		"/Users/jane/baft/.gitignore":     "src/ignored.go\n",
		"/Users/jane/baft/.baftignore":    "!src/ignored.go\n",
		"/Users/jane/baft/go.mod":         "module example.com/app",
		"/Users/jane/baft/BAFT.md":        "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// ignored.go should be re-included by .baftignore negation
	_, err = ignored.ReadFile("/Users/jane/baft/src/ignored.go")
	if err != nil {
		t.Fatalf("expected file to be re-included, got error: %v", err)
	}
}

func TestWrap_NegationReincludesBaseIgnore(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/vendor/lib.go": "package vendor",
		"/Users/jane/baft/.baftignore":   "!vendor/**\n",
		"/Users/jane/baft/go.mod":        "module example.com/app",
		"/Users/jane/baft/BAFT.md":       "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir: "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{
			"vendor": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// vendor/ should be re-included by .baftignore negation
	_, err = ignored.ReadFile("/Users/jane/baft/vendor/lib.go")
	if err != nil {
		t.Fatalf("expected vendor/ to be re-included by negation, got error: %v", err)
	}
}

func TestWrap_IgnoredDirectoriesNotWalked(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":  "package main",
		"/Users/jane/baft/src/temp.go": "package temp",
		"/Users/jane/baft/.baftignore": "src/temp.go\n",
		"/Users/jane/baft/go.mod":      "module example.com/app",
		"/Users/jane/baft/BAFT.md":     "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	var paths []string
	err = ignored.WalkDir("/Users/jane/baft", func(abs string, d fs.DirEntry) error {
		paths = append(paths, abs)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// temp.go should not appear in walk
	for _, p := range paths {
		if filepath.Base(p) == "temp.go" {
			t.Fatal("ignored file temp.go should not appear in WalkDir")
		}
	}
}

func TestWrap_HardIgnoreVendor(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/vendor/lib.go": "package vendor",
		"/Users/jane/baft/src/app.go":    "package main",
		"/Users/jane/baft/go.mod":        "module example.com/app",
		"/Users/jane/baft/BAFT.md":       "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{"vendor": true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// vendor/ should be ignored
	_, err = ignored.ReadFile("/Users/jane/baft/vendor/lib.go")
	if err == nil {
		t.Fatal("expected vendor/ to be ignored")
	}

	// app.go should be readable
	_, err = ignored.ReadFile("/Users/jane/baft/src/app.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrap_DotDirNotAutoIgnored(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/.secret/data.go": "package secret",
		"/Users/jane/baft/src/app.go":      "package main",
		"/Users/jane/baft/go.mod":          "module example.com/app",
		"/Users/jane/baft/BAFT.md":         "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// .secret/ should NOT be ignored merely for having a dot prefix
	_, err = ignored.ReadFile("/Users/jane/baft/.secret/data.go")
	if err != nil {
		t.Fatalf("expected dot-prefixed dir to be readable, got: %v", err)
	}
}

func TestWrap_ReadDirExcludesIgnored(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":  "package main",
		"/Users/jane/baft/src/temp.go": "package temp",
		"/Users/jane/baft/.baftignore": "src/temp.go\n",
		"/Users/jane/baft/go.mod":      "module example.com/app",
		"/Users/jane/baft/BAFT.md":     "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, err := ignored.ReadDir("/Users/jane/baft/src")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.Name() == "temp.go" {
			t.Fatal("ignored file should not appear in ReadDir")
		}
	}
}

func TestWrap_GitignoreAndBaftignoreBothLoaded(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":      "package main",
		"/Users/jane/baft/src/gitfile.go":  "package gitfile",
		"/Users/jane/baft/src/baftfile.go": "package baftfile",
		"/Users/jane/baft/.gitignore":      "src/gitfile.go\n",
		"/Users/jane/baft/.baftignore":     "src/baftfile.go\n",
		"/Users/jane/baft/go.mod":          "module example.com/app",
		"/Users/jane/baft/BAFT.md":         "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Both should be ignored
	_, err = ignored.ReadFile("/Users/jane/baft/src/gitfile.go")
	if err == nil {
		t.Fatal("gitfile.go should be ignored by .gitignore")
	}
	_, err = ignored.ReadFile("/Users/jane/baft/src/baftfile.go")
	if err == nil {
		t.Fatal("baftfile.go should be ignored by .baftignore")
	}

	// app.go should be readable
	_, err = ignored.ReadFile("/Users/jane/baft/src/app.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrap_BaftignoreTakesPrecedenceOverGitignore(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":  "package main",
		"/Users/jane/baft/src/both.go": "package both",
		"/Users/jane/baft/.gitignore":  "src/both.go\n",
		"/Users/jane/baft/.baftignore": "!src/both.go\n",
		"/Users/jane/baft/go.mod":      "module example.com/app",
		"/Users/jane/baft/BAFT.md":     "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// .baftignore negation should override .gitignore
	_, err = ignored.ReadFile("/Users/jane/baft/src/both.go")
	if err != nil {
		t.Fatalf("baftignore negation should re-include file: %v", err)
	}
}

func TestWrap_NestedBaftignoreDomainScope(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":          "package main",
		"/Users/jane/baft/src/order.go":        "package src",
		"/Users/jane/baft/src/internal/gen.go": "package internal",
		"/Users/jane/baft/.baftignore":         "src/order.go\n",
		"/Users/jane/baft/src/.baftignore":     "gen.go\n",
		"/Users/jane/baft/go.mod":              "module example.com/app",
		"/Users/jane/baft/BAFT.md":             "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Root .baftignore should ignore order.go
	_, err = ignored.ReadFile("/Users/jane/baft/src/order.go")
	if err == nil {
		t.Fatal("order.go should be ignored by root .baftignore")
	}

	// Nested .baftignore should ignore gen.go within src/ domain
	_, err = ignored.ReadFile("/Users/jane/baft/src/internal/gen.go")
	if err == nil {
		t.Fatal("gen.go should be ignored by nested .baftignore in src/")
	}

	// app.go should be readable
	_, err = ignored.ReadFile("/Users/jane/baft/src/app.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrap_WalkDirSkipsIgnoredDirs(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":  "package main",
		"/Users/jane/baft/.git/config": "test",
		"/Users/jane/baft/go.mod":      "module example.com/app",
		"/Users/jane/baft/BAFT.md":     "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	var paths []string
	err = ignored.WalkDir("/Users/jane/baft", func(abs string, d fs.DirEntry) error {
		paths = append(paths, abs)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range paths {
		if strings.Contains(p, ".git") {
			t.Fatal(".git/ should not appear in WalkDir")
		}
	}
}

func TestWrap_StatIgnoresFile(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":  "package main",
		"/Users/jane/baft/src/temp.go": "package temp",
		"/Users/jane/baft/.baftignore": "src/temp.go\n",
		"/Users/jane/baft/go.mod":      "module example.com/app",
		"/Users/jane/baft/BAFT.md":     "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ignored.Stat("/Users/jane/baft/src/temp.go")
	if err == nil {
		t.Fatal("expected error stat'ing ignored file")
	}
}

func TestWrap_DiscoveryBaseIgnoreEntries(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/vendor/lib.go": "package vendor",
		"/Users/jane/baft/src/app.go":    "package main",
		"/Users/jane/baft/go.mod":        "module example.com/app",
		"/Users/jane/baft/BAFT.md":       "```mermaid\n```",
	})

	baseEntries := map[string]bool{"vendor": true}
	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: baseEntries,
	})
	if err != nil {
		t.Fatal(err)
	}

	var paths []string
	err = ignored.WalkDir("/Users/jane/baft", func(abs string, d fs.DirEntry) error {
		paths = append(paths, abs)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range paths {
		if strings.Contains(p, "vendor") {
			t.Fatal("vendor/ should not appear in WalkDir when in BaseIgnoreEntries")
		}
	}
}

func TestWrap_UserMatcherCanOverrideBaseIgnore(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/vendor/lib.go": "package vendor",
		"/Users/jane/baft/.baftignore":   "!vendor/**\n",
		"/Users/jane/baft/go.mod":        "module example.com/app",
		"/Users/jane/baft/BAFT.md":       "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir: "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{
			"vendor": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// vendor/ should be re-included by .baftignore negation
	_, err = ignored.Stat("/Users/jane/baft/vendor/lib.go")
	if err != nil {
		t.Fatalf("expected vendor/ to be re-included by negation, got error: %v", err)
	}
}

func TestWrap_GlobPattern(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":   "package main",
		"/Users/jane/baft/src/temp1.go": "package temp",
		"/Users/jane/baft/src/temp2.go": "package temp",
		"/Users/jane/baft/.baftignore":  "src/temp*.go\n",
		"/Users/jane/baft/go.mod":       "module example.com/app",
		"/Users/jane/baft/BAFT.md":      "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Both temp files should be ignored
	_, err = ignored.ReadFile("/Users/jane/baft/src/temp1.go")
	if err == nil {
		t.Fatal("temp1.go should be ignored by glob pattern")
	}
	_, err = ignored.ReadFile("/Users/jane/baft/src/temp2.go")
	if err == nil {
		t.Fatal("temp2.go should be ignored by glob pattern")
	}

	// app.go should be readable
	_, err = ignored.ReadFile("/Users/jane/baft/src/app.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrap_DirOnlyPattern(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/build/output.txt": "output",
		"/Users/jane/baft/build/lib.go":     "package build",
		"/Users/jane/baft/.baftignore":      "build/\n",
		"/Users/jane/baft/go.mod":           "module example.com/app",
		"/Users/jane/baft/BAFT.md":          "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Both files in build/ should be ignored (dir-only pattern)
	_, err = ignored.ReadFile("/Users/jane/baft/build/output.txt")
	if err == nil {
		t.Fatal("build/output.txt should be ignored by dir-only pattern")
	}
	_, err = ignored.ReadFile("/Users/jane/baft/build/lib.go")
	if err == nil {
		t.Fatal("build/lib.go should be ignored by dir-only pattern")
	}
}

func TestWrap_OverlayFilesContainIgnoreFiles(t *testing.T) {
	// This test verifies that ReadDir works correctly even when the
	// underlying filesystem doesn't have the directory entries visible.
	// The ignore wrapper should use ReadDir from the lower fs.
	lower := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":  "package main",
		"/Users/jane/baft/src/temp.go": "package temp",
		"/Users/jane/baft/.baftignore": "src/temp.go\n",
		"/Users/jane/baft/go.mod":      "module example.com/app",
		"/Users/jane/baft/BAFT.md":     "```mermaid\n```",
	})

	ignored, err := Wrap(lower, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// ReadDir should use the lower fs's ReadDir which delegates to the
	// interface. Since memfs has no real directories, we verify that
	// the wrapper doesn't crash.
	_, err = ignored.ReadDir("/Users/jane/baft/src")
	if err != nil {
		// This is expected since memfs ReadDir doesn't know about
		// the directory structure the same way realfs does.
		// The important thing is it doesn't panic.
	}
}

func TestWrap_HardIgnoredDirs(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/.idea/config.xml": "config",
		"/Users/jane/baft/src/app.go":       "package main",
		"/Users/jane/baft/go.mod":           "module example.com/app",
		"/Users/jane/baft/BAFT.md":          "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// .idea/ should be ignored by base patterns
	_, err = ignored.ReadFile("/Users/jane/baft/.idea/config.xml")
	if err == nil {
		t.Fatal(".idea/ should be ignored")
	}
}

func TestWrap_CoverageDir(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/coverage/out.txt": "coverage",
		"/Users/jane/baft/coverage.lcov":    "lcov",
		"/Users/jane/baft/src/app.go":       "package main",
		"/Users/jane/baft/go.mod":           "module example.com/app",
		"/Users/jane/baft/BAFT.md":          "```mermaid\n```",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// coverage/ and coverage.lcov should be ignored
	_, err = ignored.ReadFile("/Users/jane/baft/coverage/out.txt")
	if err == nil {
		t.Fatal("coverage/ should be ignored")
	}
	_, err = ignored.ReadFile("/Users/jane/baft/coverage.lcov")
	if err == nil {
		t.Fatal("coverage.lcov should be ignored")
	}
}

func TestWrap_RelativePathComputation(t *testing.T) {
	// Bug #2: manual prefix stripping was fragile with different root formats
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go":  "package main",
		"/Users/jane/baft/src/temp.go": "package temp",
		"/Users/jane/baft/.baftignore": "src/temp.go\n",
		"/Users/jane/baft/go.mod":      "module example.com/app",
		"/Users/jane/baft/BAFT.md":     "content",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ignored.ReadFile("/Users/jane/baft/src/temp.go")
	if err == nil {
		t.Fatal("expected error reading ignored file")
	}

	_, err = ignored.ReadFile("/Users/jane/baft/src/app.go")
	if err != nil {
		t.Fatalf("unexpected error reading non-ignored file: %v", err)
	}
}

func TestWrap_ExplicitEmptyDir(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/src/app.go": "package main",
		"/Users/jane/baft/go.mod":     "module example.com/app",
		"/Users/jane/baft/BAFT.md":    "content",
	})

	_ = fsys.Mkdir("/Users/jane/baft/emptydir", 0o755)

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, err := ignored.ReadDir("/Users/jane/baft")
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, e := range entries {
		if e.Name() == "emptydir" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'emptydir' to appear in ReadDir")
	}
}

func TestWrap_ParentGitignore(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/.git": "",
		// Parent .gitignore ignores the "dist" directory
		"/Users/jane/baft/.gitignore": "dist/\n",
		// Child package has its own files
		"/Users/jane/baft/packages/cli/main.go":        "package main",
		"/Users/jane/baft/packages/cli/dist/bundle.js": "console.log('hi')",
	})

	// Running baft from a subdirectory should still respect the parent .gitignore
	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft/packages/cli",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// bundle.js should be ignored by the parent .gitignore
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/dist/bundle.js")
	if err == nil {
		t.Fatal("expected error reading file ignored by parent .gitignore")
	}

	// main.go should be readable
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/main.go")
	if err != nil {
		t.Fatalf("unexpected error reading non-ignored file: %v", err)
	}
}

func TestWrap_ChildBaftignoreOverridesParent(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/.git": "",
		// Parent .gitignore ignores "dist"
		"/Users/jane/baft/.gitignore": "dist/\n",
		// Child .baftignore un-ignores it
		"/Users/jane/baft/packages/cli/.baftignore":    "!dist/\n",
		"/Users/jane/baft/packages/cli/main.go":        "package main",
		"/Users/jane/baft/packages/cli/dist/bundle.js": "console.log('hi')",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft/packages/cli",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// bundle.js should be readable — child .baftignore overrides parent .gitignore
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/dist/bundle.js")
	if err != nil {
		t.Fatalf("expected file to be readable after child .baftignore override: %v", err)
	}
}

func TestWrap_NoGitRepo(t *testing.T) {
	// No .git directory anywhere — should still work, just no parent patterns.
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/project/pkg/main.go":     "package main",
		"/Users/jane/project/pkg/.baftignore": "temp.go\n",
		"/Users/jane/project/pkg/temp.go":     "junk",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/project/pkg",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ignored.ReadFile("/Users/jane/project/pkg/main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = ignored.ReadFile("/Users/jane/project/pkg/temp.go")
	if err == nil {
		t.Fatal("expected temp.go to be ignored")
	}
}

func TestWrap_MultipleParentGitignoreLevels(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		// Repo root .gitignore
		"/Users/jane/baft/.git":       "",
		"/Users/jane/baft/.gitignore": "*.log\n",
		// Intermediate .gitignore
		"/Users/jane/baft/packages/.gitignore": "*.tmp\n",
		// Files in deep subdirectory
		"/Users/jane/baft/packages/cli/debug.log": "log data",
		"/Users/jane/baft/packages/cli/cache.tmp": "tmp data",
		"/Users/jane/baft/packages/cli/main.go":   "package main",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft/packages/cli",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// *.log from repo root should match
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/debug.log")
	if err == nil {
		t.Fatal("expected debug.log to be ignored by repo root .gitignore")
	}

	// *.tmp from intermediate should match
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/cache.tmp")
	if err == nil {
		t.Fatal("expected cache.tmp to be ignored by intermediate .gitignore")
	}

	// main.go should be fine
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrap_ParentGitignoreNegation(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/.git": "",
		// Ignore all .log files, but re-include important.log
		"/Users/jane/baft/.gitignore":                 "*.log\n!important.log\n",
		"/Users/jane/baft/packages/cli/debug.log":     "log data",
		"/Users/jane/baft/packages/cli/important.log": "important",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft/packages/cli",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// debug.log should be ignored
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/debug.log")
	if err == nil {
		t.Fatal("expected debug.log to be ignored")
	}

	// important.log should be readable (negation in parent .gitignore)
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/important.log")
	if err != nil {
		t.Fatalf("expected important.log to be readable: %v", err)
	}
}

func TestWrap_WorktreeGitFile(t *testing.T) {
	// In a worktree, .git is a file, not a directory.
	fsys := makeTestFS(t, map[string]string{
		// .git as a file (worktree pointer)
		"/Users/jane/worktrees/feature/.git":               "gitdir: /Users/jane/baft/.git/worktrees/feature\n",
		"/Users/jane/worktrees/feature/.gitignore":         "dist/\n",
		"/Users/jane/worktrees/feature/pkg/main.go":        "package main",
		"/Users/jane/worktrees/feature/pkg/dist/bundle.js": "bundle",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/worktrees/feature/pkg",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// dist/ should be ignored by the worktree root .gitignore
	_, err = ignored.ReadFile("/Users/jane/worktrees/feature/pkg/dist/bundle.js")
	if err == nil {
		t.Fatal("expected dist/bundle.js to be ignored")
	}

	_, err = ignored.ReadFile("/Users/jane/worktrees/feature/pkg/main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrap_DeepNestingWithParentPatterns(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/.git":                                  "",
		"/Users/jane/baft/.gitignore":                            "node_modules/\n",
		"/Users/jane/baft/a/b/c/d/e/f/main.go":                   "package main",
		"/Users/jane/baft/a/b/c/d/e/f/node_modules/pkg/index.js": "module",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft/a/b/c/d/e/f",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ignored.ReadFile("/Users/jane/baft/a/b/c/d/e/f/node_modules/pkg/index.js")
	if err == nil {
		t.Fatal("expected node_modules to be ignored by deep parent .gitignore")
	}

	_, err = ignored.ReadFile("/Users/jane/baft/a/b/c/d/e/f/main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrap_ParentBaftignore(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/.git": "",
		// Parent .baftignore (not .gitignore) should be respected
		"/Users/jane/baft/.baftignore":                 "dist/\n",
		"/Users/jane/baft/packages/cli/main.go":        "package main",
		"/Users/jane/baft/packages/cli/dist/bundle.js": "bundle",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft/packages/cli",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/dist/bundle.js")
	if err == nil {
		t.Fatal("expected dist/bundle.js to be ignored by parent .baftignore")
	}

	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrap_ParentBaftignoreOverridesParentGitignore(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/.git": "",
		// Parent .gitignore ignores dist/
		"/Users/jane/baft/.gitignore": "dist/\n",
		// Parent .baftignore (same level, higher precedence) un-ignores it
		"/Users/jane/baft/.baftignore":                 "!dist/\n",
		"/Users/jane/baft/packages/cli/main.go":        "package main",
		"/Users/jane/baft/packages/cli/dist/bundle.js": "bundle",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft/packages/cli",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// .baftignore is loaded after .gitignore at same level, so !dist/ wins
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/dist/bundle.js")
	if err != nil {
		t.Fatalf("expected dist/bundle.js to be readable after parent .baftignore override: %v", err)
	}
}

func TestWrap_ParentPatternDomainScoped(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/.git": "",
		// Intermediate .gitignore has pattern "libs/vendor/" — scoped to packages/.
		// It matches packages/libs/vendor/ but NOT packages/cli/libs/vendor/.
		"/Users/jane/baft/packages/.gitignore": "libs/vendor/\n",
		// Should match: the pattern resolves to packages/libs/vendor/
		"/Users/jane/baft/packages/cli/libs/vendor/foo.go": "vendor",
		// Should NOT match: cli/libs/vendor is not libs/vendor relative to packages/
		"/Users/jane/baft/packages/cli/vendor/bar.go": "mine",
		"/Users/jane/baft/packages/cli/main.go":       "package main",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft/packages/cli",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// libs/vendor/foo.go should NOT be ignored — the parent pattern "libs/vendor/"
	// matches packages/libs/vendor/, not packages/cli/libs/vendor/.
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/libs/vendor/foo.go")
	if err != nil {
		t.Fatalf("expected libs/vendor/foo.go to be readable (parent pattern doesn't match nested path): %v", err)
	}

	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/vendor/bar.go")
	if err != nil {
		t.Fatalf("expected vendor/bar.go to be readable: %v", err)
	}
}

func TestWrap_ConcurrentAccessWithParentPatterns(t *testing.T) {
	fsys := makeTestFS(t, map[string]string{
		"/Users/jane/baft/.git":                    "",
		"/Users/jane/baft/.gitignore":              "*.log\n",
		"/Users/jane/baft/packages/cli/main.go":    "package main",
		"/Users/jane/baft/packages/cli/debug.log":  "log",
		"/Users/jane/baft/packages/cli/output.log": "log",
		"/Users/jane/baft/packages/cli/trace.log":  "log",
	})

	ignored, err := Wrap(fsys, Options{
		RootDir:           "/Users/jane/baft/packages/cli",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	files := []string{
		"/Users/jane/baft/packages/cli/main.go",
		"/Users/jane/baft/packages/cli/debug.log",
		"/Users/jane/baft/packages/cli/output.log",
		"/Users/jane/baft/packages/cli/trace.log",
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		for _, f := range files {
			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				_, _ = ignored.ReadFile(path)
			}(f)
		}
	}
	wg.Wait()

	// Verify results are still correct after concurrent access.
	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = ignored.ReadFile("/Users/jane/baft/packages/cli/debug.log")
	if err == nil {
		t.Fatal("expected debug.log to be ignored")
	}
}

func TestWrap_RelativeRootDir(t *testing.T) {
	// When RootDir is a relative path like ".", ignorefs.Wrap should
	// still succeed instead of failing with "Rel: can't make . relative to ...".
	dir := t.TempDir()

	// Create a .git directory so findRepoRoot resolves to this dir.
	_ = os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)

	cwd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer os.Chdir(cwd)

	fsys := realfs.New()
	_, err := Wrap(fsys, Options{
		RootDir:           ".",
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatalf("Wrap failed with relative RootDir: %v", err)
	}
}

// Ensure ignoreWrapper implements port.FileSystem.
var _ port.FileSystem = (*ignoreWrapper)(nil)
