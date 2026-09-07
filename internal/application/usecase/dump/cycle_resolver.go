package dump

import (
	"context"
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

func retryCycleExpansion(cc capsuleCtx, contractDir string, contractPath string, baseCfg draftConfig, cycleErr error) (*ContractDump, *AmendDiff, error, bool) {
	candidates := cycleExpansionCandidates(cc.fsys, contractDir, cc.lang, cycleErr, baseCfg)
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
		res, err := dumpCapsule(cc, contractDir, cfg)
		if err != nil {
			return nil, nil, err, true
		}
		lastRes = res
		diff, err := amendDraft(cc, contractPath, cfg)
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
	var loadErr *contractError
	if !errors.As(err, &loadErr) {
		return nil
	}
	unique := make([]string, 0, len(loadErr.cycleGroups))
	seen := map[string]bool{}
	for _, cycle := range loadErr.cycleGroups {
		for _, nodeID := range cycle {
			if seen[nodeID] || cfg.isExpandedDir(nodeID) {
				continue
			}
			if scannableFileCount(fsys, contractDir, nodeID, lang) <= 1 {
				continue
			}
			seen[nodeID] = true
			unique = append(unique, nodeID)
		}
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
	var loadErr *contractError
	if !errors.As(err, &loadErr) {
		return false
	}
	return loadErr.kind == "circular-dependency"
}

func makeDumpError(label string, err error) DumpError {
	var loadErr *contractError
	if errors.As(err, &loadErr) {
		return DumpError{Label: loadErr.contractPath, Err: loadErr}
	}
	return DumpError{Label: label, Err: err}
}

func summarizeContractValidationErrors(errors []port.Violation) string {
	parts := make([]string, 0, len(errors))
	seen := make(map[string]bool, len(errors))
	for _, violation := range errors {
		msg := normalizeValidationMessage(violation.Message)
		if violation.Rule == "circular-dependency" {
			return "circular dependency"
		}
		if seen[msg] {
			continue
		}
		seen[msg] = true
		parts = append(parts, msg)
	}
	return strings.Join(parts, "; ")
}

func contractValidationKind(errors []port.Violation) string {
	if len(errors) == 0 {
		return ""
	}
	for _, violation := range errors {
		if violation.Rule == "circular-dependency" {
			return violation.Rule
		}
	}
	return errors[0].Rule
}

func normalizeValidationMessage(message string) string {
	message = strings.TrimSpace(message)
	if idx := strings.LastIndex(message, " ("); idx >= 0 && strings.HasSuffix(message, ")") {
		return message[:idx]
	}
	return message
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
	err := fsys.WalkDir(context.Background(), capsuleDir, func(abs string, d fs.DirEntry) error {
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
