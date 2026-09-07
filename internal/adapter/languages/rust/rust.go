package rust

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/languages/internal/lineoffsets"
	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "rust" }

func (Language) IsScannableFile(rel string) bool {
	return strings.HasSuffix(rel, ".rs")
}

var cargoNameRe = regexp.MustCompile(`^name\s*=\s*"([^"]+)"`)
var cargoNameInlineRe = regexp.MustCompile(`^name\s*=\s*\{[^"]*value\s*=\s*"([^"]+)"`)

var rustImportRe = regexp.MustCompile(`(?m)^\s*(?:pub\(.*?\)\s+|pub\s+)?(?:use\s+([^;]+);|mod\s+(\w+);|extern\s+crate\s+(\w+)(?:\s+as\s+\w+)?)`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	lineOffsets := lineoffsets.MakeLineOffsets(data)
	dataStr := string(data)

	seen := make(map[string]bool)
	approxLines := bytes.Count(data, []byte{'\n'}) + 1
	imports := make([]port.ImportSpec, 0, approxLines/3)

	for _, m := range rustImportRe.FindAllSubmatchIndex(data, -1) {
		var raw string
		var specs []string
		switch {
		case m[2] != -1:
			raw = strings.TrimSpace(dataStr[m[2]:m[3]])
			specs = expandUseGroups(raw)
		case m[4] != -1: // mod statement
			raw = dataStr[m[4]:m[5]]
			specs = []string{raw}
		case m[6] != -1: // extern crate
			raw = dataStr[m[6]:m[7]]
			specs = []string{raw}
		default:
			continue
		}

		line, col := lineoffsets.OffsetToLineCol(lineOffsets, data, m[0])
		for _, spec := range specs {
			if seen[spec] {
				continue
			}
			seen[spec] = true
			imports = append(imports, port.ImportSpec{Path: spec, Line: line, Col: col, ColEnd: col + len(raw)})
		}
	}

	return imports, nil
}

// expandUseGroups flattens a use path into one path per imported item, so
// `crate::{a::X, b::Y}` becomes `crate::a::X` and `crate::b::Y`.
func expandUseGroups(s string) []string {
	open := strings.IndexByte(s, '{')
	if open == -1 {
		return []string{strings.TrimSpace(s)}
	}
	end := matchingBrace(s, open)
	if end == -1 {
		return []string{strings.TrimSpace(s)}
	}
	prefix := strings.TrimSpace(s[:open])
	suffix := s[end+1:]
	var out []string
	for _, member := range splitTopLevel(s[open+1 : end]) {
		if member == "" {
			continue
		}
		if member == "self" {
			out = append(out, expandUseGroups(strings.TrimSuffix(prefix, "::")+suffix)...)
			continue
		}
		out = append(out, expandUseGroups(prefix+member+suffix)...)
	}
	return out
}

func matchingBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
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

func (Language) GetFileNamespace(_ port.FileSystem, _ string) (string, error) { return "", nil }
func (Language) SupportsFileGlobs() bool                                      { return false }
func (Language) Register(d port.CapsuleDiscovery) {
	d.Register("rust", port.ManifestInfo{
		Names:             []string{"Cargo.toml"},
		ParseFunc:         readCargoToml,
		BaseIgnoreEntries: []string{},
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
