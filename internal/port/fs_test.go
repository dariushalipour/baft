package port

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"time"
)

// mockFS implements FileSystem for testing IsTargetVisible.
type mockFS struct {
	statFn    func(path string) (os.FileInfo, error)
	readDirFn func(path string) ([]fs.DirEntry, error)
}

func (m *mockFS) ReadFile(string) ([]byte, error) { return nil, nil }

func (m *mockFS) WriteFile(string, []byte, os.FileMode) error { return nil }

func (m *mockFS) Stat(path string) (os.FileInfo, error) {
	if m.statFn != nil {
		return m.statFn(path)
	}
	return nil, fs.ErrNotExist
}

func (m *mockFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if m.readDirFn != nil {
		return m.readDirFn(name)
	}
	return nil, fs.ErrNotExist
}

func (m *mockFS) WalkDir(context.Context, string, func(string, fs.DirEntry) error) error { return nil }

type mockFileInfo struct {
	name  string
	isDir bool
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() os.FileMode  { return 0 }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return m.isDir }
func (m mockFileInfo) Sys() any           { return nil }

type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string { return m.name }
func (m *mockDirEntry) IsDir() bool  { return m.isDir }
func (m *mockDirEntry) Type() fs.FileMode {
	if m.isDir {
		return fs.ModeDir
	}
	return 0
}
func (m *mockDirEntry) Info() (os.FileInfo, error) { return nil, nil }

// ignoringFS reports one path as ignored, as an ignorefs-wrapped FileSystem does.
type ignoringFS struct {
	*mockFS
	ignored string
}

func (f *ignoringFS) IsIgnored(path string) bool { return path == f.ignored }

func TestIsTargetVisible(t *testing.T) {
	dir := func(entries ...fs.DirEntry) *mockFS {
		return &mockFS{
			statFn:    func(string) (os.FileInfo, error) { return mockFileInfo{name: "dir", isDir: true}, nil },
			readDirFn: func(string) ([]fs.DirEntry, error) { return entries, nil },
		}
	}

	t.Run("ignored target is invisible, unknown target is not", func(t *testing.T) {
		fsys := &ignoringFS{mockFS: &mockFS{}, ignored: "/example/gen"}
		if IsTargetVisible(fsys, "/example/gen") {
			t.Fatal("expected ignored target to be invisible")
		}
		if !IsTargetVisible(fsys, "/example/unknown") {
			t.Fatal("expected unresolvable target to stay visible")
		}
	})

	t.Run("file target is visible", func(t *testing.T) {
		fsys := &mockFS{
			statFn: func(string) (os.FileInfo, error) { return mockFileInfo{name: "config.yaml"}, nil },
		}
		if !IsTargetVisible(fsys, "/example/config.yaml") {
			t.Fatal("expected visible")
		}
	})

	t.Run("directory with entries is visible", func(t *testing.T) {
		if !IsTargetVisible(dir(&mockDirEntry{name: "file.go"}), "/example/internal") {
			t.Fatal("expected visible")
		}
	})

	t.Run("directory whose entries are all ignored is invisible", func(t *testing.T) {
		if IsTargetVisible(dir(), "/example/gen") {
			t.Fatal("expected empty directory to be invisible")
		}
	})

	t.Run("visible when ReadDir fails (conservative fallback)", func(t *testing.T) {
		fsys := dir()
		fsys.readDirFn = func(string) ([]fs.DirEntry, error) { return nil, fs.ErrNotExist }
		if !IsTargetVisible(fsys, "/example/dir") {
			t.Fatal("expected visible (conservative on error)")
		}
	})
}
