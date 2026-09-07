// Package namespaces holds helpers shared by adapters whose capsule IDs are
// dot-separated namespaces (C#, JVM, Python).
package namespaces

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/dariushalipour/baft/internal/port"
)

// IsInternal reports whether spec is base itself or a namespace nested under it.
func IsInternal(spec, base string) bool {
	if spec == base {
		return true
	}
	if !strings.HasPrefix(spec, base+".") {
		return false
	}
	rest := spec[len(base)+1:]
	return rest != "" && !strings.HasSuffix(rest, ".")
}

// CommonPrefix returns the longest namespace shared by every directory holding
// a file with one of exts under srcDirs, as a dot-separated path. srcDirs are
// absolute: they are always derived from a capsule dir, and discovery walks
// from an absolute root.
func CommonPrefix(fsys port.FileSystem, srcDirs, exts []string) (string, error) {
	seen := make(map[string]bool)
	var dirs []string
	for _, src := range srcDirs {
		err := fsys.WalkDir(context.Background(), src, func(abs string, d fs.DirEntry) error {
			if d.IsDir() || !hasExt(abs, exts) {
				return nil
			}
			rel, err := filepath.Rel(src, abs)
			if err != nil {
				return nil
			}
			dir := filepath.ToSlash(filepath.Dir(rel))
			if dir != "." && !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("no %s files in subdirectories of %s", strings.Join(exts, "/"), strings.Join(srcDirs, ", "))
	}
	common := commonPathPrefix(dirs)
	if common == "" {
		return "", errors.New("cannot determine base capsule")
	}
	return strings.Replace(common, "/", ".", -1), nil
}

func hasExt(path string, exts []string) bool {
	for _, ext := range exts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func commonPathPrefix(paths []string) string {
	common := strings.Split(paths[0], "/")
	for _, p := range paths[1:] {
		parts := strings.Split(p, "/")
		if len(parts) < len(common) {
			common = common[:len(parts)]
		}
		for i := range common {
			if parts[i] != common[i] {
				common = common[:i]
				break
			}
		}
	}
	return strings.Join(common, "/")
}
