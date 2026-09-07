package golang

import (
	"fmt"
	"go/parser"
	"go/token"
	"strings"

	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "go" }

func (Language) IsScannableFile(rel string) bool {
	if len(rel) < 3 {
		return false
	}
	if rel[len(rel)-3:] != ".go" {
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
		pos := fset.Position(imp.Path.Pos())
		endPos := fset.Position(imp.Path.End())
		out = append(out, port.ImportSpec{
			Path:   strings.Trim(imp.Path.Value, `"`),
			Line:   pos.Line,
			Col:    pos.Column,
			ColEnd: endPos.Column,
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

func (Language) GetFileNamespace(_ port.FileSystem, _ string) (string, error) { return "", nil }
func (Language) SupportsFileGlobs() bool                                      { return false }
func (Language) Register(d port.CapsuleDiscovery) {
	d.Register("go", port.ManifestInfo{
		Names:             []string{"go.mod"},
		ParseFunc:         readGoModulePath,
		BaseIgnoreEntries: []string{"vendor", "*_test.go"},
	})
}

func readGoModulePath(fsys port.FileSystem, modPath string) (string, error) {
	data, err := fsys.ReadFile(modPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "module" {
			continue
		}
		// Go 1.16+ allows quoted module paths.
		return strings.Trim(fields[1], `"`), nil
	}
	return "", fmt.Errorf("no module line in %s", modPath)
}
