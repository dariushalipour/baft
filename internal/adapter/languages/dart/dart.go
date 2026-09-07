package dart

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/languages/internal/lineoffsets"
	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "dart" }

func (Language) IsScannableFile(rel string) bool {
	n := len(rel)
	if n < 5 {
		return false
	}
	return rel[n-5:] == ".dart"
}

// directiveRe captures the whole uri list of a directive, including the
// `if (config) 'other.dart'` branches of a conditional import.
var directiveRe = regexp.MustCompile(`(?m)^\s*(?:import|export|part)\s+(['"][^'"]+['"](?:\s*if\s*\([^)]*\)\s*['"][^'"]+['"])*)`)
var uriRe = regexp.MustCompile(`['"]([^'"]+)['"]`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	indices := directiveRe.FindAllSubmatchIndex(data, -1)
	out := make([]port.ImportSpec, 0, len(indices))
	lineOffsets := lineoffsets.MakeLineOffsets(data)

	for _, m := range indices {
		for _, u := range uriRe.FindAllSubmatchIndex(data[m[2]:m[3]], -1) {
			start := m[2] + u[2]
			p := string(data[start : m[2]+u[3]])
			line, col := lineoffsets.OffsetToLineCol(lineOffsets, data, start)
			out = append(out, port.ImportSpec{Path: p, Line: line, Col: col, ColEnd: col + len(p)})
		}
	}
	return out, nil
}

func (Language) ResolveInternalTarget(_ port.FileSystem, spec port.ImportSpec, c port.Capsule, fileRel string) (string, bool) {
	if strings.HasPrefix(spec.Path, "dart:") {
		return "", false
	}
	if strings.HasPrefix(spec.Path, "package:") {
		rest := strings.TrimPrefix(spec.Path, "package:")
		slash := strings.IndexByte(rest, '/')
		if slash < 0 {
			return "", false
		}
		pkgName := rest[:slash]
		subPath := rest[slash+1:]
		if pkgName != c.CapsuleID {
			return "", false
		}
		return path.Join("lib", subPath), true
	}
	base := path.Dir(fileRel)
	full := path.Clean(path.Join(base, spec.Path))
	if strings.HasPrefix(full, "../") || full == ".." {
		return "", false
	}
	return full, true
}

func (Language) GetFileNamespace(_ port.FileSystem, _ string) (string, error) { return "", nil }
func (Language) SupportsFileGlobs() bool                                      { return true }
func (Language) Register(d port.CapsuleDiscovery) {
	d.Register("dart", port.ManifestInfo{
		Names:             []string{"pubspec.yaml"},
		ParseFunc:         readPubspecName,
		BaseIgnoreEntries: []string{"*_test.dart"},
	})
}

// One group per quoting style, so a quote only counts when it is matched.
var pubspecNameRe = regexp.MustCompile(`(?m)^name\s*:\s*(?:'([A-Za-z_]\w*)'|"([A-Za-z_]\w*)"|([A-Za-z_]\w*))\s*(?:#.*)?$`)

func readPubspecName(fsys port.FileSystem, path string) (string, error) {
	data, err := fsys.ReadFile(path)
	if err != nil {
		return "", err
	}
	// Exactly one group matches, and every group comes after the full match, so
	// the last non-empty submatch is the name.
	name := ""
	for _, group := range pubspecNameRe.FindSubmatch(data) {
		if len(group) > 0 {
			name = string(group)
		}
	}
	if name == "" {
		return "", fmt.Errorf("no name: line in %s", path)
	}
	return name, nil
}
