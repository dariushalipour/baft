package overlayfs

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	// parentIndex maps parent directory -> sorted overlay entries for O(1) ReadDir.
	parentIndex map[string][]overlayEntry
}

type overlayEntry struct {
	name string
	data []byte
}

func Decode(r io.Reader) (Payload, error) {
	var payload Payload
	err := json.NewDecoder(r).Decode(&payload)
	return payload, err
}

func New(lower port.FileSystem, files map[string][]byte) *FS {
	cloned := make(map[string][]byte, len(files))
	index := make(map[string][]overlayEntry)
	for path, content := range files {
		clean := filepath.Clean(path)
		cloned[clean] = append([]byte(nil), content...)
		parent := filepath.Dir(clean)
		index[parent] = append(index[parent], overlayEntry{name: filepath.Base(clean), data: content})
	}
	for k := range index {
		sort.Slice(index[k], func(i, j int) bool {
			return index[k][i].name < index[k][j].name
		})
	}
	return &FS{lower: lower, files: cloned, parentIndex: index}
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
	if data, ok := f.files[filepath.Clean(path)]; ok {
		if info, err := f.lower.Stat(path); err == nil {
			return info, nil
		}
		return syntheticInfo{name: filepath.Base(path), size: int64(len(data)), mode: 0o644}, nil
	}
	return f.lower.Stat(path)
}

func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	lowerEntries, err := f.lower.ReadDir(name)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		lowerEntries = nil
	}

	dir := filepath.Clean(name)
	memEntries := f.parentIndex[dir]

	lowerNames := make(map[string]bool, len(lowerEntries))
	for i := range lowerEntries {
		lowerNames[lowerEntries[i].Name()] = true
	}

	result := make([]fs.DirEntry, 0, len(lowerEntries)+len(memEntries))
	result = append(result, lowerEntries...)
	for _, me := range memEntries {
		if !lowerNames[me.name] {
			result = append(result, &syntheticEntry{name: me.name, size: len(me.data)})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})

	return result, nil
}

func (f *FS) WalkDir(root string, fn func(abs string, d fs.DirEntry) error) error {
	rootClean := filepath.Clean(root)
	rootSep := rootClean + string(filepath.Separator)

	// Collect overlay files under root, grouped by parent dir.
	overlayByDir := make(map[string][]overlayEntry)
	for p, data := range f.files {
		pClean := filepath.Clean(p)
		if pClean == rootClean || strings.HasPrefix(pClean, rootSep) {
			parent := filepath.Dir(pClean)
			overlayByDir[parent] = append(overlayByDir[parent], overlayEntry{name: filepath.Base(pClean), data: data})
		}
	}

	// Per-directory buffering: collect lower entries, merge with overlay,
	// sort, then emit. This keeps memory proportional to max directory size,
	// not total tree size (audit #1: O(N²) → O(N)).
	type dirBuf struct {
		entries []fs.DirEntry
		hasDir  bool // lower walk emitted a directory entry
	}
	bufByDir := make(map[string]*dirBuf)

	lowerEmitted := make(map[string]bool)

	err := f.lower.WalkDir(rootClean, func(abs string, d fs.DirEntry) error {
		lowerEmitted[abs] = true
		if d.IsDir() {
			bufByDir[abs] = &dirBuf{hasDir: true}
			return fn(abs, d)
		}

		parent := filepath.Dir(abs)
		buf, ok := bufByDir[parent]
		if !ok {
			buf = &dirBuf{}
			bufByDir[parent] = buf
		}
		buf.entries = append(buf.entries, d)
		return nil
	})
	if err != nil {
		return err
	}

	// If lower emitted nothing, walk overlay-only content.
	if len(lowerEmitted) == 0 {
		return f.walkOverlayOnly(rootClean, overlayByDir, fn)
	}

	// Collect all directories that had content, sorted for deterministic output.
	var dirs []string
	for dir := range bufByDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	// Emit buffered entries per directory: merge lower + overlay, sort, emit.
	for _, dir := range dirs {
		buf := bufByDir[dir]
		if buf == nil {
			continue
		}

		// Merge with overlay entries.
		overlayEntries := overlayByDir[dir]
		merged := make([]fs.DirEntry, 0, len(buf.entries)+len(overlayEntries))
		merged = append(merged, buf.entries...)
		lowerNames := make(map[string]bool, len(buf.entries))
		for _, e := range buf.entries {
			lowerNames[e.Name()] = true
		}
		for _, oe := range overlayEntries {
			if !lowerNames[oe.name] {
				merged = append(merged, &syntheticEntry{name: oe.name, size: len(oe.data)})
			}
		}

		sort.Slice(merged, func(i, j int) bool {
			return merged[i].Name() < merged[j].Name()
		})

		for _, e := range merged {
			if err := fn(filepath.Join(dir, e.Name()), e); err != nil {
				return err
			}
		}
	}

	return nil
}

func (f *FS) walkOverlayOnly(root string, overlayByDir map[string][]overlayEntry, fn func(abs string, d fs.DirEntry) error) error {
	var dirs []string
	for dir := range overlayByDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	for _, dir := range dirs {
		entries := overlayByDir[dir]
		for _, entry := range entries {
			over := &syntheticEntry{name: entry.name, size: len(entry.data)}
			if err := fn(filepath.Join(dir, entry.name), over); err != nil {
				return err
			}
		}
	}
	return nil
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
}

func (e *syntheticEntry) Name() string      { return e.name }
func (e *syntheticEntry) IsDir() bool       { return false }
func (e *syntheticEntry) Type() fs.FileMode { return 0 }
func (e *syntheticEntry) Info() (os.FileInfo, error) {
	return &syntheticInfo{name: e.name, size: int64(e.size), mode: 0o644}, nil
}
