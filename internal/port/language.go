package port

// ConfigFile is the name of the Baft contract file.
const ConfigFile = "BAFT.md"

// Label returns the absolute directory path of a capsule.
func Label(c Capsule, _ string) string {
	return c.Dir
}

// ImportSpec describes a single import found in a source file.
type ImportSpec struct {
	Path   string
	Line   int
	Col    int
	ColEnd int
}

// Language abstracts per-language import parsing so the
// node-check domain (Graph + rules) stays language-agnostic.
// Capsule discovery has been moved to the shared CapsuleDiscovery service.
type Language interface {
	Name() string
	IsGovernedFile(rel string) bool
	ParseImports(fileSystem FileSystem, absPath string) ([]ImportSpec, error)
	ResolveInternalTarget(fileSystem FileSystem, spec ImportSpec, c Capsule, fileRel string) (targetDir string, internal bool)
	SupportsFileGlobs() bool
}

// ParsedImports caches the result of ParseImports for a given file path.
type ParsedImports struct {
	Imports []ImportSpec
	Hash    string
}

// ShouldSkipDir returns true for directory names that should never be
// walked during package discovery (build artifacts, dependencies, etc.).
func ShouldSkipDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn":
		return true
	case "node_modules", "vendor", ".venv", "venv":
		return true
	case "build", "dist", "out", "target", ".next", ".kotlin":
		return true
	case ".dart_tool", ".pub", ".nuxt", ".svelte-kit":
		return true
	case "__pycache__", ".pytest_cache", ".tox":
		return true
	case ".idea", ".vscode", ".vs":
		return true
	case "coverage", "coverage.lcov":
		return true
	}
	if name != "." && len(name) > 0 && name[0] == '.' {
		return true
	}
	return false
}

// Capsule is one unit of node-checking: a capsule directory and an opaque
// capsule identifier used by Language.ResolveInternalTarget.
type Capsule struct {
	Dir       string
	CapsuleID string
}
