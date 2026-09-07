package walk_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/internal/walk"
	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/fs/overlayfs"
	"github.com/dariushalipour/baft/internal/adapter/fs/realfs"
)

const memRoot = "/root"

var tree = map[string]string{
	"a/one.txt":       "1",
	"a/sub/three.txt": "3",
	"a/two.txt":       "2",
	"b/four.txt":      "4",
}

func memTree(t *testing.T) *memfs.FS {
	t.Helper()
	fsys := memfs.New()
	for rel, content := range tree {
		if err := fsys.WriteFile(path.Join(memRoot, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return fsys
}

func collect(t *testing.T, fsys walk.DirReader, root string) []string {
	t.Helper()
	var got []string
	err := walk.Dir(context.Background(), fsys, root, func(abs string, d fs.DirEntry) error {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel += "/"
		}
		got = append(got, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	return got
}

// Every adapter delegates to the shared walker, so all three must report the
// same tree.
func TestDirAdaptersAgree(t *testing.T) {
	realRoot := t.TempDir()
	overlay := make(map[string][]byte, len(tree))
	for rel, content := range tree {
		overlay[path.Join(memRoot, rel)] = []byte(content)
		abs := filepath.Join(realRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	want := []string{"a/", "a/one.txt", "a/sub/", "a/sub/three.txt", "a/two.txt", "b/", "b/four.txt"}
	for name, got := range map[string][]string{
		"memfs":     collect(t, memTree(t), memRoot),
		"realfs":    collect(t, realfs.New(), realRoot),
		"overlayfs": collect(t, overlayfs.New(memfs.New(), overlay), memRoot),
	} {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s walked %v, want %v", name, got, want)
		}
	}
}

// SkipDir on a file skips the rest of its directory, not the whole walk.
func TestDirSkipDirOnFile(t *testing.T) {
	var visited []string
	err := walk.Dir(context.Background(), memTree(t), memRoot, func(abs string, d fs.DirEntry) error {
		visited = append(visited, abs)
		if filepath.Base(abs) == "one.txt" {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/root/a", "/root/a/one.txt", "/root/b", "/root/b/four.txt"}
	if !reflect.DeepEqual(visited, want) {
		t.Errorf("visited %v, want %v", visited, want)
	}
}

func TestDirStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := walk.Dir(ctx, memTree(t), memRoot, func(string, fs.DirEntry) error {
		calls++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Errorf("called fn %d times on a canceled context, want 0", calls)
	}
}
