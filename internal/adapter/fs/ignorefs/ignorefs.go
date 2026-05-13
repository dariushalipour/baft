package ignorefs

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/fs/gitignore"
	"github.com/dariushalipour/baft/internal/port"
)

// ErrRepoRootUnreachable is returned when the repository root cannot be
// determined due to a filesystem error (e.g. permission denied on a parent
// directory). Ancestor ignore rules are skipped in this case.
var ErrRepoRootUnreachable = errors.New("repo root unreachable")

type Options struct {
	RootDir           string
	BaseIgnoreEntries map[string]bool
}

func Wrap(lower port.FileSystem, opts Options) (port.FileSystem, error) {
	rootDir, err := filepath.Abs(opts.RootDir)
	if err != nil {
		return nil, err
	}
	iw := &ignoreWrapper{
		lower:      lower,
		rootDir:    rootDir,
		baseIgnore: opts.BaseIgnoreEntries,
	}

	if err := iw.loadIgnoreRules(); err != nil {
		if errors.Is(err, ErrRepoRootUnreachable) {
			return iw, err
		}
		return nil, err
	}

	return iw, nil
}

type ignoreWrapper struct {
	lower           port.FileSystem
	rootDir         string
	baseIgnore      map[string]bool
	baseMatcher     gitignore.Matcher
	ancestorMatcher gitignore.Matcher
	ancestorOffset  []string
	localMatcher    gitignore.Matcher
}

func (w *ignoreWrapper) loadIgnoreRules() error {
	basePatterns := w.buildBasePatterns()
	if len(basePatterns) > 0 {
		w.baseMatcher = gitignore.NewMatcher(basePatterns)
	}

	if err := w.loadAncestorPatterns(); err != nil {
		return err
	}

	localPatterns, err := w.loadLocalPatterns(w.baseMatcher)
	if err != nil {
		return err
	}

	if len(localPatterns) > 0 {
		w.localMatcher = gitignore.NewMatcher(localPatterns)
	}

	return nil
}

func (w *ignoreWrapper) buildBasePatterns() []gitignore.Pattern {
	patterns := make([]gitignore.Pattern, 0, len(w.baseIgnore)+8)

	for _, name := range [...]string{
		".git", ".hg", ".svn",
		".idea", ".vscode", ".vs",
		"coverage",
	} {
		patterns = append(patterns, gitignore.ParsePattern(name+"/", nil))
	}
	patterns = append(patterns, gitignore.ParsePattern("coverage.lcov", nil))

	for entry := range w.baseIgnore {
		pattern := entry
		if !strings.Contains(entry, "*") {
			pattern = entry + "/"
		}
		patterns = append(patterns, gitignore.ParsePattern(pattern, nil))
	}

	return patterns
}

func (w *ignoreWrapper) loadAncestorPatterns() error {
	repoRoot, err := w.findRepoRoot()
	if err != nil {
		return err
	}

	if repoRoot == "" || filepath.Clean(repoRoot) == filepath.Clean(w.rootDir) {
		return nil
	}

	rel, err := filepath.Rel(repoRoot, w.rootDir)
	if err != nil {
		return err
	}
	w.ancestorOffset = strings.Split(filepath.ToSlash(rel), "/")

	var patterns []gitignore.Pattern

	var dirs []string
	dir := filepath.Dir(w.rootDir)
	for {
		dirs = append(dirs, dir)
		if filepath.Clean(dir) == filepath.Clean(repoRoot) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		d := dirs[i]
		rel, err := filepath.Rel(repoRoot, d)
		if err != nil {
			return err
		}
		if rel == "." {
			rel = ""
		}
		domain := strings.Split(filepath.ToSlash(rel), "/")
		if domain[0] == "" {
			domain = nil
		}

		for _, filename := range [...]string{".gitignore", ".baftignore"} {
			ps, err := w.readIgnoreFile(filepath.Join(d, filename), domain)
			if err != nil && !os.IsNotExist(err) {
				continue
			}
			patterns = append(patterns, ps...)
		}
	}

	if len(patterns) > 0 {
		w.ancestorMatcher = gitignore.NewMatcher(patterns)
	}

	return nil
}

func (w *ignoreWrapper) findRepoRoot() (string, error) {
	abs, err := filepath.Abs(w.rootDir)
	if err != nil {
		return "", err
	}

	dir := abs
	for {
		_, err := w.lower.Stat(filepath.Join(dir, ".git"))
		if err == nil {
			return dir, nil
		}
		if !os.IsNotExist(err) {
			// Actual filesystem error (e.g. permission denied) — cannot
			// determine repo root, ancestor rules cannot be applied.
			return "", ErrRepoRootUnreachable
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding .git — no git repo.
			return "", nil
		}
		dir = parent
	}
}

func (w *ignoreWrapper) loadLocalPatterns(baseMatcher gitignore.Matcher) ([]gitignore.Pattern, error) {
	return w.readIgnoreFiles(w.rootDir, nil, baseMatcher)
}

func (w *ignoreWrapper) readIgnoreFiles(root string, domain []string, baseMatcher gitignore.Matcher) ([]gitignore.Pattern, error) {
	var patterns []gitignore.Pattern

	for _, filename := range [...]string{".gitignore", ".baftignore"} {
		ps, err := w.readIgnoreFile(filepath.Join(root, filename), domain)
		if err != nil {
			continue
		}
		patterns = append(patterns, ps...)
	}

	entries, err := w.lower.ReadDir(root)
	if err != nil {
		return patterns, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		testPath := make([]string, len(domain)+1)
		copy(testPath, domain)
		testPath[len(domain)] = name

		if baseMatcher != nil && baseMatcher.Match(testPath, true) {
			continue
		}

		subps, err := w.readIgnoreFiles(filepath.Join(root, name), testPath, baseMatcher)
		if err != nil {
			return patterns, err
		}
		patterns = append(patterns, subps...)
	}

	return patterns, nil
}

func (w *ignoreWrapper) readIgnoreFile(path string, domain []string) ([]gitignore.Pattern, error) {
	data, err := w.lower.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var patterns []gitignore.Pattern
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(trimmed, domain))
	}

	return patterns, scanner.Err()
}

func (w *ignoreWrapper) isIgnoredWithDir(path string, hint dirHint) bool {
	abs := path
	if !filepath.IsAbs(path) {
		var err error
		abs, err = filepath.Abs(path)
		if err != nil {
			return false
		}
	}

	rel, err := filepath.Rel(w.rootDir, abs)
	if err != nil || rel == "" || rel == "." {
		return false
	}
	rel = filepath.ToSlash(rel)

	isDir := false
	switch hint {
	case dirIsDir:
		isDir = true
	case dirIsFile:
		// already false
	default:
		info, statErr := w.lower.Stat(abs)
		if statErr == nil {
			isDir = info.IsDir()
		}
	}

	pathParts := strings.Split(rel, "/")

	// Check base patterns first (lowest priority).
	ignored := false
	if w.baseMatcher != nil {
		switch w.baseMatcher.MatchResult(pathParts, isDir) {
		case gitignore.Exclude:
			ignored = true
		case gitignore.Include:
			ignored = false
		}
	}

	// Check ancestor patterns second — can override base.
	if w.ancestorMatcher != nil {
		repoRel := make([]string, len(w.ancestorOffset)+len(pathParts))
		copy(repoRel, w.ancestorOffset)
		copy(repoRel[len(w.ancestorOffset):], pathParts)
		switch w.ancestorMatcher.MatchResult(repoRel, isDir) {
		case gitignore.Exclude:
			ignored = true
		case gitignore.Include:
			ignored = false
		}
	}

	// Check local patterns last (highest priority).
	if w.localMatcher != nil {
		switch w.localMatcher.MatchResult(pathParts, isDir) {
		case gitignore.Exclude:
			ignored = true
		case gitignore.Include:
			ignored = false
		}
	}

	return ignored
}

func (w *ignoreWrapper) ReadFile(path string) ([]byte, error) {
	if w.isIgnoredWithDir(path, dirIsFile) {
		return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
	}
	return w.lower.ReadFile(path)
}

func (w *ignoreWrapper) WriteFile(path string, data []byte, perm os.FileMode) error {
	return w.lower.WriteFile(path, data, perm)
}

func (w *ignoreWrapper) Stat(path string) (os.FileInfo, error) {
	info, err := w.lower.Stat(path)
	if err != nil {
		return nil, err
	}
	if w.isIgnoredWithDir(path, dirFromBool(info.IsDir())) {
		return nil, &fs.PathError{Op: "stat", Path: path, Err: fs.ErrNotExist}
	}
	return info, nil
}

func (w *ignoreWrapper) ReadDir(name string) ([]fs.DirEntry, error) {
	if w.isIgnoredWithDir(name, dirIsDir) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	entries, err := w.lower.ReadDir(name)
	if err != nil {
		return nil, err
	}

	result := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		entryAbs := filepath.Join(name, entry.Name())
		if w.isIgnoredWithDir(entryAbs, dirFromBool(entry.IsDir())) {
			continue
		}
		result = append(result, entry)
	}

	return result, nil
}

func (w *ignoreWrapper) WalkDir(root string, fn func(abs string, d fs.DirEntry) error) error {
	if w.isIgnoredWithDir(root, dirIsDir) {
		return nil
	}

	return w.lower.WalkDir(root, func(abs string, d fs.DirEntry) error {
		if abs == root {
			return nil
		}

		if w.isIgnoredWithDir(abs, dirFromBool(d.IsDir())) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		return fn(abs, d)
	})
}

type dirHint int

const (
	dirUnknown dirHint = iota
	dirIsFile
	dirIsDir
)

func dirFromBool(b bool) dirHint {
	if b {
		return dirIsDir
	}
	return dirIsFile
}
