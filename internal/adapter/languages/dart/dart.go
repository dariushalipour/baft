package dart

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

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
	for _, m := range indices {
		p := string(data[m[2]:m[3]])
		line, col := byteOffsetToLineCol(data, m[2])
		out = append(out, port.ImportSpec{Path: p, Line: line, Col: col, ColEnd: col + len(p)})
	}
	return out, nil
}

func byteOffsetToLineCol(data []byte, offset int) (int, int) {
	if offset > len(data) {
		offset = len(data)
	}
	line, col := 1, 1
	for i := 0; i < offset; i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
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
