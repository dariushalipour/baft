package dart

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

// lineOffsetsCache is a cache for byte-offset→line/col maps per file path.
var lineOffsetsCache sync.Map

type Language struct{}

func (Language) Name() string { return "dart" }

func (Language) IsGovernedFile(rel string) bool {
	if !strings.HasSuffix(rel, ".dart") {
		return false
	}
	if !strings.HasPrefix(rel, "lib/") {
		return false
	}
	base := path.Base(rel)
	if strings.HasSuffix(base, "_test.dart") {
		return false
	}
	if strings.HasSuffix(base, ".g.dart") || strings.HasSuffix(base, ".freezed.dart") {
		return false
	}
	return true
}

var directiveRe = regexp.MustCompile(`(?m)^\s*(?:import|export|part)\s+['"]([^'"]+)['"]`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	indices := directiveRe.FindAllSubmatchIndex(data, -1)
	out := make([]port.ImportSpec, 0, len(indices))

	lineOffsets, ok := lineOffsetsCache.Load(absPath)
	if !ok {
		lineOffsets = makeLineOffsets(data)
		lineOffsetsCache.Store(absPath, lineOffsets)
	}

	for _, m := range indices {
		p := string(data[m[2]:m[3]])
		line, col := offsetToLineCol(lineOffsets.([]int), data, m[2])
		out = append(out, port.ImportSpec{Path: p, Line: line, Col: col, ColEnd: col + len(p)})
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
// Both line and col are 1-indexed.
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

func RegisterDiscovery(d *service.CapsuleDiscovery) {
	d.Register("dart", service.ManifestInfo{
		Names:     []string{"pubspec.yaml"},
		ParseFunc: readPubspecName,
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
