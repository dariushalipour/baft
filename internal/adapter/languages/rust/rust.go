package rust

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "rust" }

func (Language) IsScannableFile(rel string) bool {
	return strings.HasSuffix(rel, ".rs")
}

var cargoNameRe = regexp.MustCompile(`^name\s*=\s*"([^"]+)"`)
var cargoNameInlineRe = regexp.MustCompile(`^name\s*=\s*\{[^"]*value\s*=\s*"([^"]+)"`)

// rustImportRe matches import, mod, or extern crate statements with capture groups.
// Group 1: use spec, Group 2: mod name, Group 3: extern crate name
var rustImportRe = regexp.MustCompile(`(?m)^\s*(?:pub\(.*?\)\s+|pub\s+)?(?:use\s+([^;]+);|mod\s+(\w+);|extern\s+crate\s+(\w+)(?:\s+as\s+\w+)?)`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	// Precompute line offsets for O(1) line/col lookup.
	lineOffsets := makeLineOffsets(data)
	dataStr := string(data)

	seen := make(map[string]bool)
	// Estimate capacity from approximate line count.
	approxLines := bytes.Count(data, []byte{'\n'}) + 1
	imports := make([]port.ImportSpec, 0, approxLines/3)

	// Single-pass matching: find all import/mod/extern crate statements at once.
	for _, m := range rustImportRe.FindAllSubmatchIndex(data, -1) {
		// Determine which group matched and extract the import path.
		var spec string
		if m[2] != -1 {
			// use statement - group 2 is the use spec
			spec = strings.TrimSpace(dataStr[m[2]:m[3]])
		} else if m[4] != -1 {
			// mod statement - group 4 is the mod name
			spec = dataStr[m[4]:m[5]]
		} else if m[6] != -1 {
			// extern crate - group 6 is the crate name
			spec = dataStr[m[6]:m[7]]
		} else {
			continue
		}

		if !seen[spec] {
			seen[spec] = true
			// Find the start offset of the matched text for line/col calculation.
			matchStart := m[0]
			line, col := offsetToLineCol(lineOffsets, data, matchStart)
			imports = append(imports, port.ImportSpec{Path: spec, Line: line, Col: col, ColEnd: col + len(spec)})
		}
	}

	return imports, nil
}

// makeLineOffsets precomputes the byte offset of each line start for O(1) line/col lookup.
func makeLineOffsets(data []byte) []int {
	offsets := make([]int, 0, bytes.Count(data, []byte{'\n'})+1)
	offsets = append(offsets, 0)
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

// offsetToLineCol converts a byte offset to line/col using precomputed line offsets.
func offsetToLineCol(lineOffsets []int, data []byte, offset int) (int, int) {
	if offset > len(data) {
		offset = len(data)
	}
	if offset < 0 {
		offset = 0
	}
	idx := sort.Search(len(lineOffsets), func(i int) bool {
		return lineOffsets[i] > offset
	})
	line := idx - 1
	if line < 0 {
		line = 0
	}
	return line + 1, offset - lineOffsets[line] + 1
}

func (Language) ResolveInternalTarget(fsys port.FileSystem, spec port.ImportSpec, c port.Capsule, fileRel string) (string, bool) {
	s := spec.Path
	if idx := strings.LastIndex(s, " as "); idx != -1 {
		s = strings.TrimSpace(s[:idx])
	}

	switch {
	case strings.HasPrefix(s, "crate::"):
		relPath := strings.TrimPrefix(s, "crate::")
		parts := strings.Split(relPath, "::")
		if len(parts) > 1 {
			dirParts := parts[:len(parts)-1]
			return "src/" + strings.Join(dirParts, "/"), true
		}
		return "src", true

	case strings.HasPrefix(s, "super::"):
		fileDir := filepath.Dir(fileRel)
		remaining := s
		superCount := 0
		for strings.HasPrefix(remaining, "super::") {
			superCount++
			remaining = strings.TrimPrefix(remaining, "super::")
		}
		for i := 1; i < superCount; i++ {
			parent := filepath.Dir(fileDir)
			if parent == "." || parent == fileDir {
				break
			}
			if parent == "src" {
				fileDir = "src"
				break
			}
			fileDir = parent
		}
		if fileDir == "." {
			fileDir = "src"
		}
		if remaining != "" {
			parts := strings.Split(remaining, "::")
			if len(parts) > 1 {
				dirParts := parts[:len(parts)-1]
				resolved := filepath.Clean(filepath.Join(append([]string{fileDir}, dirParts...)...))
				return filepath.ToSlash(resolved), true
			}
			return filepath.ToSlash(fileDir), true
		}
		return filepath.ToSlash(fileDir), true

	case strings.HasPrefix(s, "self::"):
		fileDir := filepath.Dir(fileRel)
		relPath := strings.TrimPrefix(s, "self::")
		parts := strings.Split(relPath, "::")
		if len(parts) > 1 {
			dirParts := parts[:len(parts)-1]
			resolved := filepath.Clean(filepath.Join(append([]string{fileDir}, dirParts...)...))
			return filepath.ToSlash(resolved), true
		}
		return filepath.ToSlash(fileDir), true

	default:
		if strings.Contains(s, "::") {
			parts := strings.SplitN(s, "::", 2)
			crateName := parts[0]
			if crateName == c.CapsuleID {
				subParts := strings.Split(parts[1], "::")
				if len(subParts) > 1 {
					dirParts := subParts[:len(subParts)-1]
					return "src/" + strings.Join(dirParts, "/"), true
				}
				return "src", true
			}
			crateDir := filepath.Join(c.Dir, crateName)
			if _, err := fsys.Stat(filepath.Join(crateDir, "Cargo.toml")); err == nil {
				return "", false
			}
			return "", false
		}
		return filepath.ToSlash(filepath.Join(filepath.Dir(fileRel), s)), true
	}
}

func (Language) SupportsFileGlobs() bool { return false }
func (Language) Register(d port.CapsuleDiscovery) {
	d.Register("rust", port.ManifestInfo{
		Names:             []string{"Cargo.toml"},
		ParseFunc:         readCargoToml,
		BaseIgnoreEntries: []string{"target"},
	})
}

func readCargoToml(fsys port.FileSystem, path string) (string, error) {
	data, err := fsys.ReadFile(path)
	if err != nil {
		return "", err
	}

	inPackage := false
	lineStart := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			line := strings.TrimSpace(string(data[lineStart:i]))
			if line == "[package]" {
				inPackage = true
				lineStart = i + 1
				continue
			}
			if len(line) > 0 && line[0] == '[' {
				if line != "[package]" && !strings.HasPrefix(line, "[package") {
					inPackage = false
				}
			}
			if inPackage {
				if m := cargoNameRe.FindStringSubmatch(line); m != nil {
					return m[1], nil
				}
				if m := cargoNameInlineRe.FindStringSubmatch(line); m != nil {
					return m[1], nil
				}
			}
			lineStart = i + 1
		}
	}
	// Handle last line without newline.
	if lineStart < len(data) {
		line := strings.TrimSpace(string(data[lineStart:]))
		if line == "[package]" {
			inPackage = true
		} else if len(line) > 0 && line[0] == '[' {
			inPackage = false
		}
		if inPackage {
			if m := cargoNameRe.FindStringSubmatch(line); m != nil {
				return m[1], nil
			}
			if m := cargoNameInlineRe.FindStringSubmatch(line); m != nil {
				return m[1], nil
			}
		}
	}
	return "", fmt.Errorf("no package name in %s", path)
}
