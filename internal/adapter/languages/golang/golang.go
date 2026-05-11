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
	if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
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
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	return "", fmt.Errorf("no module line in %s", path)
}
