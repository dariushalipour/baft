package dump

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/fs/ignorefs"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

// DumpResult holds the outcome of a dump run.
type DumpResult struct {
	Contracts []ContractDump
	Errors    []DumpError
}

// DumpError records a non-fatal error encountered while dumping a capsule.
type DumpError struct {
	Label string
	Err   error
}

func (d DumpError) Error() string {
	return fmt.Sprintf("%s: %s", d.Label, d.Err)
}

// ContractDump holds the outcome for a single contract dump.
type ContractDump struct {
	FilesEncountered int
	FilesScanned     int
	Nodes            int
	Edges            int
	ContractPath     string
	IsNew            bool
	AmendDiff        *AmendDiff
}

// AmendDiff reports the number of nodes and edges added during an amend run.
type AmendDiff struct {
	Nodes int
	Edges int
}

type draftMode int

const (
	draftModeExactFiles draftMode = iota
	draftModeMergedDirs
)

type draftConfig struct {
	mode         draftMode
	expandedDirs map[string]bool
}

type fileRecord struct {
	rel     string
	imports []port.ImportSpec
}

type contractLoadError struct {
	contractPath string
	message      string
	cycleGroups  [][]string
}

func (e *contractLoadError) Error() string {
	return "contract-load-error: " + e.message
}

// Run walks all capsules for every supplied language, parses every
// import in every scannable file, and writes a comprehensive BAFT.md
// that reflects the current dependency reality at maximum granularity.
func Run(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery) (*DumpResult, error) {
	return RunWith(fsys, rootDir, languages, repo, discovery, os.Stderr)
}

func RunWith(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery, logWriter io.Writer) (*DumpResult, error) {
	// Wrap the filesystem with ignore rules before discovery.
	wrapped, err := ignorefs.Wrap(fsys, ignorefs.Options{
		RootDir:           rootDir,
		BaseIgnoreEntries: discovery.BaseIgnoreEntries(),
	})
	if err != nil {
		if !errors.Is(err, ignorefs.ErrRepoRootUnreachable) {
			return nil, fmt.Errorf("ignorefs: %w", err)
		}
		fmt.Fprintln(logWriter, "warning: not inside a git repository — .gitignore/.baftignore rules from parent directories will not apply")
	}

	type entry struct {
		capsule port.Capsule
		lang    port.Language
	}
	var all []entry
	entries, err := discovery.Discover(wrapped, rootDir)
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
		startDir := e.capsule.Dir
		if strings.HasPrefix(rootDir, e.capsule.Dir+string(filepath.Separator)) || rootDir == e.capsule.Dir {
			startDir = rootDir
		}
		label := port.Label(e.capsule)
		contractDir, rootExists := service.FindOrCreateContractDir(wrapped, startDir, e.capsule.Dir)
		rootContractPath := filepath.Join(contractDir, port.ContractFile)

		contracts, err := discoverScopedContracts(wrapped, e.capsule.Dir)
		if err != nil {
			de := DumpError{Label: label, Err: err}
			result.Errors = append(result.Errors, de)
			fmt.Fprintf(logWriter, "dump: %s: %s\n", label, err)
			continue
		}

		var scopedPaths []string
		for _, cp := range contracts {
			if cp == rootContractPath {
				continue
			}
			scopedPaths = append(scopedPaths, cp)
		}

		for _, contractPath := range scopedPaths {
			diff, err := amendContract(wrapped, rootDir, e.capsule, e.lang, repo, contractPath, defaultDraftConfig(e.capsule, e.lang, filepath.Dir(contractPath)))
			if err != nil {
				de := DumpError{Label: label, Err: err}
				result.Errors = append(result.Errors, de)
				fmt.Fprintf(logWriter, "dump: %s: %s\n", de.Label, de.Err)
				continue
			}
			if diff != nil {
				result.Contracts = append(result.Contracts, ContractDump{
					ContractPath: contractPath,
					IsNew:        false,
					AmendDiff:    diff,
				})
			}
		}

		if rootExists {
			diff, err := amendContract(wrapped, rootDir, e.capsule, e.lang, repo, rootContractPath, defaultDraftConfig(e.capsule, e.lang, contractDir))
			if err != nil {
				de := makeDumpError(label, err)
				result.Errors = append(result.Errors, de)
				fmt.Fprintf(logWriter, "dump: %s: %s\n", de.Label, de.Err)
				continue
			}
			if diff != nil {
				result.Contracts = append(result.Contracts, ContractDump{
					ContractPath: rootContractPath,
					IsNew:        false,
					AmendDiff:    diff,
				})
			}
			continue
		}
		cfg := defaultDraftConfig(e.capsule, e.lang, contractDir)
		capsuleRes, err := dumpCapsule(wrapped, e.capsule, e.lang, repo, rootDir, contractDir, cfg)
		if err != nil {
			de := DumpError{Label: label, Err: err}
			result.Errors = append(result.Errors, de)
			fmt.Fprintf(logWriter, "dump: %s: %s\n", label, err)
			continue
		}
		diff, err := amendContract(wrapped, rootDir, e.capsule, e.lang, repo, rootContractPath, cfg)
		if err != nil {
			if shouldTrySelectiveExpansion(cfg, err) {
				retryRes, retryDiff, retryErr, handled := retryCycleExpansion(wrapped, rootDir, e.capsule, e.lang, repo, contractDir, rootContractPath, cfg, err)
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
					fmt.Fprintf(logWriter, "dump: %s: %s\n", de.Label, de.Err)
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
			fmt.Fprintf(logWriter, "dump: %s: %s\n", de.Label, de.Err)
			continue
		}
		capsuleRes.IsNew = true
		if diff != nil {
			capsuleRes.AmendDiff = diff
		}
		result.Contracts = append(result.Contracts, *capsuleRes)
	}

	return result, nil
}
