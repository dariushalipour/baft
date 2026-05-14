package dump

import (
	"errors"
	"io/fs"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

func shouldTrySelectiveExpansion(cfg draftConfig, err error) bool {
	return cfg.mode == draftModeMergedDirs && isFreshDraftCycle(err)
}

func retryCycleExpansion(fsys port.FileSystem, rootDir string, capsule port.Capsule, lang port.Language, repo port.GraphRepository, contractDir string, contractPath string, baseCfg draftConfig, cycleErr error) (*ContractDump, *AmendDiff, error, bool) {
	candidates := cycleExpansionCandidates(fsys, contractDir, lang, cycleErr, baseCfg)
	if len(candidates) == 0 {
		return nil, nil, nil, false
	}
	plans := expansionPlans(baseCfg, candidates)
	if len(plans) == 0 {
		return nil, nil, nil, false
	}
	var lastRes *ContractDump
	var lastErr error
	for _, cfg := range plans {
		res, err := dumpCapsule(fsys, capsule, lang, repo, rootDir, contractDir, cfg)
		if err != nil {
			return nil, nil, err, true
		}
		lastRes = res
		diff, err := amendContract(fsys, rootDir, capsule, lang, repo, contractPath, cfg)
		if err == nil {
			return res, diff, nil, true
		}
		lastErr = err
	}
	if lastRes == nil {
		return nil, nil, nil, false
	}
	return lastRes, nil, lastErr, true
}

func cycleExpansionCandidates(fsys port.FileSystem, contractDir string, lang port.Language, err error, cfg draftConfig) []string {
	var loadErr *contractLoadError
	if !errors.As(err, &loadErr) {
		return nil
	}
	for _, cycle := range loadErr.cycleGroups {
		unique := make([]string, 0, len(cycle))
		seen := map[string]bool{}
		for idx, nodeID := range cycle {
			if idx == len(cycle)-1 && len(cycle) > 1 && nodeID == cycle[0] {
				continue
			}
			if seen[nodeID] || cfg.isExpandedDir(nodeID) {
				continue
			}
			if scannableFileCount(fsys, contractDir, nodeID, lang) <= 1 {
				continue
			}
			seen[nodeID] = true
			unique = append(unique, nodeID)
		}
		if len(unique) == 0 {
			continue
		}
		sort.Slice(unique, func(i, j int) bool {
			left := scannableFileCount(fsys, contractDir, unique[i], lang)
			right := scannableFileCount(fsys, contractDir, unique[j], lang)
			if left != right {
				return left < right
			}
			return unique[i] < unique[j]
		})
		return unique
	}
	return nil
}

func expansionPlans(baseCfg draftConfig, candidates []string) []draftConfig {
	if len(candidates) == 0 {
		return nil
	}
	plans := []draftConfig{baseCfg.withExpandedDirs(candidates[0])}
	if len(candidates) > 1 {
		plans = append(plans, baseCfg.withExpandedDirs(candidates[len(candidates)-1]))
		plans = append(plans, baseCfg.withExpandedDirs(candidates...))
	}
	return plans
}

func isFreshDraftCycle(err error) bool {
	var loadErr *contractLoadError
	if !errors.As(err, &loadErr) {
		return false
	}
	return strings.Contains(loadErr.message, "cycle detected")
}

func summarizeContractLoadError(err error) string {
	msg := strings.TrimSpace(err.Error())
	if strings.Contains(msg, "cycle detected") {
		return "cycle detected"
	}
	return msg
}

func parseCycleGroups(msg string) [][]string {
	parts := strings.Split(strings.TrimSpace(msg), ";")
	cycles := make([][]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "cycle detected: ") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(part, "cycle detected: "))
		if body == "" {
			continue
		}
		nodes := strings.Split(body, " → ")
		if len(nodes) >= 2 {
			cycles = append(cycles, nodes)
		}
	}
	return cycles
}

func makeDumpError(label string, err error) DumpError {
	var loadErr *contractLoadError
	if errors.As(err, &loadErr) {
		return DumpError{Label: loadErr.contractPath, Err: loadErr}
	}
	return DumpError{Label: label, Err: err}
}

func importSpecForViolation(lang port.Language, fsys port.FileSystem, absPath string, line int, column int) (*port.ImportSpec, error) {
	imports, err := lang.ParseImports(fsys, absPath)
	if err != nil {
		return nil, err
	}
	for _, spec := range imports {
		if spec.Line == line && spec.Col == column {
			matched := spec
			return &matched, nil
		}
	}
	for _, spec := range imports {
		if spec.Line == line {
			matched := spec
			return &matched, nil
		}
	}
	return nil, nil
}

func discoverScopedContracts(fsys port.FileSystem, capsuleDir string) ([]string, error) {
	var contracts []string
	err := fsys.WalkDir(capsuleDir, func(abs string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		if d.Name() == port.ContractFile {
			contracts = append(contracts, abs)
		}
		return nil
	})
	return contracts, err
}

func nodeKey(path string, fileLevel bool) string {
	if fileLevel {
		return graph.NodeKeyForFile(path)
	}
	return graph.NodeKeyForDir(path)
}

func edgeCount(edges map[string]map[string]bool) int {
	n := 0
	for _, m := range edges {
		n += len(m)
	}
	return n
}
