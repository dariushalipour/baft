package service

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dariushalipour/baft/internal/port"
)

// WalkCapsule walks a capsule directory, skipping hidden/vendor dirs,
// nested capsules, non-scannable files, and gitignored paths. For each
// scannable file it calls fn with the absolute path and the
// capsule-relative path (forward-slash).
func WalkCapsule(ctx context.Context, fsys port.FileSystem, capsuleDir string, lang port.Language, fn func(abs, rel string) error) error {
	if _, err := fsys.Stat(capsuleDir); err != nil {
		return err
	}
	return fsys.WalkDir(ctx, capsuleDir, func(abs string, d fs.DirEntry) error {
		if d.IsDir() {
			if abs != capsuleDir {
				_, err := fsys.Stat(filepath.Join(abs, port.ContractFile))
				if err == nil {
					return fs.SkipDir
				}
				if !isNotExist(err) {
					return err
				}
			}
			return nil
		}
		rel, err := filepath.Rel(capsuleDir, abs)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !lang.IsScannableFile(rel) {
			return nil
		}
		return fn(abs, rel)
	})
}

func isNotExist(err error) bool {
	return os.IsNotExist(err)
}

// WalkAllFiles walks a capsule directory including child contract file
// directories. Hidden/vendor dirs and non-scannable files are still
// skipped. For each scannable file it calls fn with the absolute path
// and the capsule-relative path (forward-slash).
func WalkAllFiles(ctx context.Context, fsys port.FileSystem, capsuleDir string, lang port.Language, fn func(abs, rel string) error) error {
	if _, err := fsys.Stat(capsuleDir); err != nil {
		return err
	}
	return fsys.WalkDir(ctx, capsuleDir, func(abs string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(capsuleDir, abs)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !lang.IsScannableFile(rel) {
			return nil
		}
		return fn(abs, rel)
	})
}

// TrackingScope returns the directory of the nearest ancestor
// contract file for the given file, bounded by capsuleDir. Returns capsuleDir
// if no child contract file is found.
func TrackingScope(fsys port.FileSystem, absFile string, capsuleDir string) string {
	dir := filepath.Dir(absFile)
	for {
		if _, err := fsys.Stat(filepath.Join(dir, port.ContractFile)); err == nil {
			return dir
		}
		if dir == capsuleDir {
			return capsuleDir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return capsuleDir
		}
		dir = parent
	}
}

// absDir returns dir in absolute, cleaned form.
func absDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	return abs
}

// contractDirFor returns the nearest directory at or above startDir holding a
// contract file. The climb never leaves capsuleDir: a contract outside the
// capsule is never adopted, while one anywhere inside it — including above a
// checked subdirectory — is. When none is found it returns startDir with false.
func contractDirFor(fsys port.FileSystem, startDir, capsuleDir string) (string, bool) {
	start := absDir(startDir)
	prefix := absDir(capsuleDir) + string(filepath.Separator)
	for dir := start; ; dir = filepath.Dir(dir) {
		if _, err := fsys.Stat(filepath.Join(dir, port.ContractFile)); err == nil {
			return dir, true
		}
		if !strings.HasPrefix(dir, prefix) {
			return start, false
		}
	}
}

// FindContract returns the absolute path of the nearest contract file at or
// above startDir, bounded by capsuleDir, or capsuleDir/BAFT.md if none exists.
func FindContract(fsys port.FileSystem, startDir string, capsuleDir string) string {
	if dir, ok := contractDirFor(fsys, startDir, capsuleDir); ok {
		return filepath.Join(dir, port.ContractFile)
	}
	return filepath.Join(absDir(capsuleDir), port.ContractFile)
}

// FindOrCreateContractDir returns the directory of the nearest contract file at
// or above startDir, bounded by capsuleDir, and whether it already exists. When
// no contract exists it returns startDir, where one should be created.
func FindOrCreateContractDir(fsys port.FileSystem, startDir string, capsuleDir string) (contractDir string, exists bool) {
	return contractDirFor(fsys, startDir, capsuleDir)
}
