package port

import (
	"io/fs"
	"os"
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

	// WalkDir walks the file tree rooted at root, calling fn for each
	// file or directory. It skips well-known directories (vendor, .git,
	// node_modules, etc.) and gitignored paths automatically.
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
