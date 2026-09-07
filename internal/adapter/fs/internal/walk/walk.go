// Package walk holds the single directory walker shared by every filesystem
// adapter, so an adapter only has to provide ReadDir.
//
// Symlinks are never followed: a symlinked directory is reported as a plain
// (non-directory) entry and its contents are not visited.
package walk

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
)

// DirReader is the only capability the walker needs from an adapter.
type DirReader interface {
	ReadDir(name string) ([]fs.DirEntry, error)
}

// Dir walks the tree rooted at root, calling fn for every entry beneath it
// (root itself is not visited) and honouring context cancellation at every
// directory boundary. Returning fs.SkipDir from fn skips that directory's
// subtree, or the rest of the containing directory when fn was given a file.
// Like filepath.WalkDir, Dir never surfaces fs.SkipDir to its caller: a skip
// asked for at the top level ends the walk without an error.
func Dir(ctx context.Context, fsys DirReader, root string, fn func(abs string, d fs.DirEntry) error) error {
	if err := walkDir(ctx, fsys, root, fn); err != nil && !errors.Is(err, fs.SkipDir) {
		return err
	}
	return nil
}

func walkDir(ctx context.Context, fsys DirReader, root string, fn func(abs string, d fs.DirEntry) error) error {
	entries, err := fsys.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		abs := filepath.Join(root, entry.Name())
		err := fn(abs, entry)
		if err == nil && entry.IsDir() {
			err = walkDir(ctx, fsys, abs, fn)
		}
		if err != nil {
			if entry.IsDir() && errors.Is(err, fs.SkipDir) {
				continue
			}
			return err
		}
	}
	return nil
}
