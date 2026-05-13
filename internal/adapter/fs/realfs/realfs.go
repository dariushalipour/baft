package realfs

import (
	"io/fs"
	"os"
	"path/filepath"
)

// FS is a FileSystem backed by the real operating system.
type FS struct{}

// New returns a FileSystem that wraps the real OS file system.
func New() *FS {
	return &FS{}
}

func (f *FS) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (f *FS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (f *FS) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (f *FS) WalkDir(root string, fn func(abs string, d fs.DirEntry) error) error {
	return filepath.WalkDir(root, func(abs string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		return fn(abs, d)
	})
}
