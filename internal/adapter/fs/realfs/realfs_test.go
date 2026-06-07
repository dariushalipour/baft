package realfs

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/ignorefs"
	"github.com/dariushalipour/baft/internal/adapter/languages/golang"
	"github.com/dariushalipour/baft/internal/adapter/languages/rust"
	"github.com/dariushalipour/baft/internal/application/service"
)

func TestStatSkipsGitIgnored(t *testing.T) {
	dir := t.TempDir()

	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0o755)

	// Write .gitignore
	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("sub/\n"), 0o644)

	// Write a file inside the ignored directory
	ignoredPath := filepath.Join(dir, "sub", "BAFT.md")
	_ = os.WriteFile(ignoredPath, []byte("# ignored"), 0o644)

	// Write a file NOT ignored
	notIgnoredPath := filepath.Join(dir, "top.md")
	_ = os.WriteFile(notIgnoredPath, []byte("# top"), 0o644)

	fsys := New()

	// Wrap with ignorefs to get gitignore behavior
	wrapped, err := ignorefs.Wrap(fsys, ignorefs.Options{
		RootDir:           dir,
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Not-ignored file should be statable
	_, err = wrapped.Stat(notIgnoredPath)
	if err != nil {
		t.Fatalf("expected not-ignored file to be statable, got: %v", err)
	}

	// Git-ignored file should NOT be statable
	_, err = wrapped.Stat(ignoredPath)
	if !os.IsNotExist(err) {
		t.Fatalf("expected git-ignored file to return ErrNotExist, got: %v", err)
	}
}

func TestReadFileSkipsGitIgnored(t *testing.T) {
	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("secret"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("hello"), 0o644)

	fsys := New()

	// Wrap with ignorefs to get gitignore behavior
	wrapped, err := ignorefs.Wrap(fsys, ignorefs.Options{
		RootDir:           dir,
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = wrapped.ReadFile(filepath.Join(dir, "visible.txt"))
	if err != nil {
		t.Fatalf("expected visible file to be readable, got: %v", err)
	}

	_, err = wrapped.ReadFile(filepath.Join(dir, "ignored.txt"))
	if !os.IsNotExist(err) {
		t.Fatalf("expected git-ignored file to return ErrNotExist, got: %v", err)
	}
}

func TestWalkDirSkipsGitIgnored(t *testing.T) {
	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("secret"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("hello"), 0o644)

	fsys := New()

	// Wrap with ignorefs to get gitignore behavior
	wrapped, err := ignorefs.Wrap(fsys, ignorefs.Options{
		RootDir:           dir,
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]bool)
	_ = wrapped.WalkDir(context.Background(), dir, func(abs string, d fs.DirEntry) error {
		seen[abs] = true
		return nil
	})

	if seen[filepath.Join(dir, "ignored.txt")] {
		t.Error("WalkDir should not have visited git-ignored file")
	}
	if !seen[filepath.Join(dir, "visible.txt")] {
		t.Error("WalkDir should have visited visible file")
	}
}

func TestWalkDirDoesNotPropagateSkipDirAsError(t *testing.T) {
	dir := t.TempDir()

	// Create an ignored directory (triggers fs.SkipDir during walk)
	_ = os.MkdirAll(filepath.Join(dir, "ignored_dir"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "ignored_dir", "file.txt"), []byte("ignored"), 0o644)

	// Create a Go capsule in a visible sibling directory — must still be discovered
	capsuleDir := filepath.Join(dir, "visible_capsule")
	_ = os.MkdirAll(capsuleDir, 0o755)
	_ = os.WriteFile(filepath.Join(capsuleDir, "go.mod"), []byte("module example.com/visible\n"), 0o644)
	_ = os.WriteFile(filepath.Join(capsuleDir, "BAFT.md"), []byte("```mermaid\nflowchart TD\nmain[\".\"]\n```\n"), 0o644)

	// .gitignore that ignores the first directory
	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored_dir/\n"), 0o644)

	fsys := New()

	wrapped, err := ignorefs.Wrap(fsys, ignorefs.Options{
		RootDir:           dir,
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Discover must NOT error and must find the visible capsule
	disco := service.NewCapsuleDiscovery()
	golang.Language{}.Register(disco)
	entries, discoverErr := disco.Discover(context.Background(), wrapped, dir)
	if discoverErr != nil {
		t.Fatalf("Discover must not propagate fs.SkipDir as error, got: %v", discoverErr)
	}

	// Must find the visible capsule
	if len(entries) != 1 {
		t.Fatalf("Discover must find 1 capsule, got %d", len(entries))
	}
	if entries[0].Capsule.Dir != capsuleDir {
		t.Errorf("Discover found wrong capsule: got %s, want %s", entries[0].Capsule.Dir, capsuleDir)
	}

	// The ignored directory must NOT have been visited
	seen := make(map[string]bool)
	walkErr := wrapped.WalkDir(context.Background(), dir, func(abs string, d fs.DirEntry) error {
		seen[abs] = true
		return nil
	})
	if walkErr != nil {
		t.Fatalf("WalkDir must not propagate fs.SkipDir as error, got: %v", walkErr)
	}
	if seen[filepath.Join(dir, "ignored_dir")] {
		t.Error("WalkDir must not visit ignored directory")
	}
	if !seen[capsuleDir] {
		t.Error("WalkDir must visit visible capsule directory")
	}
}

func TestDiscoverSkipsGitIgnoredBaft(t *testing.T) {
	dir := t.TempDir()

	// Create a Rust capsule with a git-ignored BAFT.md
	capsuleDir := filepath.Join(dir, "web", "pkg")
	_ = os.MkdirAll(capsuleDir, 0o755)
	_ = os.WriteFile(filepath.Join(capsuleDir, "Cargo.toml"), []byte("[package]\nname = \"web-pkg\"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(capsuleDir, "BAFT.md"), []byte("# ignored"), 0o644)

	// Create a Rust capsule with a visible BAFT.md
	capsuleDir2 := filepath.Join(dir, "api", "pkg")
	_ = os.MkdirAll(capsuleDir2, 0o755)
	_ = os.WriteFile(filepath.Join(capsuleDir2, "Cargo.toml"), []byte("[package]\nname = \"api-pkg\"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(capsuleDir2, "BAFT.md"), []byte("# visible"), 0o644)

	// .gitignore that ignores web/pkg/BAFT.md
	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("web/pkg/BAFT.md\n"), 0o644)

	fsys := New()

	// Wrap with ignorefs to get gitignore behavior
	wrapped, err := ignorefs.Wrap(fsys, ignorefs.Options{
		RootDir:           dir,
		BaseIgnoreEntries: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Use Rust Discover — it should only find api/pkg, not web/pkg
	disco := service.NewCapsuleDiscovery()
	rust.Language{}.Register(disco)
	entries, err := disco.Discover(context.Background(), wrapped, dir)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	// Discover no longer requires BAFT.md, so both capsules are found
	if len(entries) != 2 {
		t.Fatalf("expected 2 capsules, got %d", len(entries))
	}
}
