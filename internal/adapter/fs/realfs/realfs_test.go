package realfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

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

	// Not-ignored file should be statable
	_, err := fsys.Stat(notIgnoredPath)
	if err != nil {
		t.Fatalf("expected not-ignored file to be statable, got: %v", err)
	}

	// Git-ignored file should NOT be statable
	_, err = fsys.Stat(ignoredPath)
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

	_, err := fsys.ReadFile(filepath.Join(dir, "visible.txt"))
	if err != nil {
		t.Fatalf("expected visible file to be readable, got: %v", err)
	}

	_, err = fsys.ReadFile(filepath.Join(dir, "ignored.txt"))
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

	seen := make(map[string]bool)
	_ = fsys.WalkDir(dir, func(abs string, d fs.DirEntry) error {
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

func TestDiscoverSkipsGitIgnoredBAFT(t *testing.T) {
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

	// Use Rust Discover — it should only find api/pkg, not web/pkg
	disco := service.NewCapsuleDiscovery()
	rust.Language{}.Register(disco)
	entries, err := disco.Discover(fsys, dir)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	// Discover no longer requires BAFT.md, so both capsules are found
	if len(entries) != 2 {
		t.Fatalf("expected 2 capsules, got %d", len(entries))
	}
}
