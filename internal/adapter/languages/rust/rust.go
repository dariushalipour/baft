package rust

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dariushalipour/strata/internal/application/service"
	"github.com/dariushalipour/strata/internal/port"
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

	lines := strings.Split(string(data), "\n")
	seen := make(map[string]bool)
	var imports []port.ImportSpec

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		if m := useRe.FindStringSubmatchIndex(line); m != nil {
			spec := strings.TrimSpace(line[m[4]:m[5]])
			if !seen[spec] {
				seen[spec] = true
				col := m[4] + 1
				imports = append(imports, port.ImportSpec{Path: spec, Line: lineNum, Col: col, ColEnd: col + len(spec)})
			}
			continue
		}
		if m := modRe.FindStringSubmatchIndex(line); m != nil {
			modName := line[m[4]:m[5]]
			if !seen[modName] {
				seen[modName] = true
				col := m[4] + 1
				imports = append(imports, port.ImportSpec{Path: modName, Line: lineNum, Col: col, ColEnd: col + len(modName)})
			}
			continue
		}
		if m := externCrateRe.FindStringSubmatchIndex(line); m != nil {
			crateName := line[m[4]:m[5]]
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
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "[package]" {
			inPackage = true
			continue
		}
		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[package") {
			inPackage = false
			continue
		}
		if inPackage {
			if m := cargoNameRe.FindStringSubmatch(line); m != nil {
				return m[1], nil
			}
		}
	}
	return "", fmt.Errorf("no package name in %s", path)
}
