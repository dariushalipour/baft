// Package dryrunfs buffers writes in memory so a command can report what it
// would change without touching the working tree.
package dryrunfs

import (
	"os"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/port"
)

// Wrap returns fsys with every write kept in memory. Reads and stats see the
// buffered writes first, so a later pass still observes what it just wrote.
func Wrap(fsys port.FileSystem) port.FileSystem {
	return &dryRunFS{FileSystem: fsys, mem: memfs.New()}
}

type dryRunFS struct {
	port.FileSystem
	mem *memfs.FS
}

func (f *dryRunFS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return f.mem.WriteFile(path, data, perm)
}

func (f *dryRunFS) ReadFile(path string) ([]byte, error) {
	if data, err := f.mem.ReadFile(path); err == nil {
		return data, nil
	}
	return f.FileSystem.ReadFile(path)
}

func (f *dryRunFS) Stat(path string) (os.FileInfo, error) {
	if info, err := f.mem.Stat(path); err == nil {
		return info, nil
	}
	return f.FileSystem.Stat(path)
}
