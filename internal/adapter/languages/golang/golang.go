package golang

import (
	"fmt"
	"go/parser"
	"go/token"
	"strings"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "go" }

func (Language) IsGovernedFile(rel string) bool {
	if len(rel) < 3 {
		return false
	}
	// Check .go suffix efficiently.
	if rel[len(rel)-3:] != ".go" {
		return false
	}
	// Check _test.go suffix efficiently.
	if len(rel) >= 8 && rel[len(rel)-8:] == "_test.go" {
		return false
	}
	return true
}

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, data, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	out := make([]port.ImportSpec, 0, len(file.Imports))
	for _, imp := range file.Imports {
		out = append(out, port.ImportSpec{
			Path:   strings.Trim(imp.Path.Value, `"`),
			Line:   fset.Position(imp.Path.Pos()).Line,
			Col:    fset.Position(imp.Path.Pos()).Column,
			ColEnd: fset.Position(imp.Path.End()).Column,
		})
	}
	return out, nil
}

func (Language) ResolveInternalTarget(_ port.FileSystem, spec port.ImportSpec, c port.Capsule, _ string) (string, bool) {
	prefix := c.CapsuleID + "/"
	if !strings.HasPrefix(spec.Path, prefix) {
		return "", false
	}
	return strings.TrimPrefix(spec.Path, prefix), true
}

func (Language) SupportsFileGlobs() bool { return false }

func RegisterDiscovery(d *service.CapsuleDiscovery) {
	d.Register("go", service.ManifestInfo{
		Names:     []string{"go.mod"},
		ParseFunc: readGoModulePath,
	})
}

func readGoModulePath(fsys port.FileSystem, path string) (string, error) {
	data, err := fsys.ReadFile(path)
	if err != nil {
		return "", err
	}
	lineStart := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			line := strings.TrimSpace(string(data[lineStart:i]))
			if len(line) >= 7 && line[:7] == "module " {
				return strings.TrimSpace(line[7:]), nil
			}
			lineStart = i + 1
		}
	}
	// Handle last line without newline.
	if lineStart < len(data) {
		line := strings.TrimSpace(string(data[lineStart:]))
		if len(line) >= 7 && line[:7] == "module " {
			return strings.TrimSpace(line[7:]), nil
		}
	}
	return "", fmt.Errorf("no module line in %s", path)
}
