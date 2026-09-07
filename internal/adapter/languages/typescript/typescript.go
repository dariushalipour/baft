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

// combinedImportRe anchors on the module specifier's keyword rather than on the
// import line, so specifiers that sit on a later line than their `import` are
// still found; ParseImports discards matches whose quote does not open a real
// string literal, so text inside another literal is not mistaken for one.
var combinedImportRe = regexp.MustCompile(`(?m)(?:\bfrom[ \t]*|^[ \t]*(?:import|export)[ \t]*|\bimport[ \t]*\([ \t]*|\brequire[ \t]*\([ \t]*)('([^'\n]+)'|"([^"\n]+)")`)

func (l *Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	raw, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	data, literals := maskComments(raw)
	lineOffsets := lineoffsets.MakeLineOffsets(data)
	dataStr := string(data)
	seen := make(map[string]bool, 16)
	out := make([]port.ImportSpec, 0, 16)
	for _, m := range combinedImportRe.FindAllStringSubmatchIndex(dataStr, -1) {
		quote, pathEnd := m[2], m[3]
		if quote < 0 {
			quote, pathEnd = m[4], m[5]
		}
		// Only a quote that opens a real literal starts a module specifier; a
		// quote nested inside another literal is just text.
		if !literals[quote] {
			continue
		}
		pathStart := quote + 1
		pathEnd--
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

// maskComments blanks out comment bytes (keeping newlines and every other
// offset intact) so commented-out imports never match. String literals are
// skipped so that a `//` or `/*` inside one is not read as a comment, and the
// offset of each literal's opening quote is reported so callers can tell a real
// specifier from a quote nested inside another literal.
func maskComments(src []byte) ([]byte, map[int]bool) {
	data := append([]byte(nil), src...)
	literals := make(map[int]bool, 16)
	for i := 0; i < len(data); {
		switch {
		case data[i] == '/' && i+1 < len(data) && data[i+1] == '/':
			for ; i < len(data) && data[i] != '\n'; i++ {
				data[i] = ' '
			}
		case data[i] == '/' && i+1 < len(data) && data[i+1] == '*':
			for ; i < len(data); i++ {
				if data[i] == '*' && i+1 < len(data) && data[i+1] == '/' {
					data[i], data[i+1] = ' ', ' '
					i += 2
					break
				}
				if data[i] != '\n' {
					data[i] = ' '
				}
			}
		case data[i] == '\'' || data[i] == '"' || data[i] == '`':
			quote := data[i]
			end := i + 1
			for ; end < len(data); end++ {
				if data[end] == '\\' {
					end++
					continue
				}
				if data[end] == quote || (quote != '`' && data[end] == '\n') {
					break
				}
			}
			if end >= len(data) && quote == '`' {
				// A backtick with no partner is not a delimiter — it is text
				// inside something else, e.g. a regex literal. Treating it as
				// one would swallow the rest of the file.
				i++
				continue
			}
			literals[i] = true
			i = end + 1
		default:
			i++
		}
	}
	return data, literals
}

// stripJSONC turns JSONC (comments, trailing commas) into plain JSON.
func stripJSONC(src []byte) []byte {
	data, _ := maskComments(src)
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			start := i
			for i++; i < len(data) && data[i] != '"'; i++ {
				if data[i] == '\\' {
					i++
				}
			}
			if i >= len(data) {
				i = len(data) - 1
			}
			out = append(out, data[start:i+1]...)
			continue
		}
		if data[i] == ',' {
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				continue
			}
		}
		out = append(out, data[i])
	}
	return out
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

	if cfg := l.resolveTsconfigCached(fsys, c.Dir); cfg != nil {
		if resolved := cfg.resolvePaths(fsys, spec.Path); resolved != "" {
			return resolveExtension(fsys, resolved, c.Dir), true
		}
	}

	resolved, ok := resolveByCapsuleName(spec.Path, c)
	if ok {
		resolved = resolveExtension(fsys, resolved, c.Dir)
	}
	return resolved, ok
}

// resolveTsconfigCached caches misses as well as hits, so a capsule without a
// readable tsconfig is not re-read once per import.
func (l *Language) resolveTsconfigCached(fsys port.FileSystem, capsuleDir string) *tsconfig {
	if cached, ok := l.tsconfigCache.Load(capsuleDir); ok {
		return cached.(*tsconfig)
	}
	cfg, err := resolveTsconfig(fsys, capsuleDir)
	if err != nil {
		cfg = nil
	}
	cached, _ := l.tsconfigCache.LoadOrStore(capsuleDir, cfg)
	return cached.(*tsconfig)
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

	// A dot in the basename is not necessarily an extension: `./user.service`
	// still resolves to user.service.ts.
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

func (l *Language) GetFileNamespace(_ port.FileSystem, _ string) (string, error) { return "", nil }
func (l *Language) SupportsFileGlobs() bool                                      { return true }
func (l *Language) Register(d port.CapsuleDiscovery) {
	d.Register("typescript", port.ManifestInfo{
		Names:     []string{"package.json"},
		ParseFunc: readCapsuleName,
		// node_modules is deliberately absent: base ignores are unioned across
		// every language, and hiding it would also hide the package tsconfigs
		// that `extends` resolves path aliases through.
		BaseIgnoreEntries: []string{"*.test.ts", "*.test.tsx", "*.spec.ts", "*.spec.tsx"},
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
	if err := json.Unmarshal(stripJSONC(data), &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	cfg.configDir = capsuleDir

	if cfg.Extends != "" {
		visited := map[string]bool{filepath.Clean(cfgPath): true}
		parent, err := resolveTsconfigExtends(fsys, cfg.Extends, capsuleDir, visited)
		if err == nil && parent != nil {
			cfg.merge(parent)
		}
	}

	return &cfg, nil
}

func resolveTsconfigExtends(fsys port.FileSystem, extends string, capsuleDir string, visited map[string]bool) (*tsconfig, error) {
	// Only ./ and ../ (and absolute) specifiers are file paths; everything else
	// names a package under node_modules.
	target := extends
	isPath := filepath.IsAbs(extends) || strings.HasPrefix(extends, "./") || strings.HasPrefix(extends, "../")
	if isPath {
		if !filepath.IsAbs(extends) {
			target = filepath.Join(capsuleDir, extends)
		}
		if !strings.HasSuffix(target, ".json") {
			target += ".json"
		}
	} else {
		target = filepath.Join(capsuleDir, "node_modules", extends)
		if !strings.HasSuffix(target, ".json") {
			target = filepath.Join(target, "tsconfig.json")
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
	if err := json.Unmarshal(stripJSONC(data), &cfg); err != nil {
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
