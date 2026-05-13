package memfs

import (
	"testing"
)

func TestReadDir_NestedDirsWithoutDirectFiles(t *testing.T) {
	fsys := New()
	fsys.WriteFile("/root/a/b/c/file.txt", []byte("data"), 0o644)

	entries, err := fsys.ReadDir("/root")
	if err != nil {
		t.Fatal(err)
	}

	// Should find "a" as a directory.
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name() != "a" || !entries[0].IsDir() {
		t.Fatalf("expected dir 'a', got %s (isDir=%v)", entries[0].Name(), entries[0].IsDir())
	}

	// a/b should also be traversable.
	entries, err = fsys.ReadDir("/root/a/b")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "c" {
		t.Fatalf("expected [c], got %v", entries)
	}
}

func TestReadDir_MixedFilesAndDirectories(t *testing.T) {
	fsys := New()
	fsys.WriteFile("/src/main.go", []byte("package main"), 0o644)
	fsys.WriteFile("/src/handler/handler.go", []byte("package handler"), 0o644)
	fsys.WriteFile("/src/model/model.go", []byte("package model"), 0o644)

	entries, err := fsys.ReadDir("/src")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].Name() != "handler" || !entries[0].IsDir() {
		t.Fatalf("expected [0]=dir 'handler', got %s (isDir=%v)", entries[0].Name(), entries[0].IsDir())
	}
	if entries[1].Name() != "main.go" || entries[1].IsDir() {
		t.Fatalf("expected [1]=file 'main.go', got %s (isDir=%v)", entries[1].Name(), entries[1].IsDir())
	}
	if entries[2].Name() != "model" || !entries[2].IsDir() {
		t.Fatalf("expected [2]=dir 'model', got %s (isDir=%v)", entries[2].Name(), entries[2].IsDir())
	}
}

func TestReadDir_EmptyDirectory(t *testing.T) {
	fsys := New()

	entries, err := fsys.ReadDir("/empty")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestReadDir_RootDirectory(t *testing.T) {
	fsys := New()
	fsys.WriteFile("/go.mod", []byte("module example.com/test"), 0o644)
	fsys.WriteFile("/internal/main.go", []byte("package main"), 0o644)

	entries, err := fsys.ReadDir("/")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Name() != "go.mod" || entries[0].IsDir() {
		t.Fatalf("expected [0]=file 'go.mod', got %s (isDir=%v)", entries[0].Name(), entries[0].IsDir())
	}
	if entries[1].Name() != "internal" || !entries[1].IsDir() {
		t.Fatalf("expected [1]=dir 'internal', got %s (isDir=%v)", entries[1].Name(), entries[1].IsDir())
	}
}

func TestReadDir_SortedOrder(t *testing.T) {
	fsys := New()
	fsys.WriteFile("/z.txt", []byte("z"), 0o644)
	fsys.WriteFile("/a.txt", []byte("a"), 0o644)
	fsys.WriteFile("/m.txt", []byte("m"), 0o644)
	fsys.WriteFile("/dir/sub.txt", []byte("sub"), 0o644)

	entries, err := fsys.ReadDir("/")
	if err != nil {
		t.Fatal(err)
	}

	expected := []struct {
		name  string
		isDir bool
	}{
		{"a.txt", false},
		{"dir", true},
		{"m.txt", false},
		{"z.txt", false},
	}

	if len(entries) != len(expected) {
		t.Fatalf("expected %d entries, got %d", len(expected), len(entries))
	}

	for i, exp := range expected {
		if entries[i].Name() != exp.name || entries[i].IsDir() != exp.isDir {
			t.Fatalf("entry[%d]: expected %s (isDir=%v), got %s (isDir=%v)",
				i, exp.name, exp.isDir, entries[i].Name(), entries[i].IsDir())
		}
	}
}

func TestReadDir_DeeplyNestedMultipleBranches(t *testing.T) {
	fsys := New()
	fsys.WriteFile("/a/b/c/d/file.txt", []byte("deep"), 0o644)
	fsys.WriteFile("/a/x/y/file2.txt", []byte("deep2"), 0o644)

	entries, err := fsys.ReadDir("/a")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Should find "b" and "x" as directories.
	names := make(map[string]bool)
	for _, e := range entries {
		if !e.IsDir() {
			t.Fatalf("expected all dirs, got file %s", e.Name())
		}
		names[e.Name()] = true
	}
	if !names["b"] || !names["x"] {
		t.Fatalf("expected dirs b and x, got %v", names)
	}

	// b/c should lead to d.
	entries, err = fsys.ReadDir("/a/b/c")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "d" {
		t.Fatalf("expected [d], got %v", entries)
	}
}
