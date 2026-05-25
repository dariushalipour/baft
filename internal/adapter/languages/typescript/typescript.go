package typescript

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/dariushalipour/baft/internal/adapter/languages/internal/lineoffsets"
	"github.com/dariushalipour/baft/internal/port"
)

type Language struct {
	tsconfigCache sync.Map
}

func (l *Language) Name() string { return "typescript" }

func (l *Language) IsScannableFile(rel string) bool {
	n := len(rel)
	if n < 3 {
		return false
	}
	if rel[n-3:] == ".ts" {
		return true
	}
	if n >= 4 && rel[n-4:] == ".tsx" {
		return true
	}
	return false
}

var combinedImportRe = regexp.MustCompile(`(?m)(?:^\s*(?:import|export)\s+.*?|^\s*import\s+\w+\s*=\s*require\s*\(|\bimport\s*\(|\brequire\s*\()('([^']+)'|\"([^\"]+)\")`)

func (l *Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	lineOffsets := lineoffsets.MakeLineOffsets(data)
	dataStr := string(data)
	seen := make(map[string]bool, 16)
	out := make([]port.ImportSpec, 0, 16)
	for _, m := range combinedImportRe.FindAllStringSubmatchIndex(dataStr, -1) {
		var pathStart, pathEnd int
		if m[2] >= 0 {
			pathStart = m[2] + 1
			pathEnd = m[3] - 1
		} else {
			pathStart = m[4] + 1
			pathEnd = m[5] - 1
		}
		line, col := lineoffsets.OffsetToLineCol(lineOffsets, data, pathStart)
		_, colEnd := lineoffsets.OffsetToLineCol(lineOffsets, data, pathEnd)
		spec := dataStr[pathStart:pathEnd]
		if !seen[spec] {
			seen[spec] = true
			out = append(out, port.ImportSpec{Path: spec, Line: line, Col: col, ColEnd: colEnd})
		}
	}
	return out, nil
}

func (l *Language) ResolveInternalTarget(fsys port.FileSystem, spec port.ImportSpec, c port.Capsule, fileRel string) (string, bool) {
	if strings.HasPrefix(spec.Path, ".") {
		base := path.Dir(fileRel)
		full := path.Clean(path.Join(base, spec.Path))
		if strings.HasPrefix(full, "../") || full == ".." {
			return "", false
		}
		return resolveExtension(fsys, full, c.Dir), true
	}

	tsconfig, err := l.resolveTsconfigCached(fsys, c.Dir)
	if err != nil || tsconfig == nil {
		resolved, ok := resolveByCapsuleName(spec.Path, c)
		if ok {
			resolved = resolveExtension(fsys, resolved, c.Dir)
		}
		return resolved, ok
	}

	if resolved := tsconfig.resolvePaths(fsys, spec.Path); resolved != "" {
		return resolveExtension(fsys, resolved, c.Dir), true
	}

	resolved, ok := resolveByCapsuleName(spec.Path, c)
	if ok {
		resolved = resolveExtension(fsys, resolved, c.Dir)
	}
	return resolved, ok
}

func (l *Language) resolveTsconfigCached(fsys port.FileSystem, capsuleDir string) (*tsconfig, error) {
	if cached, ok := l.tsconfigCache.Load(capsuleDir); ok {
		return cached.(*tsconfig), nil
	}
	cfg, err := resolveTsconfig(fsys, capsuleDir)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		if loaded, ok := l.tsconfigCache.LoadOrStore(capsuleDir, cfg); ok {
			return loaded.(*tsconfig), nil
		}
	}
	return cfg, nil
}

func resolveByCapsuleName(spec string, c port.Capsule) (string, bool) {
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
	hasDot := false
	for i := 0; i < len(base); i++ {
		if base[i] == '.' {
			hasDot = true
			break
		}
	}

	if strings.HasSuffix(resolved, ".ts") || strings.HasSuffix(resolved, ".tsx") {
		return resolved
	}

	jsToTs := [][2]string{{".js", ".ts"}, {".jsx", ".tsx"}, {".mjs", ".mts"}, {".cjs", ".cts"}}

	var jsExt, tsExt string
	for _, pair := range jsToTs {
		if strings.HasSuffix(resolved, pair[0]) {
			jsExt = pair[0]
			tsExt = pair[1]
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

	for _, ext := range [4]string{".ts", ".tsx", ".js", ".jsx"} {
		candidate := resolved + ext
		if _, err := fsys.Stat(filepath.Join(capsuleDir, filepath.FromSlash(candidate))); err == nil {
			return candidate
		}
	}

	dirAbs := filepath.Join(capsuleDir, filepath.FromSlash(resolved))
	if _, err := fsys.Stat(dirAbs); err == nil {
		for _, ext := range [4]string{"index.ts", "index.tsx", "index.js", "index.jsx"} {
			if _, err := fsys.Stat(filepath.Join(dirAbs, ext)); err == nil {
				return path.Join(resolved, ext)
			}
		}
	}

	return resolved
}

func (l *Language) SupportsFileGlobs() bool { return true }
func (l *Language) Register(d port.CapsuleDiscovery) {
	d.Register("typescript", port.ManifestInfo{
		Names:             []string{"package.json"},
		ParseFunc:         readCapsuleName,
		BaseIgnoreEntries: []string{"node_modules", "*.d.ts", "*.d.tsx", "*.test.ts", "*.test.tsx", "*.spec.ts", "*.spec.tsx"},
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
		visited := map[string]bool{filepath.Clean(capsuleDir): true}
		parent, err := resolveTsconfigExtends(fsys, cfg.Extends, capsuleDir, visited)
		if err == nil && parent != nil {
			cfg.merge(parent)
		}
	}

	return &cfg, nil
}

func resolveTsconfigExtends(fsys port.FileSystem, extends string, capsuleDir string, visited map[string]bool) (*tsconfig, error) {
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

	targetClean := filepath.Clean(target)
	if visited[targetClean] {
		return nil, fmt.Errorf("circular extends detected: %s", targetClean)
	}
	visited[targetClean] = true

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
		parent, err := resolveTsconfigExtends(fsys, cfg.Extends, filepath.Dir(target), visited)
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
