package overlayfs

import (
	"io/fs"
	"testing"
	"time"

	"github.com/dariushalipour/baft/internal/port"
)

func TestOverlayFS_ReadDir_WithMemoryFiles(t *testing.T) {
	lower := &mockLower{}
	fsys := New(lower, map[string][]byte{
		"/root/overlay.txt":   []byte("overlay content"),
		"/root/deep/file.txt": []byte("deep overlay"),
	})

	entries, err := fsys.ReadDir("/root")
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["overlay.txt"] {
		t.Fatal("expected 'overlay.txt' in ReadDir results")
	}
}

func TestOverlayFS_WalkDir_WithMemoryFiles(t *testing.T) {
	lower := &mockLower{}
	fsys := New(lower, map[string][]byte{
		"/root/a.txt":   []byte("aaa"),
		"/root/b/c.txt": []byte("ccc"),
	})

	var paths []string
	err := fsys.WalkDir("/root", func(abs string, d fs.DirEntry) error {
		paths = append(paths, abs)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, p := range paths {
		if p == "/root/a.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /root/a.txt in walk, got: %v", paths)
	}
}

func TestOverlayFS_ReadFile_MemoryFile(t *testing.T) {
	lower := &mockLower{}
	fsys := New(lower, map[string][]byte{
		"/root/secret.txt": []byte("top secret"),
	})

	data, err := fsys.ReadFile("/root/secret.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "top secret" {
		t.Fatalf("expected 'top secret', got '%s'", data)
	}
}

func TestOverlayFS_ReadFile_DelegatesToLower(t *testing.T) {
	lower := &mockLower{
		readFileFn: func(path string) ([]byte, error) {
			return []byte("lower content"), nil
		},
	}
	fsys := New(lower, map[string][]byte{
		"/root/other.txt": []byte("overlay"),
	})

	data, err := fsys.ReadFile("/root/other.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "overlay" {
		t.Fatalf("expected 'overlay', got '%s'", data)
	}
}

func TestOverlayFS_WalkDir_SortedOutput(t *testing.T) {
	lower := &mockLower{
		readDirFn: func(path string) ([]fs.DirEntry, error) {
			if path == "/root" {
				return []fs.DirEntry{
					&mockEntry{name: "z.txt", isDir: false},
					&mockEntry{name: "a.txt", isDir: false},
				}, nil
			}
			return nil, nil
		},
		walkDirFn: func(root string, fn func(string, fs.DirEntry) error) error {
			if root == "/root" {
				_ = fn("/root/a.txt", &mockEntry{name: "a.txt", isDir: false})
				_ = fn("/root/z.txt", &mockEntry{name: "z.txt", isDir: false})
				return nil
			}
			return nil
		},
	}
	fsys := New(lower, map[string][]byte{
		"/root/m.txt": []byte("middle"),
	})

	var names []string
	err := fsys.WalkDir("/root", func(abs string, d fs.DirEntry) error {
		names = append(names, d.Name())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should include all 3 files sorted
	if len(names) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(names), names)
	}
	if names[0] != "a.txt" || names[1] != "m.txt" || names[2] != "z.txt" {
		t.Fatalf("expected sorted [a.txt, m.txt, z.txt], got %v", names)
	}
}

func TestOverlayFS_Stat_MemoryFile(t *testing.T) {
	lower := &mockLower{}
	fsys := New(lower, map[string][]byte{
		"/root/stats.txt": []byte("data"),
	})

	info, err := fsys.Stat("/root/stats.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name() != "stats.txt" {
		t.Fatalf("expected name 'stats.txt', got '%s'", info.Name())
	}
	if info.Size() != 4 {
		t.Fatalf("expected size 4, got %d", info.Size())
	}
	if info.IsDir() {
		t.Fatal("expected file, not directory")
	}
}

type mockLower struct {
	readFileFn func(path string) ([]byte, error)
	readDirFn  func(path string) ([]fs.DirEntry, error)
	walkDirFn  func(root string, fn func(string, fs.DirEntry) error) error
}

func (m *mockLower) ReadFile(path string) ([]byte, error) {
	if m.readFileFn != nil {
		return m.readFileFn(path)
	}
	return nil, &fs.PathError{Op: "readfile", Path: path, Err: nil}
}

func (m *mockLower) WriteFile(path string, data []byte, perm fs.FileMode) error {
	return nil
}

func (m *mockLower) Stat(path string) (fs.FileInfo, error) {
	return nil, &fs.PathError{Op: "stat", Path: path, Err: nil}
}

func (m *mockLower) ReadDir(path string) ([]fs.DirEntry, error) {
	if m.readDirFn != nil {
		return m.readDirFn(path)
	}
	return nil, nil
}

func (m *mockLower) WalkDir(root string, fn func(string, fs.DirEntry) error) error {
	if m.walkDirFn != nil {
		return m.walkDirFn(root, fn)
	}
	return nil
}

var _ port.FileSystem = (*mockLower)(nil)

type mockEntry struct {
	name  string
	isDir bool
}

func (e *mockEntry) Name() string               { return e.name }
func (e *mockEntry) IsDir() bool                { return e.isDir }
func (e *mockEntry) Type() fs.FileMode          { return 0 }
func (e *mockEntry) Info() (fs.FileInfo, error) { return &mockInfo{name: e.name, isDir: e.isDir}, nil }

type mockInfo struct {
	name  string
	isDir bool
}

func (i *mockInfo) Name() string       { return i.name }
func (i *mockInfo) Size() int64        { return 0 }
func (i *mockInfo) Mode() fs.FileMode  { return 0 }
func (i *mockInfo) ModTime() time.Time { return time.Time{} }
func (i *mockInfo) IsDir() bool        { return i.isDir }
func (i *mockInfo) Sys() interface{}   { return nil }
