package port

import (
	"context"
	"io/fs"
	"os"
)

// FileSystem abstracts all file system operations so the core logic
// never touches the real disk. Implementations may be backed by the
// real OS or by an in-memory store for testing.
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	Stat(path string) (os.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)

	// WalkDir walks the file tree rooted at root, calling fn for each
	// file or directory. It respects context cancellation at every directory
	// boundary, allowing long walks to be aborted promptly. Symlinks are not
	// followed: a symlinked directory is reported as a non-directory entry
	// and is never descended into.
	WalkDir(ctx context.Context, root string, fn func(abs string, d fs.DirEntry) error) error
}

// IgnoreLookup is implemented by filesystems that hide paths matched by
// .gitignore/.baftignore rules. It tells a deliberately ignored path apart
// from one that simply does not exist, which Stat alone cannot do.
type IgnoreLookup interface {
	IsIgnored(path string) bool
}

// IsTargetVisible reports whether an import target belongs to the checked
// tree. Ignored paths are invisible, and so is a directory whose entries are
// all ignored. A target the filesystem cannot resolve at all stays visible:
// extensionless Kotlin and Python targets have no on-disk counterpart.
func IsTargetVisible(fsys FileSystem, targetAbs string) bool {
	if ig, ok := fsys.(IgnoreLookup); ok && ig.IsIgnored(targetAbs) {
		return false
	}
	info, err := fsys.Stat(targetAbs)
	if err != nil || !info.IsDir() {
		return true
	}
	entries, readErr := fsys.ReadDir(targetAbs)
	return readErr != nil || len(entries) > 0
}
