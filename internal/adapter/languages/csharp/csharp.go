package csharp

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/languages/internal/lineoffsets"
	"github.com/dariushalipour/baft/internal/adapter/languages/internal/namespaces"
	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "csharp" }

func (Language) IsScannableFile(rel string) bool {
	return strings.HasSuffix(rel, ".cs")
}

var importRe = regexp.MustCompile(`(?m)^\s*(?:global\s+)?using\s+(static\s+)?([A-Za-z_][\w]*(?:\.[A-Za-z_][\w]*)*)(?:\s*=\s*([A-Za-z_][\w]*(?:\.[A-Za-z_][\w]*)*))?`)

var namespaceRe = regexp.MustCompile(`(?m)^\s*namespace\s+([A-Za-z_][\w]*(?:\.[A-Za-z_][\w]*)*)`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	indices := importRe.FindAllSubmatchIndex(data, -1)
	out := make([]port.ImportSpec, 0, len(indices))
	lineOffsets := lineoffsets.MakeLineOffsets(data)

	for _, m := range indices {
		// Group 1: "static" marker (if present), Group 2: namespace/alias, Group 3: alias target
		isStatic := m[2] != -1 && m[3] != -1
		if isStatic {
			// Static imports (e.g. "using static System.Math") are not namespace
			// dependencies — they only bring members into scope.
			continue
		}
		// For "using X = Actual.Namespace;" group 2 is the alias "X" and group 3
		// is the actual namespace. Use the actual namespace for resolution.
		nsStart, nsEnd := m[4], m[5]
		hasAlias := m[6] != -1
		if hasAlias {
			nsStart, nsEnd = m[6], m[7]
		}
		ns := string(data[nsStart:nsEnd])
		// Point Col to the first identifier (alias if present) so violations highlight
		// the visible token the developer wrote.
		line, col := lineoffsets.OffsetToLineCol(lineOffsets, data, m[4])
		// ColEnd must cover the visible token (alias), not the resolved namespace.
		colSpan := m[5] - m[4]
		out = append(out, port.ImportSpec{
			Path:      ns,
			Namespace: ns,
			Line:      line,
			Col:       col,
			ColEnd:    col + colSpan,
		})
	}
	return out, nil
}

func (Language) GetFileNamespace(fsys port.FileSystem, absPath string) (string, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	m := namespaceRe.FindSubmatch(data)
	if m == nil {
		return "", nil
	}
	return string(m[1]), nil
}

func (Language) ResolveInternalTarget(_ port.FileSystem, spec port.ImportSpec, c port.Capsule, _ string) (string, bool) {
	basePkg := c.CapsuleID
	if basePkg == "" {
		return "", false
	}
	ns := spec.Path
	if !namespaces.IsInternal(ns, basePkg) {
		return "", false
	}
	rest := strings.TrimPrefix(ns, basePkg)
	rest = strings.TrimPrefix(rest, ".")
	if rest == "" {
		return ".", true
	}
	return strings.Replace(rest, ".", "/", -1), true
}

func (Language) SupportsFileGlobs() bool { return false }
func (Language) Register(d port.CapsuleDiscovery) {
	d.Register("csharp", port.ManifestInfo{
		Names:             []string{"*.csproj"},
		ParseFunc:         ReadCsprojName,
		BaseIgnoreEntries: []string{"*Test.cs", "*Tests.cs", "*.Test.cs", "*.Tests.cs", "*.Designer.cs", "*.generated.cs"},
	})
}

func ReadCsprojName(fsys port.FileSystem, path string) (string, error) {
	data, err := fsys.ReadFile(path)
	if err != nil {
		return "", err
	}
	lineStart := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			line := string(data[lineStart:i])
			if name := extractCsprojName(line); name != "" {
				return name, nil
			}
			lineStart = i + 1
		}
	}
	// Handle last line without newline
	if lineStart < len(data) {
		line := string(data[lineStart:])
		if name := extractCsprojName(line); name != "" {
			return name, nil
		}
	}
	// Fall back to directory name (without extension)
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return base, nil
}

func extractCsprojName(line string) string {
	trimmed := strings.TrimSpace(line)
	// Check for <RootNamespace>Name</RootNamespace>
	if strings.HasPrefix(trimmed, "<RootNamespace>") {
		end := strings.Index(trimmed, "</RootNamespace>")
		if end > 0 {
			return trimmed[len("<RootNamespace>"):end]
		}
	}
	// Check for <AssemblyName>Name</AssemblyName>
	if strings.HasPrefix(trimmed, "<AssemblyName>") {
		end := strings.Index(trimmed, "</AssemblyName>")
		if end > 0 {
			return trimmed[len("<AssemblyName>"):end]
		}
	}
	return ""
}
