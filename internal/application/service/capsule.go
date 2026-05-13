package service

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dariushalipour/baft/internal/port"
)

// WalkCapsule walks a capsule directory, skipping hidden/vendor dirs,
// nested capsules, non-scannable files, and gitignored paths. For each
// scannable file it calls fn with the absolute path and the
// capsule-relative path (forward-slash).
func WalkCapsule(fsys port.FileSystem, capsuleDir string, lang port.Language, fn func(abs, rel string) error) error {
	if _, err := fsys.Stat(capsuleDir); err != nil {
		return err
	}
	return fsys.WalkDir(capsuleDir, func(abs string, d fs.DirEntry) error {
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

// WalkAllFiles walks a capsule directory including child BAFT.md
// directories. Hidden/vendor dirs and non-scannable files are still
// skipped. For each scannable file it calls fn with the absolute path
// and the capsule-relative path (forward-slash).
func WalkAllFiles(fsys port.FileSystem, capsuleDir string, lang port.Language, fn func(abs, rel string) error) error {
	if _, err := fsys.Stat(capsuleDir); err != nil {
		return err
	}
	return fsys.WalkDir(capsuleDir, func(abs string, d fs.DirEntry) error {
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
// BAFT.md for the given file, bounded by capsuleDir. Returns capsuleDir
// if no child BAFT.md is found.
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

// FindContract walks upward from startDir toward capsuleDir looking for
// BAFT.md. It returns the absolute path to the nearest ancestor
// BAFT.md, or capsuleDir/BAFT.md if none is found.
func FindContract(fsys port.FileSystem, startDir string, capsuleDir string) string {
	dir := startDir
	for {
		contractPath := filepath.Join(dir, port.ContractFile)
		if _, err := fsys.Stat(contractPath); err == nil {
			return contractPath
		}
		if dir == capsuleDir {
			return filepath.Join(capsuleDir, port.ContractFile)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Join(capsuleDir, port.ContractFile)
		}
		dir = parent
	}
}

// FindOrCreateContractDir walks upward from startDir toward capsuleDir.
// If BAFT.md already exists in any directory along the way, it
// returns that directory (contract exists). If no BAFT.md is found,
// it returns startDir (contract should be created there). The second
// return value is true if BAFT.md already exists.
func FindOrCreateContractDir(fsys port.FileSystem, startDir string, capsuleDir string) (contractDir string, exists bool) {
	dir := startDir
	for {
		contractPath := filepath.Join(dir, port.ContractFile)
		if _, err := fsys.Stat(contractPath); err == nil {
			return dir, true
		}
		if dir == capsuleDir {
			return startDir, false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return startDir, false
		}
		dir = parent
	}
}
