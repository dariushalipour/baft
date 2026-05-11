package typescript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "typescript" }

func (Language) IsGovernedFile(rel string) bool {
	if !strings.HasSuffix(rel, ".ts") && !strings.HasSuffix(rel, ".tsx") {
		return false
	}
	if strings.HasSuffix(rel, ".d.ts") || strings.HasSuffix(rel, ".d.tsx") {
		return false
	}
	base := path.Base(rel)
	if strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".test.tsx") {
		return false
	}
	if strings.HasSuffix(base, ".spec.ts") || strings.HasSuffix(base, ".spec.tsx") {
		return false
	}
	return true
}

var staticImportRe = regexp.MustCompile(`(?m)^\s*(?:import|export)\s+.*?['"]([^'"]+)['"]`)
var dynamicImportRe = regexp.MustCompile(`\bimport\s*\(\s*['"]([^'"]+)['"]\s*\)`)
var requireRe = regexp.MustCompile(`\brequire\s*\(\s*['"]([^'"]+)['"]\s*\)`)
var importRequireRe = regexp.MustCompile(`^\s*import\s+\w+\s*=\s*require\s*\(\s*['"]([^'"]+)['"]\s*\)`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	lineOffsets := makeLineOffsets(data)
	dataStr := string(data)
	seen := make(map[string]bool)
	out := make([]port.ImportSpec, 0, 16)
	for _, m := range staticImportRe.FindAllStringSubmatchIndex(dataStr, -1) {
		line, col := offsetToLineCol(lineOffsets, data, m[2])
		_, colEnd := offsetToLineCol(lineOffsets, data, m[3])
		spec := dataStr[m[2]:m[3]]
		if !seen[spec] {
			seen[spec] = true
			out = append(out, port.ImportSpec{Path: spec, Line: line, Col: col, ColEnd: colEnd})
		}
	}
	for _, m := range dynamicImportRe.FindAllStringSubmatchIndex(dataStr, -1) {
		line, col := offsetToLineCol(lineOffsets, data, m[2])
		_, colEnd := offsetToLineCol(lineOffsets, data, m[3])
		spec := dataStr[m[2]:m[3]]
		if !seen[spec] {
			seen[spec] = true
			out = append(out, port.ImportSpec{Path: spec, Line: line, Col: col, ColEnd: colEnd})
		}
	}
	for _, m := range requireRe.FindAllStringSubmatchIndex(dataStr, -1) {
		line, col := offsetToLineCol(lineOffsets, data, m[2])
		_, colEnd := offsetToLineCol(lineOffsets, data, m[3])
		spec := dataStr[m[2]:m[3]]
		if !seen[spec] {
			seen[spec] = true
			out = append(out, port.ImportSpec{Path: spec, Line: line, Col: col, ColEnd: colEnd})
		}
	}
	for _, m := range importRequireRe.FindAllStringSubmatchIndex(dataStr, -1) {
		line, col := offsetToLineCol(lineOffsets, data, m[2])
		_, colEnd := offsetToLineCol(lineOffsets, data, m[3])
		spec := dataStr[m[2]:m[3]]
		if !seen[spec] {
			seen[spec] = true
			out = append(out, port.ImportSpec{Path: spec, Line: line, Col: col, ColEnd: colEnd})
		}
	}
	return out, nil
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
// Both line and col are 1-indexed to match the original byteOffsetToLineCol behavior.
func offsetToLineCol(lineOffsets []int, data []byte, offset int) (int, int) {
	if offset > len(data) {
		offset = len(data)
	}
	if offset < 0 {
		offset = 0
	}
	// Binary search for the first line offset greater than the target offset.
	idx := sort.Search(len(lineOffsets), func(i int) bool {
		return lineOffsets[i] > offset
	})
	line := idx - 1
	if line < 0 {
		line = 0
	}
	// Return 1-indexed line number.
	return line + 1, offset - lineOffsets[line] + 1
}

func (Language) ResolveInternalTarget(fsys port.FileSystem, spec port.ImportSpec, c port.Capsule, fileRel string) (string, bool) {
	if strings.HasPrefix(spec.Path, ".") {
		base := path.Dir(fileRel)
		full := path.Clean(path.Join(base, spec.Path))
		if strings.HasPrefix(full, "../") || full == ".." {
			return "", false
		}
		return resolveExtension(fsys, full, c.Dir), true
	}

	tsconfig, err := resolveTsconfig(fsys, c.Dir)
	if err != nil || tsconfig == nil {
		resolved, ok := resolveByCapsuleName(spec.Path, c, fileRel)
		if ok {
			resolved = resolveExtension(fsys, resolved, c.Dir)
		}
		return resolved, ok
	}

	if resolved := tsconfig.resolvePaths(fsys, spec.Path); resolved != "" {
		return resolveExtension(fsys, resolved, c.Dir), true
	}

	resolved, ok := resolveByCapsuleName(spec.Path, c, fileRel)
	if ok {
		resolved = resolveExtension(fsys, resolved, c.Dir)
	}
	return resolved, ok
}

func resolveByCapsuleName(spec string, c port.Capsule, fileRel string) (string, bool) {
	pkgName := c.CapsuleID
	if pkgName == "" {
		return "", false
	}
	if spec == pkgName || strings.HasPrefix(spec, pkgName+"/") {
		subPath := strings.TrimPrefix(spec, pkgName)
		subPath = strings.TrimPrefix(subPath, "/")
		if subPath == "" {
			return "", false
		}
		return path.Join("src", subPath), true
	}
	return "", false
}

func resolveExtension(fsys port.FileSystem, resolved, capsuleDir string) string {
	base := path.Base(resolved)
	hasDot := strings.Contains(base, ".")

	if strings.HasSuffix(resolved, ".ts") || strings.HasSuffix(resolved, ".tsx") {
		return resolved
	}

	jsToTs := map[string]string{
		".js":  ".ts",
		".jsx": ".tsx",
		".mjs": ".mts",
		".cjs": ".cts",
	}

	jsExt := ""
	var tsExt string
	for jsExtCandidate, tsExtCandidate := range jsToTs {
		if strings.HasSuffix(resolved, jsExtCandidate) {
			jsExt = jsExtCandidate
			tsExt = tsExtCandidate
			break
		}
	}

	if jsExt != "" {
		jsAbs := filepath.Join(capsuleDir, filepath.FromSlash(resolved))
		if _, err := fsys.Stat(jsAbs); err == nil {
			return resolved
		}

		tsResolved := strings.TrimSuffix(resolved, jsExt) + tsExt
		tsAbs := filepath.Join(capsuleDir, filepath.FromSlash(tsResolved))
		if _, err := fsys.Stat(tsAbs); err == nil {
			return tsResolved
		}

		return resolved
	}

	if hasDot {
		return resolved
	}

	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx"} {
		candidate := resolved + ext
		if _, err := fsys.Stat(filepath.Join(capsuleDir, filepath.FromSlash(candidate))); err == nil {
			return candidate
		}
	}

	dirAbs := filepath.Join(capsuleDir, filepath.FromSlash(resolved))
	if _, err := fsys.Stat(dirAbs); err == nil {
		for _, ext := range []string{"index.ts", "index.tsx", "index.js", "index.jsx"} {
			if _, err := fsys.Stat(filepath.Join(dirAbs, ext)); err == nil {
				return path.Join(resolved, ext)
			}
		}
	}

	return resolved
}

func (Language) SupportsFileGlobs() bool { return true }

func RegisterDiscovery(d *service.CapsuleDiscovery) {
	d.Register("typescript", service.ManifestInfo{
		Names:     []string{"package.json"},
		ParseFunc: readCapsuleName,
	})
}

type packageJSON struct {
	Name string `json:"name"`
}

func readCapsuleName(fsys port.FileSystem, p string) (string, error) {
	data, err := fsys.ReadFile(p)
	if err != nil {
		return "", err
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", fmt.Errorf("parse %s: %w", p, err)
	}
	if pkg.Name == "" {
		return "", nil
	}
	return pkg.Name, nil
}

type tsconfig struct {
	CompilerOptions struct {
		BaseURL string              `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
	Extends   string `json:"extends"`
	configDir string
}

func resolveTsconfig(fsys port.FileSystem, capsuleDir string) (*tsconfig, error) {
	cfgPath := filepath.Join(capsuleDir, "tsconfig.json")
	data, err := fsys.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	var cfg tsconfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	cfg.configDir = capsuleDir

	if cfg.Extends != "" {
		parent, err := resolveTsconfigExtends(fsys, cfg.Extends, capsuleDir)
		if err == nil && parent != nil {
			cfg.merge(parent)
		}
	}

	return &cfg, nil
}

func resolveTsconfigExtends(fsys port.FileSystem, extends string, capsuleDir string) (*tsconfig, error) {
	target := extends
	if !filepath.IsAbs(extends) {
		if !strings.HasPrefix(extends, "@") && !strings.Contains(extends, "/") {
			target = filepath.Join(capsuleDir, "node_modules", extends, "tsconfig.json")
		} else if strings.HasPrefix(extends, "@") {
			target = filepath.Join(capsuleDir, "node_modules", extends, "tsconfig.json")
		} else {
			parts := strings.SplitN(extends, "/", 2)
			target = filepath.Join(capsuleDir, "node_modules", parts[0], parts[1], "tsconfig.json")
		}
	}

	data, err := fsys.ReadFile(target)
	if err != nil {
		return nil, err
	}
	var cfg tsconfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", target, err)
	}
	cfg.configDir = filepath.Dir(target)

	if cfg.Extends != "" {
		parent, err := resolveTsconfigExtends(fsys, cfg.Extends, filepath.Dir(target))
		if err == nil && parent != nil {
			cfg.merge(parent)
		}
	}

	return &cfg, nil
}

func (c *tsconfig) merge(parent *tsconfig) {
	if c.CompilerOptions.BaseURL == "" && parent.CompilerOptions.BaseURL != "" {
		c.CompilerOptions.BaseURL = parent.CompilerOptions.BaseURL
	}
	if c.CompilerOptions.Paths == nil {
		c.CompilerOptions.Paths = parent.CompilerOptions.Paths
	} else if parent.CompilerOptions.Paths != nil {
		for k, v := range parent.CompilerOptions.Paths {
			if _, exists := c.CompilerOptions.Paths[k]; !exists {
				c.CompilerOptions.Paths[k] = v
			}
		}
	}
}

func (c *tsconfig) resolvePaths(fsys port.FileSystem, spec string) string {
	if c.CompilerOptions.Paths == nil {
		return ""
	}

	baseURL := c.CompilerOptions.BaseURL
	if baseURL != "" {
		baseURL = strings.TrimSuffix(baseURL, "/")
	}

	for pattern, replacements := range c.CompilerOptions.Paths {
		if matchPath(pattern, spec) {
			var candidates []string
			for _, replacement := range replacements {
				resolved := substitutePattern(pattern, replacement, spec)
				if baseURL != "" {
					resolved = path.Join(baseURL, resolved)
				}
				resolved = strings.Replace(resolved, "${configDir}", c.configDir, -1)
				candidates = append(candidates, resolved)
			}
			for _, resolved := range candidates {
				abs := filepath.Join(c.configDir, filepath.FromSlash(resolved))
				if _, err := fsys.Stat(abs); err == nil {
					return resolved
				}
			}
			return candidates[0]
		}
	}
	return ""
}

func matchPath(pattern, spec string) bool {
	wildcard := strings.Index(pattern, "*")
	if wildcard < 0 {
		return pattern == spec
	}
	prefix := pattern[:wildcard]
	suffix := pattern[wildcard+1:]
	if len(spec) < len(prefix)+len(suffix) {
		return false
	}
	return strings.HasPrefix(spec, prefix) && strings.HasSuffix(spec, suffix)
}

func substitutePattern(pattern, replacement, spec string) string {
	wildcard := strings.Index(pattern, "*")
	if wildcard < 0 {
		return replacement
	}
	prefix := pattern[:wildcard]
	suffix := pattern[wildcard+1:]
	matched := spec[len(prefix) : len(spec)-len(suffix)]
	return strings.Replace(replacement, "*", matched, -1)
}
