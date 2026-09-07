package dump

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/fs/ignorefs"
	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

type DumpResult struct {
	Contracts []ContractDump
	Errors    []DumpError
}

type DumpError struct {
	Label string
	Err   error
}

func (d DumpError) Error() string {
	return fmt.Sprintf("%s: %s", d.Label, d.Err)
}

type ContractDump struct {
	FilesEncountered int
	FilesScanned     int
	Nodes            int
	Edges            int
	ContractPath     string
	IsNew            bool
	AmendDiff        *AmendDiff
}

// AmendDiff lists exactly what an amendment added to an existing contract:
// node ids, and edges rendered as "src --> dst".
type AmendDiff struct {
	Nodes []string
	Edges []string
}

type Options struct {
	Save   port.GraphSaveOptions
	DryRun bool
	Log    io.Writer
}

type draftMode int

const (
	draftModeExactFiles draftMode = iota
	draftModeMergedDirs
)

type draftConfig struct {
	mode          draftMode
	expandedDirs  map[string]bool
	saveOpts      port.GraphSaveOptions
	namespaceMode bool
}

// capsuleCtx bundles everything a dump of one capsule needs.
type capsuleCtx struct {
	fsys       port.FileSystem
	rootDir    string
	capsule    port.Capsule
	lang       port.Language
	repo       port.GraphRepository
	nestedDirs []string
}

type fileRecord struct {
	rel     string
	imports []port.ImportSpec
}

type contractError struct {
	contractPath string
	kind         string
	message      string
	cycleGroups  [][]string
}

func (e *contractError) Error() string {
	return e.kind + ": " + e.message
}

// dryRunFS keeps every write in memory so a dump can report what it would
// change without touching the working tree. Reads and stats see the buffered
// writes first, so the amend pass still observes the draft it just produced.
type dryRunFS struct {
	port.FileSystem
	mem *memfs.FS
}

func (f *dryRunFS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return f.mem.WriteFile(path, data, perm)
}

func (f *dryRunFS) ReadFile(path string) ([]byte, error) {
	if data, err := f.mem.ReadFile(path); err == nil {
		return data, nil
	}
	return f.FileSystem.ReadFile(path)
}

func (f *dryRunFS) Stat(path string) (os.FileInfo, error) {
	if info, err := f.mem.Stat(path); err == nil {
		return info, nil
	}
	return f.FileSystem.Stat(path)
}

// Run walks all capsules for every supplied language, parses every
// import in every scannable file, and writes a comprehensive contract file
// that reflects the current dependency reality at maximum granularity.
//
// When a contract already exists it is amended, not overwritten: every import
// the current contract forbids becomes an allowed edge. Options.DryRun reports
// those additions without writing anything.
func Run(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery) (*DumpResult, error) {
	return RunWith(fsys, rootDir, languages, repo, discovery, os.Stderr)
}

func RunWith(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery, logWriter io.Writer) (*DumpResult, error) {
	return RunWithOptions(fsys, rootDir, languages, repo, discovery, Options{
		Save: port.GraphSaveOptions{ColorPalette: port.ColorPaletteNone},
		Log:  logWriter,
	})
}

func RunWithOptions(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery, opts Options) (*DumpResult, error) {
	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		fsys = &dryRunFS{FileSystem: fsys, mem: memfs.New()}
	}

	wrapped, err := ignorefs.Wrap(fsys, ignorefs.Options{
		RootDir:           absRootDir,
		BaseIgnoreEntries: discovery.BaseIgnoreEntries(),
	})
	if err != nil {
		if !errors.Is(err, ignorefs.ErrRepoRootUnreachable) {
			return nil, fmt.Errorf("ignorefs: %w", err)
		}
		fmt.Fprintln(opts.Log, "warning: not inside a git repository — .gitignore/.baftignore rules from parent directories will not apply")
	}

	type entry struct {
		capsule port.Capsule
		lang    port.Language
	}
	var all []entry
	entries, err := discovery.Discover(context.Background(), wrapped, absRootDir)
	if err != nil {
		return nil, err
	}
	langMap := make(map[string]port.Language)
	for _, lang := range languages {
		langMap[lang.Name()] = lang
	}
	for _, e := range entries {
		lang := langMap[e.LangName]
		if lang != nil {
			all = append(all, entry{capsule: e.Capsule, lang: lang})
		}
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("no capsules found at %s", rootDir)
	}

	sort.Slice(all, func(i, j int) bool {
		di := strings.Count(filepath.Clean(all[i].capsule.Dir), string(filepath.Separator))
		dj := strings.Count(filepath.Clean(all[j].capsule.Dir), string(filepath.Separator))
		if di != dj {
			return di > dj
		}
		return port.Label(all[i].capsule) < port.Label(all[j].capsule)
	})

	result := &DumpResult{}

	for _, e := range all {
		var nested []string
		for _, other := range all {
			if strings.HasPrefix(other.capsule.Dir, e.capsule.Dir+string(filepath.Separator)) {
				nested = append(nested, other.capsule.Dir)
			}
		}
		cc := capsuleCtx{fsys: wrapped, rootDir: absRootDir, capsule: e.capsule, lang: e.lang, repo: repo, nestedDirs: nested}

		startDir := e.capsule.Dir
		if strings.HasPrefix(absRootDir, e.capsule.Dir+string(filepath.Separator)) || absRootDir == e.capsule.Dir {
			startDir = absRootDir
		}
		label := port.Label(e.capsule)
		contractDir, rootExists := service.FindOrCreateContractDir(wrapped, startDir, e.capsule.Dir)
		rootContractPath := filepath.Join(contractDir, port.ContractFile)

		contracts, err := discoverScopedContracts(wrapped, e.capsule.Dir)
		if err != nil {
			de := DumpError{Label: label, Err: err}
			result.Errors = append(result.Errors, de)
			fmt.Fprintf(opts.Log, "dump: %s: %s\n", label, err)
			continue
		}

		for _, contractPath := range contracts {
			if contractPath == rootContractPath {
				continue
			}
			diff, err := amendExisting(cc, contractPath, opts.Save)
			if err != nil {
				de := DumpError{Label: label, Err: err}
				result.Errors = append(result.Errors, de)
				fmt.Fprintf(opts.Log, "dump: %s: %s\n", de.Label, de.Err)
				continue
			}
			if diff != nil {
				result.Contracts = append(result.Contracts, ContractDump{ContractPath: contractPath, AmendDiff: diff})
			}
		}

		if rootExists {
			diff, err := amendExisting(cc, rootContractPath, opts.Save)
			if err != nil {
				de := makeDumpError(label, err)
				result.Errors = append(result.Errors, de)
				fmt.Fprintf(opts.Log, "dump: %s: %s\n", de.Label, de.Err)
				continue
			}
			if diff != nil {
				result.Contracts = append(result.Contracts, ContractDump{ContractPath: rootContractPath, AmendDiff: diff})
			}
			continue
		}

		cfg := defaultDraftConfig(e.capsule, e.lang, contractDir, opts.Save, nil)
		capsuleRes, err := dumpCapsule(cc, contractDir, cfg)
		if err != nil {
			de := DumpError{Label: label, Err: err}
			result.Errors = append(result.Errors, de)
			fmt.Fprintf(opts.Log, "dump: %s: %s\n", label, err)
			continue
		}
		diff, err := amendDraft(cc, rootContractPath, cfg)
		if err != nil {
			if shouldTrySelectiveExpansion(cfg, err) {
				retryRes, retryDiff, retryErr, handled := retryCycleExpansion(cc, contractDir, rootContractPath, cfg, err)
				if handled {
					if retryErr == nil || isFreshDraftCycle(retryErr) {
						retryRes.IsNew = true
						if retryErr == nil && retryDiff != nil {
							retryRes.AmendDiff = retryDiff
						}
						result.Contracts = append(result.Contracts, *retryRes)
						continue
					}
					de := makeDumpError(label, retryErr)
					result.Errors = append(result.Errors, de)
					fmt.Fprintf(opts.Log, "dump: %s: %s\n", de.Label, de.Err)
					continue
				}
			}
			if isFreshDraftCycle(err) {
				capsuleRes.IsNew = true
				result.Contracts = append(result.Contracts, *capsuleRes)
				continue
			}
			de := makeDumpError(label, err)
			result.Errors = append(result.Errors, de)
			fmt.Fprintf(opts.Log, "dump: %s: %s\n", de.Label, de.Err)
			continue
		}
		capsuleRes.IsNew = true
		capsuleRes.AmendDiff = diff
		result.Contracts = append(result.Contracts, *capsuleRes)
	}

	return result, nil
}

// amendExisting amends a contract the user already maintains, deriving the
// draft config from the contract it just loaded.
func amendExisting(cc capsuleCtx, contractPath string, saveOpts port.GraphSaveOptions) (*AmendDiff, error) {
	current, err := loadContract(cc, contractPath)
	if err != nil {
		return nil, err
	}
	contractDir := filepath.Dir(contractPath)
	return amendContract(cc, contractPath, current, defaultDraftConfig(cc.capsule, cc.lang, contractDir, saveOpts, current))
}

// amendDraft amends the draft dumpCapsule just wrote, keeping its config.
func amendDraft(cc capsuleCtx, contractPath string, cfg draftConfig) (*AmendDiff, error) {
	current, err := loadContract(cc, contractPath)
	if err != nil {
		return nil, err
	}
	return amendContract(cc, contractPath, current, cfg)
}

func loadContract(cc capsuleCtx, contractPath string) (*graph.Graph, error) {
	raw, err := cc.fsys.ReadFile(contractPath)
	if err != nil {
		return nil, &contractError{contractPath: contractPath, kind: "contract-load-error", message: err.Error()}
	}
	g, err := cc.repo.Load(string(raw))
	if err != nil {
		return nil, &contractError{contractPath: contractPath, kind: "contract-load-error", message: strings.TrimSpace(err.Error())}
	}
	return g, nil
}
