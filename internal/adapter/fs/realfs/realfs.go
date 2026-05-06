package realfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dariushalipour/strata/internal/adapter/fs/realfs/gitignore"
	"github.com/dariushalipour/strata/internal/port"
)

// FS is a FileSystem backed by the real operating system.
type FS struct {
	gitMatcher    gitignore.Matcher
	cacheMu       sync.RWMutex
	ignoreCache   map[string]bool
	repoRoot      string
	patternsReady bool
}

// New returns a FileSystem that wraps the real OS file system.
func New() *FS {
	return &FS{
		ignoreCache: make(map[string]bool),
	}
}

func (f *FS) findRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}

	for {
		info, err := os.Stat(filepath.Join(dir, ".git"))
		if err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", &port.NotGitRepoError{Path: start}
		}
		dir = parent
	}
}

func (f *FS) ensurePatternsLoaded(root string) {
	f.cacheMu.Lock()
	defer f.cacheMu.Unlock()

	if f.patternsReady {
		return
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return
	}

	// Try to find a git repo root first
	repoRoot, err := f.findRepoRoot(root)
	if err != nil {
		// No git repo — fall back to the directory containing the path
		repoRoot = filepath.Dir(root)
	}

	f.repoRoot = repoRoot

	patterns, err := gitignore.ReadPatterns(repoRoot)
	if err == nil && len(patterns) > 0 {
		f.gitMatcher = gitignore.NewMatcher(patterns)
	}

	f.patternsReady = true
}

func (f *FS) ReadFile(path string) ([]byte, error) {
	f.ensurePatternsLoaded(path)
	if f.isIgnored(path) {
		return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
	}
	return os.ReadFile(path)
}

func (f *FS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (f *FS) Stat(path string) (os.FileInfo, error) {
	f.ensurePatternsLoaded(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if f.isIgnored(path) {
		return nil, &fs.PathError{Op: "stat", Path: path, Err: fs.ErrNotExist}
	}
	return info, nil
}

func (f *FS) isIgnored(path string) bool {
	if f.gitMatcher == nil {
		return false
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	repoRoot := f.repoRoot
	if repoRoot == "" {
		repoRoot, err = f.findRepoRoot(abs)
		if err != nil {
			return false
		}
	}

	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)

	f.cacheMu.RLock()
	if ignored, ok := f.ignoreCache[rel]; ok {
		f.cacheMu.RUnlock()
		return ignored
	}
	f.cacheMu.RUnlock()

	pathParts := strings.Split(rel, "/")
	isDir := false
	info, statErr := os.Stat(abs)
	if statErr == nil {
		isDir = info.IsDir()
	}

	ignored := f.gitMatcher.Match(pathParts, isDir)

	f.cacheMu.Lock()
	f.ignoreCache[rel] = ignored
	f.cacheMu.Unlock()

	return ignored
}

func (f *FS) WalkDir(root string, fn func(abs string, d fs.DirEntry) error) error {
	f.ensurePatternsLoaded(root)

	return filepath.WalkDir(root, func(abs string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() && port.ShouldSkipDir(d.Name()) {
			return fs.SkipDir
		}
		if f.isIgnored(abs) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		return fn(abs, d)
	})
}
