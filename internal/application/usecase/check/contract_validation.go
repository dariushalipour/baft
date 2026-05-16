package check

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

type ContractValidationResult struct {
	Errors                []port.Violation
	HasOverlapError       bool
	HasDuplicateGlobError bool
	HasInvalidGlobError   bool
}

func ValidateContract(fsys port.FileSystem, lang port.Language, contractPath string, g *graph.Graph) ContractValidationResult {
	return validateContractGraph(fsys, lang, contractPath, g)
}

func validateContractGraph(fsys port.FileSystem, lang port.Language, contractPath string, g *graph.Graph) ContractValidationResult {
	var result ContractValidationResult
	if g == nil {
		return result
	}

	result.Errors = append(result.Errors, emptyGlobErrors(g, contractPath)...)
	result.Errors = append(result.Errors, undefinedEdgeNodeErrors(g, contractPath)...)
	result.Errors = append(result.Errors, cycleErrors(g, contractPath)...)

	duplicateGlobErrors := duplicateNodeGlobErrors(g, contractPath)
	if len(duplicateGlobErrors) > 0 {
		result.HasDuplicateGlobError = true
		result.Errors = append(result.Errors, duplicateGlobErrors...)
	}

	invalidGlobErrors := invalidNodeGlobErrors(g, contractPath)
	if len(invalidGlobErrors) > 0 {
		result.HasInvalidGlobError = true
		result.Errors = append(result.Errors, invalidGlobErrors...)
	}

	overlapErrors := contractOverlapErrors(fsys, lang, g, contractPath)
	if len(overlapErrors) > 0 {
		result.HasOverlapError = true
		result.Errors = append(result.Errors, overlapErrors...)
	}

	return result
}

func emptyGlobErrors(g *graph.Graph, contractPath string) []port.Violation {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var errs []port.Violation
	for _, id := range ids {
		if g.Nodes[id] == "" {
			errs = append(errs, makeEmptyNodeGlobError(contractPath, g.NodeLines[id], id))
		}
	}
	return errs
}

func undefinedEdgeNodeErrors(g *graph.Graph, contractPath string) []port.Violation {
	var errs []port.Violation
	for src, dsts := range g.Edges {
		for dst := range dsts {
			line := g.EdgeLines[src+"\t"+dst]
			if _, ok := g.Nodes[src]; !ok {
				errs = append(errs, makeUndefinedEdgeNodeError(contractPath, line, src))
			}
			if _, ok := g.Nodes[dst]; !ok {
				errs = append(errs, makeUndefinedEdgeNodeError(contractPath, line, dst))
			}
		}
	}
	sort.Slice(errs, func(i, j int) bool {
		return errs[i].Message < errs[j].Message
	})
	return errs
}

func cycleErrors(g *graph.Graph, contractPath string) []port.Violation {
	type state byte
	const (
		white state = iota
		gray
		black
	)

	color := make(map[string]state, len(g.Nodes))
	for id := range g.Nodes {
		color[id] = white
	}

	path := make([]string, 0, len(g.Nodes))
	var errs []port.Violation
	seenCycles := make(map[string]struct{})

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = gray
		path = append(path, node)
		for dst := range g.Edges[node] {
			c, ok := color[dst]
			if !ok {
				continue
			}
			if c == gray {
				cycleStart := -1
				for i, p := range path {
					if p == dst {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycleStr := ""
					for i := cycleStart; i < len(path); i++ {
						if i > cycleStart {
							cycleStr += " → "
						}
						cycleStr += path[i]
					}
					cycleStr += " → " + dst
					if _, ok := seenCycles[cycleStr]; !ok {
						seenCycles[cycleStr] = struct{}{}
						line := g.EdgeLines[path[len(path)-1]+"\t"+dst]
						errs = append(errs, makeCycleError(contractPath, line, cycleStr))
					}
				}
			} else if c == white {
				dfs(dst)
			}
		}
		path = path[:len(path)-1]
		color[node] = black
	}

	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if color[id] == white {
			dfs(id)
		}
	}

	return errs
}

func duplicateNodeGlobErrors(g *graph.Graph, cfgPath string) []port.Violation {
	byGlob := map[string][]string{}
	for id, glob := range g.Nodes {
		byGlob[glob] = append(byGlob[glob], id)
	}

	var globs []string
	for glob, ids := range byGlob {
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		globs = append(globs, glob)
	}
	sort.Strings(globs)

	err := make([]port.Violation, 0, len(globs))
	for _, glob := range globs {
		ids := byGlob[glob]
		line := g.NodeLines[ids[1]]
		err = append(err, makeDuplicateNodeGlobError(cfgPath, line, glob, ids))
	}
	return err
}

func invalidNodeGlobErrors(g *graph.Graph, cfgPath string) []port.Violation {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var errs []port.Violation
	for _, id := range ids {
		glob := g.Nodes[id]
		for _, msg := range graph.ValidateNodeGlob(glob) {
			errs = append(errs, makeInvalidNodeGlobError(id, cfgPath, g.NodeLines[id], glob, msg))
		}
	}
	return errs
}

func contractOverlapErrors(fsys port.FileSystem, lang port.Language, g *graph.Graph, cfgPath string) []port.Violation {
	if fsys == nil || lang == nil {
		return nil
	}

	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var candidatePairs [][2]string
	for i := 0; i < len(ids); i++ {
		aGlob := g.Nodes[ids[i]]
		if graph.IsFileGlob(aGlob) {
			continue
		}
		for j := i + 1; j < len(ids); j++ {
			bGlob := g.Nodes[ids[j]]
			if aGlob == bGlob || graph.IsFileGlob(bGlob) {
				continue
			}
			if !graph.GlobsOverlap(aGlob, bGlob) {
				continue
			}
			candidatePairs = append(candidatePairs, [2]string{ids[i], ids[j]})
		}
	}
	if len(candidatePairs) == 0 {
		return nil
	}

	dirKeys := collectDirKeys(fsys, lang, filepath.Dir(cfgPath))
	if len(dirKeys) == 0 {
		return nil
	}

	type overlapResult struct {
		witness string
		i       string
		j       string
	}
	overlapChan := make(chan overlapResult, len(candidatePairs))
	numWorkers := min(runtime.NumCPU(), len(candidatePairs))
	if numWorkers < 1 {
		numWorkers = 1
	}

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, pair := range candidatePairs {
				a, b := pair[0], pair[1]
				if witness := findWitnessInDirs(g.Nodes[a], g.Nodes[b], dirKeys); witness != "" {
					overlapChan <- overlapResult{witness: witness, i: a, j: b}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(overlapChan)
	}()

	var errs []port.Violation
	for r := range overlapChan {
		errs = append(errs, makeOverlapError(r.i, r.j, cfgPath, g.NodeLines[r.i], g.NodeLines[r.j], r.witness))
	}
	sort.Slice(errs, func(i, j int) bool {
		return errs[i].Message < errs[j].Message
	})
	return errs
}

func collectDirKeys(fsys port.FileSystem, lang port.Language, baseDir string) []string {
	var keys []string
	_ = service.WalkAllFiles(fsys, baseDir, lang, func(abs, rel string) error {
		keys = append(keys, rel)
		return nil
	})
	return keys
}

func findWitnessInDirs(aGlob, bGlob string, fileRels []string) string {
	seen := make(map[string]struct{})
	for _, rel := range fileRels {
		key := graph.NodeKeyForDir(rel)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if graph.MatchDirGlob(aGlob, key) && graph.MatchDirGlob(bGlob, key) {
			return rel
		}
	}
	return ""
}

func makeEmptyNodeGlobError(contractPath string, line int, id string) port.Violation {
	return makeContractViolation("empty-node-glob", contractPath, line, fmt.Sprintf("node %q has empty glob", id))
}

func makeUndefinedEdgeNodeError(contractPath string, line int, id string) port.Violation {
	return makeContractViolation("undefined-edge-node", contractPath, line, fmt.Sprintf("edge references undefined node %q", id))
}

func makeCycleError(contractPath string, line int, cycleStr string) port.Violation {
	return makeContractViolation("circular-dependency", contractPath, line, "circular dependency: "+cycleStr)
}

func makeContractViolation(rule, contractPath string, line int, msg string) port.Violation {
	v := port.Violation{
		Rule:     rule,
		Severity: "error",
		Source:   "baft",
		File:     contractPath,
	}
	if line > 0 {
		v.Line = line
		v.Message = fmt.Sprintf("%s (%s:%d)", msg, contractPath, line)
		return v
	}
	v.Message = fmt.Sprintf("%s (%s)", msg, contractPath)
	return v
}
