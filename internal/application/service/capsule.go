package service

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dariushalipour/strata/internal/port"
)

// WalkCapsule walks a capsule directory, skipping hidden/vendor dirs,
// nested capsules, non-governed files, and gitignored paths. For each
// governed file it calls fn with the absolute path and the
// capsule-relative path (forward-slash).
func WalkCapsule(fsys port.FileSystem, capsuleDir string, lang port.Language, fn func(abs, rel string) error) error {
	if _, err := fsys.Stat(capsuleDir); err != nil {
		return err
	}
	return fsys.WalkDir(capsuleDir, func(abs string, d fs.DirEntry) error {
		if d.IsDir() {
			if abs != capsuleDir {
				_, err := fsys.Stat(filepath.Join(abs, port.ConfigFile))
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
		if !lang.IsGovernedFile(rel) {
			return nil
		}
		return fn(abs, rel)
	})
}

func isNotExist(err error) bool {
	return os.IsNotExist(err)
}

// WalkAllFiles walks a capsule directory including child STRATA.md
// directories. Hidden/vendor dirs and non-governed files are still
// skipped. For each governed file it calls fn with the absolute path
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
		if !lang.IsGovernedFile(rel) {
			return nil
		}
		return fn(abs, rel)
	})
}

// GoverningScope returns the directory of the nearest ancestor
// STRATA.md for the given file, bounded by capsuleDir. Returns capsuleDir
// if no child STRATA.md is found.
func GoverningScope(fsys port.FileSystem, absFile string, capsuleDir string) string {
	dir := filepath.Dir(absFile)
	for {
		if _, err := fsys.Stat(filepath.Join(dir, port.ConfigFile)); err == nil {
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

// FindConfig walks upward from startDir toward capsuleDir looking for
// STRATA.md. It returns the absolute path to the nearest ancestor
// STRATA.md, or capsuleDir/STRATA.md if none is found.
func FindConfig(fsys port.FileSystem, startDir string, capsuleDir string) string {
	dir := startDir
	for {
		cfg := filepath.Join(dir, port.ConfigFile)
		if _, err := fsys.Stat(cfg); err == nil {
			return cfg
		}
		if dir == capsuleDir {
			return filepath.Join(capsuleDir, port.ConfigFile)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Join(capsuleDir, port.ConfigFile)
		}
		dir = parent
	}
}

// FindOrCreateConfigDir walks upward from startDir toward capsuleDir.
// If STRATA.md already exists in any directory along the way, it
// returns that directory (config exists). If no STRATA.md is found,
// it returns startDir (config should be created there). The second
// return value is true if STRATA.md already exists.
func FindOrCreateConfigDir(fsys port.FileSystem, startDir string, capsuleDir string) (configDir string, exists bool) {
	dir := startDir
	for {
		cfg := filepath.Join(dir, port.ConfigFile)
		if _, err := fsys.Stat(cfg); err == nil {
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
