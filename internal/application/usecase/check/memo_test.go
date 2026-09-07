package check

import (
	"io/fs"
	"os"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/port"
)

type countingFS struct {
	port.FileSystem
	stats    int
	readDirs int
}

func (c *countingFS) Stat(path string) (os.FileInfo, error) {
	c.stats++
	return c.FileSystem.Stat(path)
}

func (c *countingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	c.readDirs++
	return c.FileSystem.ReadDir(name)
}

func countingChecker(t *testing.T, files map[string]string) (*capsuleChecker, *countingFS) {
	t.Helper()
	mem := memfs.New()
	for path, content := range files {
		if err := mem.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fsys := &countingFS{FileSystem: mem}
	return &capsuleChecker{fsys: fsys, capsule: port.Capsule{Dir: "/capsule"}}, fsys
}

// The memo is keyed by directory, which is only sound because TrackingScope
// depends on nothing but the file's directory and the fixed capsule dir.
func TestTrackingScopeMemoizesPerDirectory(t *testing.T) {
	ch, fsys := countingChecker(t, map[string]string{
		"/capsule/BAFT.md":     "# capsule",
		"/capsule/sub/BAFT.md": "# sub",
		"/capsule/sub/a.go":    "package sub",
		"/capsule/sub/b.go":    "package sub",
		"/capsule/c.go":        "package capsule",
	})

	if got := ch.trackingScope("/capsule/sub/a.go"); got != "/capsule/sub" {
		t.Fatalf("trackingScope(a.go) = %q, want /capsule/sub", got)
	}
	stats := fsys.stats
	if stats == 0 {
		t.Fatal("first lookup must consult the filesystem")
	}

	if got := ch.trackingScope("/capsule/sub/b.go"); got != "/capsule/sub" {
		t.Errorf("trackingScope(b.go) = %q, want /capsule/sub", got)
	}
	if fsys.stats != stats {
		t.Errorf("sibling lookup cost %d stats, want a memo hit", fsys.stats-stats)
	}

	if got := ch.trackingScope("/capsule/c.go"); got != "/capsule" {
		t.Errorf("trackingScope(c.go) = %q, want /capsule", got)
	}
	if fsys.stats == stats {
		t.Error("a different directory must be computed, not served from the memo")
	}
}

func TestTargetVisibleMemoizesPerTarget(t *testing.T) {
	ch, fsys := countingChecker(t, map[string]string{"/capsule/pkg/a.go": "package pkg"})

	if !ch.targetVisible("/capsule/pkg") {
		t.Fatal("a non-empty directory must be visible")
	}
	if fsys.readDirs != 1 {
		t.Fatalf("first probe read %d directories, want 1", fsys.readDirs)
	}

	if !ch.targetVisible("/capsule/pkg") {
		t.Error("repeated probe must agree with the first")
	}
	if fsys.readDirs != 1 {
		t.Errorf("repeated probe read %d directories, want a memo hit", fsys.readDirs)
	}
}
