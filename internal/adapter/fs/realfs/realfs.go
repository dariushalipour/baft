package realfs

import (
	"context"
	"io/fs"
	"os"

	"github.com/dariushalipour/baft/internal/adapter/fs/internal/walk"
)

type FS struct{}

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

func (f *FS) WalkDir(ctx context.Context, root string, fn func(abs string, d fs.DirEntry) error) error {
	return walk.Dir(ctx, f, root, fn)
}
