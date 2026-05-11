package overlayfs

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/dariushalipour/strata/internal/port"
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
}

func Decode(r io.Reader) (Payload, error) {
	var payload Payload
	err := json.NewDecoder(r).Decode(&payload)
	return payload, err
}

func New(lower port.FileSystem, files map[string][]byte) *FS {
	cloned := make(map[string][]byte, len(files))
	for path, content := range files {
		cloned[filepath.Clean(path)] = append([]byte(nil), content...)
	}
	return &FS{lower: lower, files: cloned}
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

func (f *FS) WalkDir(root string, fn func(abs string, d fs.DirEntry) error) error {
	return f.lower.WalkDir(root, fn)
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
