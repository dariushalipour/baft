package memfs

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FS is an in-memory FileSystem for testing.
type FS struct {
	mu         sync.RWMutex
	files      map[string]*file
	dirs       map[string]bool
	readErrors map[string]error
	statErrors map[string]error
	walkErrors map[string]error
}

type file struct {
	data    []byte
	mode    os.FileMode
	modTime time.Time
}

// New creates a new in-memory file system.
func New() *FS {
	return &FS{
		files:      make(map[string]*file),
		dirs:       make(map[string]bool),
		readErrors: make(map[string]error),
		statErrors: make(map[string]error),
		walkErrors: make(map[string]error),
	}
}

func (f *FS) SetReadError(path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readErrors[path] = err
}

func (f *FS) SetStatError(path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statErrors[path] = err
}

func (f *FS) SetWalkError(path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.walkErrors[path] = err
}

func (f *FS) ReadFile(filepathArg string) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if err, ok := f.readErrors[filepathArg]; ok {
		return nil, err
	}
	fp := path.Clean("/" + filepathArg)
	if fi, ok := f.files[fp]; ok {
		return fi.data, nil
	}
	return nil, &fs.PathError{Op: "read", Path: filepathArg, Err: fs.ErrNotExist}
}

func (f *FS) WriteFile(filepathArg string, data []byte, perm os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fp := path.Clean("/" + filepathArg)
	f.files[fp] = &file{
		data:    append([]byte(nil), data...),
		mode:    perm,
		modTime: time.Now(),
	}
	// Ensure all parent directories are tracked.
	for p := path.Dir(fp); p != "/" && p != "."; p = path.Dir(p) {
		f.dirs[p] = true
	}
	return nil
}

// Mkdir creates a directory (and all parent directories) in the in-memory FS.
func (f *FS) Mkdir(pathArg string, perm os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	dir := path.Clean("/" + pathArg)
	for dir != "/" && dir != "." {
		f.dirs[dir] = true
		dir = path.Dir(dir)
	}
	return nil
}

// MkdirAll is an alias for Mkdir that mirrors the standard library signature.
func (f *FS) MkdirAll(pathArg string, perm os.FileMode) error {
	return f.Mkdir(pathArg, perm)
}

func (f *FS) Stat(filepathArg string) (os.FileInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if err, ok := f.statErrors[filepathArg]; ok {
		return nil, err
	}
	fp := path.Clean("/" + filepathArg)
	if fi, ok := f.files[fp]; ok {
		return &stat{
			name: path.Base(fp),
			mode: fi.mode,
		}, nil
	}

	// O(1) directory check via tracked dirs map.
	if f.dirs[fp] {
		return &stat{
			name: path.Base(fp),
			mode: 0o755 | fs.ModeDir,
		}, nil
	}

	// Fallback: scan files to detect implicit directories.
	for p := range f.files {
		if strings.HasPrefix(p, fp+"/") {
			return &stat{
				name: path.Base(fp),
				mode: 0o755 | fs.ModeDir,
			}, nil
		}
	}

	return nil, &fs.PathError{Op: "stat", Path: filepathArg, Err: fs.ErrNotExist}
}

func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	dir := path.Clean("/" + filepath.Clean(name))
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir
	}

	// Single O(n) pass: collect direct file names and immediate subdirectory names.
	fileEntries := make(map[string]bool) // direct file names
	dirEntries := make(map[string]bool)  // immediate subdirectory names

	for p := range f.files {
		prefix := dir
		if prefix != "/" {
			prefix = dir + "/"
		}
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := strings.TrimPrefix(p, prefix)
		if rest == "" || strings.HasPrefix(rest, "/") {
			continue
		}
		firstSlash := strings.Index(rest, "/")
		top := rest
		if firstSlash >= 0 {
			top = rest[:firstSlash]
		}
		if firstSlash >= 0 {
			dirEntries[top] = true
		} else {
			fileEntries[top] = true
		}
	}

	// Include explicitly tracked empty directories.
	for d := range f.dirs {
		if d == dir {
			continue
		}
		// Check if d is a direct child of dir.
		dParent := path.Dir(d)
		if dParent == dir {
			base := path.Base(d)
			// Only add if not already present from files.
			if !dirEntries[base] && !fileEntries[base] {
				dirEntries[base] = true
			}
		}
	}

	// Build sorted entry list.
	var entries []fs.DirEntry
	for name := range fileEntries {
		entries = append(entries, &dirEntry{name: name, isDir: false})
	}
	for name := range dirEntries {
		entries = append(entries, &dirEntry{name: name, isDir: true})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	return entries, nil
}

func (f *FS) WalkDir(root string, fn func(abs string, d fs.DirEntry) error) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if err, ok := f.walkErrors[root]; ok {
		return err
	}

	root = path.Clean("/" + root)
	if !strings.HasPrefix(root, "/") {
		root = "/" + root
	}
	if root != "/" {
		root = root + "/"
	}

	// Collect all file entries under root
	var fileEntries []string
	for p := range f.files {
		if p == root || strings.HasPrefix(p, root) {
			fileEntries = append(fileEntries, p)
		}
	}
	sort.Strings(fileEntries)

	// Extract all directory paths from file entries
	dirSet := make(map[string]bool)
	for _, fe := range fileEntries {
		for {
			fe = path.Dir(fe)
			if fe == "/" || !strings.HasPrefix(fe, root) {
				break
			}
			if fe != root {
				dirSet[fe] = true
			}
		}
	}

	// Merge explicitly tracked directories (empty dirs).
	for d := range f.dirs {
		if d == root || strings.HasPrefix(d, root) {
			dirSet[d] = true
		}
	}

	// Combine files and directories, sorted
	var allEntries []string
	for de := range dirSet {
		allEntries = append(allEntries, de)
	}
	allEntries = append(allEntries, fileEntries...)
	sort.Strings(allEntries)

	visited := make(map[string]bool)
	var skipped []string // directories whose children must be skipped

	for _, entry := range allEntries {
		rel := strings.TrimPrefix(entry, root)
		abs := root + rel

		if rel == "" {
			continue
		}

		// Skip entries under a previously skipped directory
		underSkipped := false
		for _, skip := range skipped {
			if strings.HasPrefix(abs, skip+"/") {
				underSkipped = true
				break
			}
		}
		if underSkipped {
			continue
		}

		_, isDir := dirSet[entry]

		if isDir {
			if visited[abs] {
				continue
			}
			visited[abs] = true

			err := fn(abs, &dirEntry{name: filepath.Base(abs), isDir: true})
			if err == fs.SkipDir {
				skipped = append(skipped, abs)
				continue
			}
			if err != nil {
				return err
			}
		} else {
			err := fn(abs, &dirEntry{name: filepath.Base(abs), isDir: false})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type stat struct {
	name string
	mode os.FileMode
}

func (s *stat) Name() string       { return s.name }
func (s *stat) Size() int64        { return 0 }
func (s *stat) Mode() os.FileMode  { return s.mode }
func (s *stat) ModTime() time.Time { return time.Time{} }
func (s *stat) IsDir() bool        { return s.mode.IsDir() }
func (s *stat) Sys() any           { return nil }

type dirEntry struct {
	name  string
	isDir bool
}

func (d *dirEntry) Name() string { return d.name }
func (d *dirEntry) IsDir() bool  { return d.isDir }
func (d *dirEntry) Type() fs.FileMode {
	if d.isDir {
		return fs.ModeDir
	}
	return 0
}
func (d *dirEntry) Info() (os.FileInfo, error) {
	return &stat{name: d.name, mode: d.Type()}, nil
}
