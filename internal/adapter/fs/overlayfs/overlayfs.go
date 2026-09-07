package overlayfs

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/dariushalipour/baft/internal/adapter/fs/internal/walk"
	"github.com/dariushalipour/baft/internal/port"
)

type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type Payload struct {
	Files []File `json:"files"`
}

type FS struct {
	lower port.FileSystem
	files map[string][]byte
	// index maps a directory to its sorted overlay entries: the overlay files
	// it holds plus the directories they imply, so overlay-only directories
	// are visible to ReadDir and therefore to the walker.
	index map[string][]fs.DirEntry
	dirs  map[string]bool
}

func Decode(r io.Reader) (Payload, error) {
	var payload Payload
	err := json.NewDecoder(r).Decode(&payload)
	return payload, err
}

func New(lower port.FileSystem, files map[string][]byte) *FS {
	f := &FS{
		lower: lower,
		files: make(map[string][]byte, len(files)),
		index: make(map[string][]fs.DirEntry),
		dirs:  make(map[string]bool),
	}
	byDir := make(map[string]map[string]fs.DirEntry)
	add := func(dir string, entry fs.DirEntry) bool {
		if byDir[dir] == nil {
			byDir[dir] = make(map[string]fs.DirEntry)
		}
		if _, ok := byDir[dir][entry.Name()]; ok {
			return false
		}
		byDir[dir][entry.Name()] = entry
		return true
	}
	for path, content := range files {
		clean := filepath.Clean(path)
		f.files[clean] = append([]byte(nil), content...)
		add(filepath.Dir(clean), &syntheticEntry{name: filepath.Base(clean), size: len(content)})
		for dir := filepath.Dir(clean); ; {
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			f.dirs[dir] = true
			if !add(parent, &syntheticEntry{name: filepath.Base(dir), dir: true}) {
				break
			}
			dir = parent
		}
	}
	for dir, entries := range byDir {
		list := make([]fs.DirEntry, 0, len(entries))
		for _, e := range entries {
			list = append(list, e)
		}
		sortEntries(list)
		f.index[dir] = list
	}
	return f
}

func NewFromPayload(lower port.FileSystem, payload Payload) *FS {
	files := make(map[string][]byte, len(payload.Files))
	for _, file := range payload.Files {
		files[file.Path] = []byte(file.Content)
	}
	return New(lower, files)
}

func (f *FS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[filepath.Clean(path)]; ok {
		return append([]byte(nil), data...), nil
	}
	return f.lower.ReadFile(path)
}

func (f *FS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return f.lower.WriteFile(path, data, perm)
}

func (f *FS) Stat(path string) (os.FileInfo, error) {
	info, err := f.lower.Stat(path)
	if err == nil {
		return info, nil
	}
	clean := filepath.Clean(path)
	if data, ok := f.files[clean]; ok {
		return syntheticInfo{name: filepath.Base(clean), size: int64(len(data)), mode: 0o644}, nil
	}
	if f.dirs[clean] {
		return syntheticInfo{name: filepath.Base(clean), mode: 0o755 | os.ModeDir}, nil
	}
	return nil, err
}

func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	lowerEntries, err := f.lower.ReadDir(name)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		lowerEntries = nil
	}

	overlay := f.index[filepath.Clean(name)]
	if len(overlay) == 0 {
		return lowerEntries, nil
	}

	lowerNames := make(map[string]bool, len(lowerEntries))
	for _, e := range lowerEntries {
		lowerNames[e.Name()] = true
	}

	result := make([]fs.DirEntry, 0, len(lowerEntries)+len(overlay))
	result = append(result, lowerEntries...)
	for _, e := range overlay {
		if !lowerNames[e.Name()] {
			result = append(result, e)
		}
	}
	sortEntries(result)

	return result, nil
}

func (f *FS) WalkDir(ctx context.Context, root string, fn func(abs string, d fs.DirEntry) error) error {
	return walk.Dir(ctx, f, root, fn)
}

func sortEntries(entries []fs.DirEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
}

type syntheticInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (s syntheticInfo) Name() string       { return s.name }
func (s syntheticInfo) Size() int64        { return s.size }
func (s syntheticInfo) Mode() os.FileMode  { return s.mode }
func (s syntheticInfo) ModTime() time.Time { return time.Time{} }
func (s syntheticInfo) IsDir() bool        { return s.mode.IsDir() }
func (s syntheticInfo) Sys() any           { return nil }

type syntheticEntry struct {
	name string
	size int
	dir  bool
}

func (e *syntheticEntry) Name() string { return e.name }
func (e *syntheticEntry) IsDir() bool  { return e.dir }
func (e *syntheticEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}
func (e *syntheticEntry) Info() (os.FileInfo, error) {
	if e.dir {
		return &syntheticInfo{name: e.name, mode: 0o755 | fs.ModeDir}, nil
	}
	return &syntheticInfo{name: e.name, size: int64(e.size), mode: 0o644}, nil
}
