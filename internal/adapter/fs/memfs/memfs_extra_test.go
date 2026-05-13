package memfs

import (
	"io/fs"
	"testing"
)

func TestMkdir_CreatesDirectory(t *testing.T) {
	fsys := New()

	err := fsys.Mkdir("/emptydir", 0o755)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := fsys.ReadDir("/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name() != "emptydir" || !entries[0].IsDir() {
		t.Fatalf("expected dir 'emptydir', got %s (isDir=%v)", entries[0].Name(), entries[0].IsDir())
	}
}

func TestMkdirAll_CreatesNestedDirectories(t *testing.T) {
	fsys := New()

	err := fsys.MkdirAll("/a/b/c", 0o755)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := fsys.ReadDir("/a")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "b" {
		t.Fatalf("expected [b], got %v", entries)
	}

	entries, err = fsys.ReadDir("/a/b")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "c" {
		t.Fatalf("expected [c], got %v", entries)
	}
}

func TestMkdirAll_WithFileInEmptyDir(t *testing.T) {
	fsys := New()

	err := fsys.MkdirAll("/x/y", 0o755)
	if err != nil {
		t.Fatal(err)
	}

	err = fsys.WriteFile("/x/y/file.txt", []byte("content"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := fsys.ReadDir("/x")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "y" {
		t.Fatalf("expected [y], got %v", entries)
	}
}

func TestWalkDir_EmptyDirectory(t *testing.T) {
	fsys := New()

	err := fsys.MkdirAll("/empty/deep", 0o755)
	if err != nil {
		t.Fatal(err)
	}

	var count int
	var paths []string
	err = fsys.WalkDir("/empty", func(path string, d fs.DirEntry) error {
		count++
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// WalkDir skips the root itself, but should include child dirs
	if count < 1 {
		t.Fatalf("expected at least 1 entry (deep), got %d: %v", count, paths)
	}
	found := false
	for _, p := range paths {
		if p == "/empty/deep" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /empty/deep in walk, got: %v", paths)
	}
}

func TestMkdir_ParentNotExist(t *testing.T) {
	fsys := New()

	err := fsys.Mkdir("/no/parent/dir", 0o755)
	if err != nil {
		t.Fatal("Mkdir should succeed even when parent doesn't exist (creates all parents)")
	}

	entries, err := fsys.ReadDir("/no/parent")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "dir" {
		t.Fatalf("expected [dir], got %v", entries)
	}
}
