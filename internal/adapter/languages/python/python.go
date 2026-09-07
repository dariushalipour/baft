package python

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/languages/internal/lineoffsets"
	"github.com/dariushalipour/baft/internal/adapter/languages/internal/namespaces"
	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "python" }

var exts = []string{".py", ".pyi"}

func (Language) IsScannableFile(rel string) bool {
	return strings.HasSuffix(rel, exts[0]) || strings.HasSuffix(rel, exts[1])
}

var importRe = regexp.MustCompile(`(?m)^\s*(?:from\s+(\.+(?:[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)?|[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)\s+import|import\s+([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*))`)
var moduleOnlyRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	filtered, lineMap := filterCodeLines(data)
	filteredData := []byte(strings.Join(filtered, "\n"))
	filteredLineOffsets := lineoffsets.MakeLineOffsets(filteredData)

	indices := importRe.FindAllSubmatchIndex(filteredData, -1)
	out := make([]port.ImportSpec, 0)

	for _, m := range indices {
		var modulePath string
		var matchStart int
		if m[2] != -1 {
			modulePath = string(filteredData[m[2]:m[3]])
			matchStart = m[2]
		} else if m[4] != -1 {
			modulePath = string(filteredData[m[4]:m[5]])
			matchStart = m[4]
		} else {
			continue
		}
		fLine, col := lineoffsets.OffsetToLineCol(filteredLineOffsets, filteredData, matchStart)
		if fLine < 1 || fLine-1 >= len(lineMap) {
			continue
		}
		origLine := lineMap[fLine-1]
		out = append(out, port.ImportSpec{Path: modulePath, Line: origLine, Col: col, ColEnd: col + len(modulePath)})
	}

	// Handle additional modules from comma-separated imports: import os, sys
	for i, line := range filtered {
		stripped := strings.TrimSpace(line)
		if !strings.HasPrefix(stripped, "import ") || strings.HasPrefix(stripped, "from ") {
			continue
		}
		rest := strings.TrimPrefix(stripped, "import ")
		rest = strings.TrimPrefix(rest, "(")
		rest = strings.TrimRight(rest, ")")
		if !strings.Contains(rest, ",") {
			continue
		}
		aliases := moduleOnlyRe.FindAllString(rest, -1)
		found := make(map[string]bool)
		for _, s := range out {
			found[s.Path] = true
		}
		fLine := i
		origLine := lineMap[fLine]
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		skipNext := false
		for _, mod := range aliases {
			if skipNext {
				skipNext = false
				continue
			}
			if mod == "as" || found[mod] {
				if mod == "as" {
					skipNext = true
				}
				continue
			}
			found[mod] = true
			colIdx := strings.Index(rest, mod)
			out = append(out, port.ImportSpec{Path: mod, Line: origLine, Col: indent + colIdx, ColEnd: indent + colIdx + len(mod)})
		}
	}

	// Handle bare relative imports: from . import a, b
	// The regex only captures "." as the module, but the actual dependencies are a and b.
	// We need to construct ".a" and ".b" as the module paths.
	for i, line := range filtered {
		stripped := strings.TrimSpace(line)
		if !isBareRelativeImport(stripped) {
			continue
		}
		names := extractBareRelativeNames(stripped)
		if len(names) == 0 {
			continue
		}
		found := make(map[string]bool)
		for _, s := range out {
			found[s.Path] = true
		}
		origLine := lineMap[i]
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		dots := extractRelativeDots(stripped)
		for _, name := range names {
			modPath := dots + name
			if found[modPath] {
				continue
			}
			found[modPath] = true
			colIdx := strings.Index(stripped, name)
			if colIdx == -1 {
				continue
			}
			out = append(out, port.ImportSpec{Path: modPath, Line: origLine, Col: indent + colIdx, ColEnd: indent + colIdx + len(modPath)})
		}
	}

	// Remove standalone dot-only paths (".", "..", etc.) from bare relative imports,
	// since we've replaced them with proper ".name" paths above.
	var cleaned []port.ImportSpec
	for _, spec := range out {
		isOnlyDots := spec.Path != ""
		for _, c := range spec.Path {
			if c != '.' {
				isOnlyDots = false
				break
			}
		}
		if !isOnlyDots {
			cleaned = append(cleaned, spec)
		}
	}

	sort.Slice(cleaned, func(i, j int) bool {
		if cleaned[i].Line != cleaned[j].Line {
			return cleaned[i].Line < cleaned[j].Line
		}
		return cleaned[i].Col < cleaned[j].Col
	})

	return cleaned, nil
}

// filterCodeLines removes comment lines, blank-only lines, and lines inside
// multiline string literals, returning the cleaned lines and a mapping from
// filtered line index -> original 1-based line number.
func filterCodeLines(data []byte) ([]string, []int) {
	lines := strings.Split(string(data), "\n")
	var rawOut []string
	var lineMap []int // lineMap[i] = original 1-based line number of rawOut[i]
	inMultiline := false
	quote := ""

	for i, line := range lines {
		origLine := i + 1

		if inMultiline {
			idx := findUnescapedTripleQuote(line, quote)
			if idx == -1 {
				rawOut = append(rawOut, "")
				lineMap = append(lineMap, origLine)
				continue
			}
			line = line[idx+3:]
			inMultiline = false
		}

		// Process triple-quoted strings
		for {
			stripped := false
			for _, q := range []string{`"""`, "'''"} {
				idx := findUnescapedTripleQuote(line, q)
				if idx == -1 {
					continue
				}
				after := line[idx+3:]
				closeIdx := findUnescapedTripleQuote(after, q)
				if closeIdx != -1 {
					line = after[closeIdx+3:]
					stripped = true
					break
				}
				inMultiline = true
				quote = q
				line = line[:idx]
				stripped = true
				break
			}
			if !stripped {
				break
			}
		}

		stripped := strings.TrimSpace(line)
		if stripped == "" {
			continue
		}
		if strings.HasPrefix(stripped, "#") {
			continue
		}

		if cIdx := strings.Index(stripped, "#"); cIdx != -1 {
			line = strings.TrimSpace(stripped[:cIdx])
		}

		rawOut = append(rawOut, line)
		lineMap = append(lineMap, origLine)
	}

	// Join parenthesized import blocks: import (\n    os,\n    sys\n)
	return joinParenthesizedImports(rawOut, lineMap)
}

// findUnescapedTripleQuote finds the first occurrence of a triple quote in s,
// ignoring occurrences that are preceded by an odd number of backslashes.
// isBareRelativeImport checks if a line is a bare relative import like "from . import x" or "from .. import x, y".
// These are imports where the module path is only dots (no module name after the dots).
func isBareRelativeImport(stripped string) bool {
	if !strings.HasPrefix(stripped, "from ") {
		return false
	}
	rest := strings.TrimPrefix(stripped, "from ")
	importIdx := strings.Index(rest, " import ")
	if importIdx == -1 {
		return false
	}
	module := strings.TrimSpace(rest[:importIdx])
	if module == "" {
		return false
	}
	for _, c := range module {
		if c != '.' {
			return false
		}
	}
	return true
}

// extractBareRelativeNames extracts the imported names from a bare relative import line.
// e.g., "from . import a, b" → ["a", "b"]
func extractBareRelativeNames(stripped string) []string {
	importIdx := strings.Index(stripped, " import ")
	if importIdx == -1 {
		return nil
	}
	rest := stripped[importIdx+len(" import "):]
	rest = strings.TrimPrefix(rest, "(")
	rest = strings.TrimSuffix(rest, ")")
	var names []string
	for _, part := range strings.Split(rest, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		name = strings.Fields(name)[0]
		if moduleOnlyRe.MatchString(name) {
			names = append(names, name)
		}
	}
	return names
}

// extractRelativeDots extracts the dot prefix from a bare relative import line.
// e.g., "from . import a" → ".", "from .. import a" → ".."
func extractRelativeDots(stripped string) string {
	importIdx := strings.Index(stripped, " import ")
	if importIdx == -1 {
		return ""
	}
	module := strings.TrimSpace(strings.TrimPrefix(stripped[:importIdx], "from "))
	return module
}

func findUnescapedTripleQuote(s, quote string) int {
	idx := 0
	for {
		pos := strings.Index(s[idx:], quote)
		if pos == -1 {
			return -1
		}
		absPos := idx + pos
		// Count preceding backslashes
		backslashes := 0
		for j := absPos - 1; j >= 0 && s[j] == '\\'; j-- {
			backslashes++
		}
		if backslashes%2 == 0 {
			return absPos
		}
		idx = absPos + 1
	}
}

func joinParenthesizedImports(lines []string, lineMap []int) ([]string, []int) {
	if len(lines) == 0 {
		return lines, lineMap
	}

	var out []string
	var outMap []int
	i := 0

	for i < len(lines) {
		stripped := strings.TrimSpace(lines[i])
		depth := parenDepth(stripped)
		if depth <= 0 || !isImportStart(stripped) {
			out = append(out, lines[i])
			outMap = append(outMap, lineMap[i])
			i++
			continue
		}
		parts := []string{stripped}
		baseLine := lineMap[i]
		i++
		for i < len(lines) && depth > 0 {
			s := strings.TrimSpace(lines[i])
			depth += parenDepth(s)
			parts = append(parts, s)
			i++
		}
		out = append(out, strings.Join(parts, " "))
		outMap = append(outMap, baseLine)
	}

	return out, outMap
}

func isImportStart(stripped string) bool {
	return strings.HasPrefix(stripped, "import ") || strings.HasPrefix(stripped, "import(") ||
		strings.HasPrefix(stripped, "from ")
}

func parenDepth(s string) int {
	depth := 0
	for _, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		}
	}
	return depth
}

func (Language) ResolveInternalTarget(_ port.FileSystem, spec port.ImportSpec, c port.Capsule, fileRel string) (string, bool) {
	pkg := c.CapsuleID
	if pkg == "" {
		return "", false
	}

	// Handle relative imports: .sibling, ..parent, ..parent.module
	if strings.HasPrefix(spec.Path, ".") {
		return resolveRelativeImport(spec.Path, pkg, fileRel)
	}

	if !namespaces.IsInternal(spec.Path, pkg) {
		return "", false
	}
	srcPrefix := resolveSourcePrefix(fileRel)
	rest := strings.TrimPrefix(spec.Path, pkg)
	rest = strings.TrimPrefix(rest, ".")
	pkgPath := strings.Replace(pkg, ".", "/", -1)
	if rest == "" {
		return filepath.ToSlash(filepath.Join(srcPrefix, pkgPath)), true
	}
	relPath := strings.Replace(rest, ".", "/", -1)
	return filepath.ToSlash(filepath.Join(srcPrefix, pkgPath, relPath)), true
}

func resolveRelativeImport(modulePath, pkg, fileRel string) (string, bool) {
	dots := 0
	for dots < len(modulePath) && modulePath[dots] == '.' {
		dots++
	}
	modPart := modulePath[dots:]

	// Compute current file's package relative to the capsule root
	fileParts := strings.Split(fileRel, "/")

	// Find the file's package: the path from capsule root to the file's directory
	fileDir := strings.Join(fileParts[:len(fileParts)-1], "/")

	// Strip source prefix (src/, lib/, etc.)
	srcPrefix := resolveSourcePrefix(fileRel)
	if srcPrefix != "" {
		fileDir = strings.TrimPrefix(fileDir, srcPrefix+"/")
	}

	// fileDir is now like "mypackage/sub/deep"
	filePkgParts := strings.Split(fileDir, "/")

	// Navigate up (dots - 1) levels from the current package
	// dots=1 means current package, dots=2 means parent, etc.
	levelsUp := dots - 1
	baseIdx := len(filePkgParts) - levelsUp
	if baseIdx < 0 {
		return "", false
	}

	baseParts := filePkgParts[:baseIdx]

	// Validate resolved path stays within the capsule
	pkgParts := strings.Split(pkg, ".")
	if len(baseParts) < len(pkgParts) {
		return "", false
	}
	for i, p := range pkgParts {
		if baseParts[i] != p {
			return "", false
		}
	}

	// Append the module portion
	if modPart != "" {
		modParts := strings.Split(modPart, ".")
		baseParts = append(baseParts, modParts...)
	}

	if len(baseParts) == 0 {
		return "", false
	}

	resolvedPath := strings.Join(baseParts, "/")
	if srcPrefix != "" {
		resolvedPath = srcPrefix + "/" + resolvedPath
	}
	return filepath.ToSlash(resolvedPath), true
}

// resolveSourcePrefix returns the capsule-relative source root, which can only
// be the first path segment: a nested directory named src/python/lib is part of
// the package, not a source root.
func resolveSourcePrefix(fileRel string) string {
	first := strings.SplitN(fileRel, "/", 2)[0]
	if first == "src" || first == "python" || first == "lib" {
		return first
	}
	return ""
}

func (Language) GetFileNamespace(_ port.FileSystem, _ string) (string, error) { return "", nil }
func (Language) SupportsFileGlobs() bool                                      { return false }
func (Language) Register(d port.CapsuleDiscovery) {
	d.Register("python", port.ManifestInfo{
		Names: []string{"pyproject.toml", "setup.py"},
		ParseFunc: func(fsys port.FileSystem, path string) (string, error) {
			dir := filepath.Dir(path)
			return findBaseCapsule(fsys, dir)
		},
		BaseIgnoreEntries: []string{"*test.py", "*_test.py", "test_*.py", "*conftest.py"},
	})
}

func findBaseCapsule(fsys port.FileSystem, projectRoot string) (string, error) {
	srcDirs := []string{
		filepath.Join(projectRoot, "src"),
		projectRoot,
	}
	var chosenSrc string
	for _, sd := range srcDirs {
		entries, err := fsys.ReadDir(sd)
		if err != nil {
			continue
		}
		hasPython := false
		for _, e := range entries {
			if e.IsDir() {
				initPath := filepath.Join(sd, e.Name(), "__init__.py")
				if _, statErr := fsys.Stat(initPath); statErr == nil {
					hasPython = true
					break
				}
				initStubs := filepath.Join(sd, e.Name(), "__init__.pyi")
				if _, statErr := fsys.Stat(initStubs); statErr == nil {
					hasPython = true
					break
				}
			}
		}
		if hasPython {
			chosenSrc = sd
			break
		}
	}
	if chosenSrc == "" {
		return "", nil
	}

	return namespaces.CommonPrefix(fsys, []string{chosenSrc}, exts)
}
