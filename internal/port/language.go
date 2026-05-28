package port

const ContractFile = "BAFT.md"

func Label(c Capsule) string {
	return c.Dir
}

type ImportSpec struct {
	Path      string
	Namespace string // raw namespace from import statement (e.g., "MyApp.Domain.Entities")
	Line      int
	Col       int
	ColEnd    int
}

// Language abstracts per-language import parsing so the
// node-check domain (Graph + rules) stays language-agnostic.
// Capsule discovery has been moved to the shared CapsuleDiscovery service.
type Language interface {
	Name() string
	IsScannableFile(rel string) bool
	ParseImports(fileSystem FileSystem, absPath string) ([]ImportSpec, error)
	GetFileNamespace(fileSystem FileSystem, absPath string) (string, error)
	ResolveInternalTarget(fileSystem FileSystem, spec ImportSpec, c Capsule, fileRel string) (targetDir string, internal bool)
	SupportsFileGlobs() bool
	Register(d CapsuleDiscovery)
}

type ParsedImports struct {
	Imports []ImportSpec
	Hash    string
}

type ManifestInfo struct {
	// Names are the file names to look for (e.g. ["go.mod"], ["build.gradle.kts", "build.gradle"]).
	Names []string
	// ParseFunc reads a manifest file and extracts the capsule identifier.
	ParseFunc func(fsys FileSystem, path string) (string, error)
	// BaseIgnoreEntries are directory names and file glob patterns to skip during
	// discovery (e.g. ["vendor"], ["node_modules"], ["*_test.go"]).
	BaseIgnoreEntries []string
}

// CapsuleDiscovery is the minimal interface needed for language registration.
// It is implemented by service.CapsuleDiscovery to avoid circular imports.
type CapsuleDiscovery interface {
	Register(name string, info ManifestInfo)
}

type Capsule struct {
	Dir       string
	CapsuleID string
}
