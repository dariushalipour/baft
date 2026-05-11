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
		// Call fset.Position once per import, reusing the result.
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

func (Language) SupportsFileGlobs() bool { return false }

func RegisterDiscovery(d *service.CapsuleDiscovery) {
	d.Register("go", service.ManifestInfo{
		Names:     []string{"go.mod"},
		ParseFunc: readGoModulePath,
	})
}

func readGoModulePath(fsys port.FileSystem, modPath string) (string, error) {
	data, err := fsys.ReadFile(modPath)
	if err != nil {
		return "", err
	}
	lineStart := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			line := data[lineStart:i]
			// Trim whitespace by scanning bytes.
			trimStart := 0
			for trimStart < len(line) && (line[trimStart] == ' ' || line[trimStart] == '\t') {
				trimStart++
			}
			trimEnd := len(line)
			for trimEnd > trimStart && (line[trimEnd-1] == ' ' || line[trimEnd-1] == '\t' || line[trimEnd-1] == '\r') {
				trimEnd--
			}
			trimmed := line[trimStart:trimEnd]
			if len(trimmed) >= 7 && trimmed[0] == 'm' && trimmed[1] == 'o' && trimmed[2] == 'd' && trimmed[3] == 'u' && trimmed[4] == 'l' && trimmed[5] == 'e' && trimmed[6] == ' ' {
				// Trim the module name value.
				modStart := 7
				for modStart < len(trimmed) && (trimmed[modStart] == ' ' || trimmed[modStart] == '\t') {
					modStart++
				}
				modEnd := len(trimmed)
				for modEnd > modStart && (trimmed[modEnd-1] == ' ' || trimmed[modEnd-1] == '\t') {
					modEnd--
				}
				return string(trimmed[modStart:modEnd]), nil
			}
			lineStart = i + 1
		}
	}
	// Handle last line without newline.
	if lineStart < len(data) {
		line := data[lineStart:]
		trimStart := 0
		for trimStart < len(line) && (line[trimStart] == ' ' || line[trimStart] == '\t') {
			trimStart++
		}
		trimEnd := len(line)
		for trimEnd > trimStart && (line[trimEnd-1] == ' ' || line[trimEnd-1] == '\t' || line[trimEnd-1] == '\r') {
			trimEnd--
		}
		trimmed := line[trimStart:trimEnd]
		if len(trimmed) >= 7 && trimmed[0] == 'm' && trimmed[1] == 'o' && trimmed[2] == 'd' && trimmed[3] == 'u' && trimmed[4] == 'l' && trimmed[5] == 'e' && trimmed[6] == ' ' {
			modStart := 7
			for modStart < len(trimmed) && (trimmed[modStart] == ' ' || trimmed[modStart] == '\t') {
				modStart++
			}
			modEnd := len(trimmed)
			for modEnd > modStart && (trimmed[modEnd-1] == ' ' || trimmed[modEnd-1] == '\t') {
				modEnd--
			}
			return string(trimmed[modStart:modEnd]), nil
		}
	}
	return "", fmt.Errorf("no module line in %s", modPath)
}
