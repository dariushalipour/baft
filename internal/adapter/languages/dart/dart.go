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

var directiveRe = regexp.MustCompile(`(?m)^\s*(?:import|export|part)\s+['"]([^'"]+)['"]`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	indices := directiveRe.FindAllSubmatchIndex(data, -1)
	out := make([]port.ImportSpec, 0, len(indices))
	lineOffsets := lineoffsets.MakeLineOffsets(data)

	for _, m := range indices {
		p := string(data[m[2]:m[3]])
		line, col := lineoffsets.OffsetToLineCol(lineOffsets, data, m[2])
		out = append(out, port.ImportSpec{Path: p, Line: line, Col: col, ColEnd: col + len(p)})
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

func (Language) SupportsFileGlobs() bool { return true }
func (Language) Register(d port.CapsuleDiscovery) {
	d.Register("dart", port.ManifestInfo{
		Names:             []string{"pubspec.yaml"},
		ParseFunc:         readPubspecName,
		BaseIgnoreEntries: []string{".dart_tool", ".pub", "*.g.dart", "*.freezed.dart", "*_test.dart"},
	})
}

var pubspecNameRe = regexp.MustCompile(`(?m)^name\s*:\s*([A-Za-z_][A-Za-z0-9_]*)\s*$`)

func readPubspecName(fsys port.FileSystem, path string) (string, error) {
	data, err := fsys.ReadFile(path)
	if err != nil {
		return "", err
	}
	m := pubspecNameRe.FindSubmatch(data)
	if m == nil {
		return "", fmt.Errorf("no name: line in %s", path)
	}
	return string(m[1]), nil
}
