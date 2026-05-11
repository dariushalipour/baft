package rust

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "rust" }

func (Language) IsGovernedFile(rel string) bool {
	if !strings.HasSuffix(rel, ".rs") {
		return false
	}
	if !strings.HasPrefix(rel, "src/") {
		return false
	}
	if strings.HasPrefix(rel, "src/bin/") {
		return false
	}
	if strings.HasPrefix(rel, "src/examples/") {
		return false
	}
	return true
}

var useRe = regexp.MustCompile(`(?m)^(\s*)(?:pub\(.*?\)\s+|pub\s+)?use\s+([^;]+);\s*$`)
var modRe = regexp.MustCompile(`(?m)^(\s*)(?:pub\(.*?\)\s+|pub\s+)?mod\s+(\w+)\s*;\s*$`)
var externCrateRe = regexp.MustCompile(`(?m)^(\s*)extern\s+crate\s+(\w+)(?:\s+as\s+(\w+))?\s*;\s*$`)
var cargoNameRe = regexp.MustCompile(`^name\s*=\s*"([^"]+)"`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	// Estimate capacity from approximate line count.
	approxLines := bytes.Count(data, []byte{'\n'}) + 1
	imports := make([]port.ImportSpec, 0, approxLines/3)

	lineNum := 1
	lineStart := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			lineStr := string(data[lineStart:i])
			if m := useRe.FindStringSubmatchIndex(lineStr); m != nil {
				spec := strings.TrimSpace(lineStr[m[4]:m[5]])
				if !seen[spec] {
					seen[spec] = true
					col := m[4] + 1
					imports = append(imports, port.ImportSpec{Path: spec, Line: lineNum, Col: col, ColEnd: col + len(spec)})
				}
			} else if m := modRe.FindStringSubmatchIndex(lineStr); m != nil {
				modName := lineStr[m[4]:m[5]]
				if !seen[modName] {
					seen[modName] = true
					col := m[4] + 1
					imports = append(imports, port.ImportSpec{Path: modName, Line: lineNum, Col: col, ColEnd: col + len(modName)})
				}
			} else if m := externCrateRe.FindStringSubmatchIndex(lineStr); m != nil {
				crateName := lineStr[m[4]:m[5]]
				if !seen[crateName] {
					seen[crateName] = true
					col := m[4] + 1
					imports = append(imports, port.ImportSpec{Path: crateName, Line: lineNum, Col: col, ColEnd: col + len(crateName)})
				}
			}
			lineNum++
			lineStart = i + 1
		}
	}
	// Handle last line without newline.
	if lineStart < len(data) {
		lineStr := string(data[lineStart:])
		if m := useRe.FindStringSubmatchIndex(lineStr); m != nil {
			spec := strings.TrimSpace(lineStr[m[4]:m[5]])
			if !seen[spec] {
				seen[spec] = true
				col := m[4] + 1
				imports = append(imports, port.ImportSpec{Path: spec, Line: lineNum, Col: col, ColEnd: col + len(spec)})
			}
		} else if m := modRe.FindStringSubmatchIndex(lineStr); m != nil {
			modName := lineStr[m[4]:m[5]]
			if !seen[modName] {
				seen[modName] = true
				col := m[4] + 1
				imports = append(imports, port.ImportSpec{Path: modName, Line: lineNum, Col: col, ColEnd: col + len(modName)})
			}
		} else if m := externCrateRe.FindStringSubmatchIndex(lineStr); m != nil {
			crateName := lineStr[m[4]:m[5]]
			if !seen[crateName] {
				seen[crateName] = true
				col := m[4] + 1
				imports = append(imports, port.ImportSpec{Path: crateName, Line: lineNum, Col: col, ColEnd: col + len(crateName)})
			}
		}
	}

	return imports, nil
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

func RegisterDiscovery(d *service.CapsuleDiscovery) {
	d.Register("rust", service.ManifestInfo{
		Names:     []string{"Cargo.toml"},
		ParseFunc: readCargoToml,
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
		}
	}
	return "", fmt.Errorf("no package name in %s", path)
}
