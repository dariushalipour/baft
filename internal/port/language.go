package port

// ContractFile is the name of the Baft contract file.
const ContractFile = "BAFT.md"

// Label returns the absolute directory path of a capsule.
func Label(c Capsule) string {
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
	IsScannableFile(rel string) bool
	ParseImports(fileSystem FileSystem, absPath string) ([]ImportSpec, error)
	ResolveInternalTarget(fileSystem FileSystem, spec ImportSpec, c Capsule, fileRel string) (targetDir string, internal bool)
	SupportsFileGlobs() bool
	Register(d CapsuleDiscovery)
}

// ParsedImports caches the result of ParseImports for a given file path.
type ParsedImports struct {
	Imports []ImportSpec
	Hash    string
}

// ManifestInfo describes how to find and parse a build manifest for a language.
type ManifestInfo struct {
	// Names are the file names to look for (e.g. ["go.mod"], ["build.gradle.kts", "build.gradle"]).
	Names []string
	// ParseFunc reads a manifest file and extracts the capsule identifier.
	ParseFunc func(fsys FileSystem, path string) (string, error)
	// BaseIgnoreEntries are directories to skip during discovery (e.g. ["vendor"], ["node_modules"]).
	BaseIgnoreEntries []string
}

// CapsuleDiscovery is the minimal interface needed for language registration.
// It is implemented by service.CapsuleDiscovery to avoid circular imports.
type CapsuleDiscovery interface {
	Register(name string, info ManifestInfo)
}

// Capsule is one unit of node-checking: a capsule directory and an opaque
// capsule identifier used by Language.ResolveInternalTarget.
type Capsule struct {
	Dir       string
	CapsuleID string
}
