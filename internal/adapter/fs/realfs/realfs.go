package realfs

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
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
	return walkDirCtx(ctx, root, fn)
}

func walkDirCtx(ctx context.Context, dir string, fn func(abs string, d fs.DirEntry) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		abs := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if err := fn(abs, entry); err != nil {
				return err
			}
			if err := walkDirCtx(ctx, abs, fn); err != nil {
				return err
			}
		} else {
			if err := fn(abs, entry); err != nil {
				return err
			}
		}
	}
	return nil
}
