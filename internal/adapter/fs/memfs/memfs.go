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

	"github.com/dariushalipour/baft/internal/port"
)

// FS is an in-memory FileSystem for testing.
type FS struct {
	mu         sync.RWMutex
	files      map[string]*file
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
	return nil
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

	for p := range f.files {
		if p == fp || strings.HasPrefix(p, fp+"/") {
			return &stat{
				name: path.Base(fp),
				mode: 0755,
			}, nil
		}
	}

	return nil, &fs.PathError{Op: "stat", Path: filepathArg, Err: fs.ErrNotExist}
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

			if port.ShouldSkipDir(filepath.Base(abs)) {
				continue
			}

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
func (s *stat) IsDir() bool        { return false }
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
