package port

import (
	"io/fs"
	"os"
	"path/filepath"
)

// FileSystem abstracts all file system operations so the core logic
// never touches the real disk. Implementations may be backed by the
// real OS or by an in-memory store for testing.
type FileSystem interface {
	// ReadFile returns the contents of the named file.
	ReadFile(path string) ([]byte, error)

	// WriteFile writes data to the named file.
	WriteFile(path string, data []byte, perm os.FileMode) error

	// Stat returns a FileInfo describing the named file.
	Stat(path string) (os.FileInfo, error)

	// ReadDir lists the contents of the named directory, excluding ignored entries.
	ReadDir(name string) ([]fs.DirEntry, error)

	// WalkDir walks the file tree rooted at root, calling fn for each
	// file or directory. It skips ignored paths.
	WalkDir(root string, fn func(abs string, d fs.DirEntry) error) error
}

// NotGitRepoError is returned when a directory is not inside a git
// repository.
type NotGitRepoError struct {
	Path string
}

func (e *NotGitRepoError) Error() string {
	return "not inside a git repository: " + e.Path
}

// IsTargetVisible reports whether a target path is visible through the
// filesystem (i.e. not baftignored).
//
// When fsys is an ignorefs-wrapped FileSystem (the typical case), ignored
// paths return fs.ErrNotExist from Stat. The function checks:
//  1. The target itself is statable (file or directory with entries).
//  2. A ".go" variant exists for directory-level targets (e.g. orphan.go).
//
// A directory whose entries are all ignored is considered invisible.
func IsTargetVisible(fsys FileSystem, targetAbs string) bool {
	info, statErr := fsys.Stat(targetAbs)
	if statErr == nil {
		if !info.IsDir() {
			return true
		}
		// For directories, check if any entries are visible.
		entries, readErr := fsys.ReadDir(targetAbs)
		if readErr == nil && len(entries) == 0 {
			return false
		}
		return true
	}

	base := filepath.Base(targetAbs)
	goFile := targetAbs + "/" + base + ".go"
	if _, err := fsys.Stat(goFile); err == nil {
		return true
	}

	return true
}
