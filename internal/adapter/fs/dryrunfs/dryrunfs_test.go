package dryrunfs

import (
	"context"
	"io/fs"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/port"
)

const (
	existing = "/repo/old.md"
	pending  = "/repo/new.md"
)

func base(t *testing.T) *memfs.FS {
	t.Helper()
	fsys := memfs.New()
	if err := fsys.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	return fsys
}

// A write never reaches the wrapped filesystem, yet the pass that made it
// still reads and stats what it wrote.
func TestWriteIsBufferedButVisible(t *testing.T) {
	lower := base(t)
	fsys := Wrap(lower)
	if err := fsys.WriteFile(pending, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := lower.Stat(pending); err == nil {
		t.Errorf("%s reached the wrapped filesystem", pending)
	}
	if _, err := fsys.Stat(pending); err != nil {
		t.Errorf("Stat(%q) = %v, want the buffered write", pending, err)
	}
	if data, err := fsys.ReadFile(pending); err != nil || string(data) != "new" {
		t.Errorf("ReadFile(%q) = (%q, %v), want (%q, nil)", pending, data, err, "new")
	}
	if data, err := fsys.ReadFile(existing); err != nil || string(data) != "old" {
		t.Errorf("ReadFile(%q) = (%q, %v), want (%q, nil)", existing, data, err, "old")
	}
}

// Directory listings are not overlaid: a buffered write stays out of ReadDir
// and WalkDir.
func TestDirListingsAreNotOverlaid(t *testing.T) {
	fsys := Wrap(base(t))
	if err := fsys.WriteFile(pending, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := fsys.ReadDir("/repo")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "old.md" {
		t.Errorf("ReadDir = %v, want [old.md]", names)
	}

	var walked []string
	if err := fsys.WalkDir(context.Background(), "/repo", func(abs string, _ fs.DirEntry) error {
		walked = append(walked, abs)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(walked) != 1 || walked[0] != existing {
		t.Errorf("WalkDir = %v, want [%s]", walked, existing)
	}
}

// IsIgnored forwards to the wrapped filesystem, and reports false when the
// wrapped filesystem has no ignore rules at all.
func TestIsIgnoredIsForwarded(t *testing.T) {
	if ig := Wrap(&ignoring{FS: base(t)}).(port.IgnoreLookup); !ig.IsIgnored(existing) || ig.IsIgnored(pending) {
		t.Errorf("IsIgnored(%q, %q) = (%v, %v), want (true, false)", existing, pending, ig.IsIgnored(existing), ig.IsIgnored(pending))
	}
	if ig := Wrap(base(t)).(port.IgnoreLookup); ig.IsIgnored(existing) {
		t.Errorf("IsIgnored(%q) = true for a filesystem without ignore rules", existing)
	}
}

// ignoring is a filesystem that ignores every path it holds.
type ignoring struct {
	*memfs.FS
}

func (i *ignoring) IsIgnored(path string) bool {
	_, err := i.FS.Stat(path)
	return err == nil
}
