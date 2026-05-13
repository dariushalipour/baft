package port

import (
	"io/fs"
	"os"
	"testing"
	"time"
)

// mockFS implements FileSystem for testing IsTargetVisible.
type mockFS struct {
	statFn      func(path string) (os.FileInfo, error)
	readDirFn   func(path string) ([]fs.DirEntry, error)
	readFileFn  func(path string) ([]byte, error)
	writeFileFn func(path string, data []byte, perm os.FileMode) error
	walkDirFn   func(root string, fn func(abs string, d fs.DirEntry) error) error
}

func (m *mockFS) ReadFile(path string) ([]byte, error) {
	if m.readFileFn != nil {
		return m.readFileFn(path)
	}
	return nil, nil
}

func (m *mockFS) WriteFile(path string, data []byte, perm os.FileMode) error {
	if m.writeFileFn != nil {
		return m.writeFileFn(path, data, perm)
	}
	return nil
}

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

func (m *mockFS) WalkDir(root string, fn func(abs string, d fs.DirEntry) error) error {
	if m.walkDirFn != nil {
		return m.walkDirFn(root, fn)
	}
	return nil
}

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

func TestIsTargetVisible(t *testing.T) {
	t.Run("visible Go file via stat", func(t *testing.T) {
		fsys := &mockFS{
			statFn: func(path string) (os.FileInfo, error) {
				return mockFileInfo{name: "orphan.go", isDir: false}, nil
			},
		}
		if !IsTargetVisible(fsys, "/example/internal/orphan") {
			t.Fatal("expected visible")
		}
	})

	t.Run("visible file target", func(t *testing.T) {
		fsys := &mockFS{
			statFn: func(path string) (os.FileInfo, error) {
				return mockFileInfo{name: "config.yaml", isDir: false}, nil
			},
		}
		if !IsTargetVisible(fsys, "/example/config.yaml") {
			t.Fatal("expected visible")
		}
	})

	t.Run("visible directory with entries", func(t *testing.T) {
		fsys := &mockFS{
			statFn: func(path string) (os.FileInfo, error) {
				return mockFileInfo{name: "internal", isDir: true}, nil
			},
			readDirFn: func(string) ([]fs.DirEntry, error) {
				return []fs.DirEntry{&mockDirEntry{name: "file.go", isDir: false}}, nil
			},
		}
		if !IsTargetVisible(fsys, "/example/internal") {
			t.Fatal("expected visible")
		}
	})

	t.Run("empty directory is visible (conservative)", func(t *testing.T) {
		fsys := &mockFS{
			statFn: func(path string) (os.FileInfo, error) {
				if path == "/example/ignored/ignored.go" {
					return nil, fs.ErrNotExist
				}
				return mockFileInfo{name: "ignored", isDir: true}, nil
			},
		}
		if !IsTargetVisible(fsys, "/example/ignored") {
			t.Fatal("expected visible (stat succeeds)")
		}
	})

	t.Run("visible when ReadDir fails (conservative fallback)", func(t *testing.T) {
		fsys := &mockFS{
			statFn: func(path string) (os.FileInfo, error) {
				return mockFileInfo{name: "dir", isDir: true}, nil
			},
			readDirFn: func(string) ([]fs.DirEntry, error) {
				return nil, fs.ErrNotExist
			},
		}
		if !IsTargetVisible(fsys, "/example/dir") {
			t.Fatal("expected visible (conservative on error)")
		}
	})

	t.Run("visible fallback when no Go file, no stat, no ReadDir", func(t *testing.T) {
		fsys := &mockFS{
			statFn:    func(path string) (os.FileInfo, error) { return nil, fs.ErrNotExist },
			readDirFn: func(string) ([]fs.DirEntry, error) { return nil, fs.ErrNotExist },
		}
		if !IsTargetVisible(fsys, "/example/unknown") {
			t.Fatal("expected visible fallback")
		}
	})

	t.Run("visible when stat succeeds and target is not a directory", func(t *testing.T) {
		fsys := &mockFS{
			statFn: func(path string) (os.FileInfo, error) {
				if path == "/example/foo.go" {
					return nil, fs.ErrNotExist
				}
				return mockFileInfo{name: "data.json", isDir: false}, nil
			},
		}
		if !IsTargetVisible(fsys, "/example/data.json") {
			t.Fatal("expected visible")
		}
	})

	t.Run("visible when directory has entries even if stat fails", func(t *testing.T) {
		fsys := &mockFS{
			statFn: func(path string) (os.FileInfo, error) {
				return nil, fs.ErrNotExist
			},
			readDirFn: func(string) ([]fs.DirEntry, error) {
				return []fs.DirEntry{
					&mockDirEntry{name: "a.go", isDir: false},
					&mockDirEntry{name: "b.go", isDir: false},
				}, nil
			},
		}
		if !IsTargetVisible(fsys, "/example/dir") {
			t.Fatal("expected visible")
		}
	})
}
