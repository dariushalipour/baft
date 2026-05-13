// Package check verifies that code imports respect architecture rules
// declared in BAFT.md contract files across one or more capsules.
//
// Algorithm:
//
//  1. Discover capsules (modules with manifest files like go.mod)
//  2. For each capsule, find the root BAFT.md and any scoped contracts
//  3. Walk every scannable file, resolve its imports to internal targets
//  4. For each import, determine the tracking scope and check the
//     relation against the appropriate graph, walking up ancestor scopes
//     when source and target are in different scopes
//  5. Validate that all graphs use only node types supported by the language
//  6. Aggregate violations and errors into a CheckResult
package check

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/dariushalipour/baft/internal/adapter/fs/ignorefs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

type capsuleResult struct {
	graph                 *graph.Graph
	filesEncountered      int
	filesScanned          int
	relations             int
	violations            []port.Violation
	errors                []port.Violation
	hasOverlapError       bool
	hasDuplicateGlobError bool
	hasInvalidGlobError   bool
}

func (r *capsuleResult) toPublic(label string) port.CapsuleResult {
	cr := port.CapsuleResult{
		Label:            label,
		FilesEncountered: r.filesEncountered,
		FilesScanned:     r.filesScanned,
		Relations:        r.relations,
		Violations:       r.violations,
		Errors:           r.errors,
	}
	if r.graph != nil {
		cr.Nodes = len(r.graph.Nodes)
		cr.Edges = r.graph.EdgeCount()
	}
	return cr
}

type contractContext struct {
	rootGraph       *graph.Graph
	hasRootContract bool
	contractPathAbs string
	loadErr         []port.Violation
}

type capsuleChecker struct {
	res               *capsuleResult
	fsys              port.FileSystem
	capsule           port.Capsule
	lang              port.Language
	contractDir       string
	contractDirAbs    string
	scopeCache        *scopeCache
	parseCache        *parseCache
	nestedCapsuleDirs []string
	contractContext
}

func newCapsuleChecker(
	fsys port.FileSystem,
	capsule port.Capsule,
	lang port.Language,
	repo port.GraphRepository,
	contractDir string,
	ctx contractContext,
	nestedCapsuleDirs []string,
) *capsuleChecker {
	contractDirAbs, _ := filepath.Abs(contractDir)
	return &capsuleChecker{
		res:               &capsuleResult{graph: ctx.rootGraph},
		fsys:              fsys,
		capsule:           capsule,
		lang:              lang,
		contractDir:       contractDir,
		contractDirAbs:    contractDirAbs,
		scopeCache:        newScopeCache(fsys, repo),
		parseCache:        newParseCache(),
		nestedCapsuleDirs: nestedCapsuleDirs,
		contractContext:   ctx,
	}
}

type capsuleEntry struct {
	capsule port.Capsule
	lang    port.Language
}

type capsuleWork struct {
	index      int
	capsule    port.Capsule
	lang       port.Language
	label      string
	nestedDirs []string
}

type capsuleResultItem struct {
	index  int
	label  string
	result *capsuleResult
	err    error
}

func Run(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery) *port.CheckResult {
	return RunWithContext(context.Background(), fsys, rootDir, languages, repo, discovery)
}

func RunWithContext(ctx context.Context, fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery) *port.CheckResult {
	// Wrap the filesystem with ignore rules before discovery.
	wrapped, err := ignorefs.Wrap(fsys, ignorefs.Options{
		RootDir:           rootDir,
		BaseIgnoreEntries: discovery.BaseIgnoreEntries(),
	})
	var warnings []string
	if err != nil {
		if !errors.Is(err, ignorefs.ErrRepoRootUnreachable) {
			return &port.CheckResult{Errors: []string{"ignorefs: " + err.Error()}}
		}
		warnings = []string{"not inside a git repository — .gitignore/.baftignore rules from parent directories will not apply"}
	}

	entries, err := discovery.Discover(wrapped, rootDir)
	if err != nil {
		return &port.CheckResult{Errors: []string{"discovery: " + err.Error()}}
	}

	capsules := matchCapsulesToLanguages(entries, languages)
	if len(capsules) == 0 {
		return &port.CheckResult{Warnings: warnings}
	}

	sort.Slice(capsules, func(i, j int) bool {
		return port.Label(capsules[i].capsule) < port.Label(capsules[j].capsule)
	})

	// Build work items with their original indices for deterministic output ordering
	workItems := make([]capsuleWork, 0, len(capsules))
	for i, c := range capsules {
		label := port.Label(c.capsule)
		nestedDirs := nestedCapsuleDirs(capsules, c.capsule.Dir)
		workItems = append(workItems, capsuleWork{
			index:      i,
			capsule:    c.capsule,
			lang:       c.lang,
			label:      label,
			nestedDirs: nestedDirs,
		})
	}

	// Use fan-out pattern: distribute work items via a channel to a pool of workers,
	// collect via result channel. The book recommends worker pools for independent
	// tasks to bound concurrency and avoid overwhelming the system.
	numWorkers := min(runtime.NumCPU(), len(workItems))
	workChan := make(chan capsuleWork, numWorkers*2)
	results := make(chan capsuleResultItem, numWorkers*2)

	go func() {
		for _, w := range workItems {
			workChan <- w
		}
		close(workChan)
	}()

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for work := range workChan {
				capsuleRes, err := checkCapsule(ctx, wrapped, work.capsule, work.lang, repo, rootDir, work.nestedDirs)
				results <- capsuleResultItem{
					index:  work.index,
					label:  work.label,
					result: capsuleRes,
					err:    err,
				}
			}
		}()
	}

	// Close channel once all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all results
	collected := make([]capsuleResultItem, 0, len(workItems))
	for r := range results {
		collected = append(collected, r)
	}

	// Build result in original sorted order for deterministic output
	sort.Slice(collected, func(i, j int) bool {
		return collected[i].index < collected[j].index
	})

	var result port.CheckResult
	// Pre-allocate slices with estimated capacity.
	result.Warnings = warnings
	result.Errors = make([]string, 0, len(collected)*2)
	result.Capsules = make([]port.CapsuleResult, 0, len(collected))
	result.Violations = make([]string, 0, len(collected)*5)
	seenContractErrors := map[string]bool{}
	for _, r := range collected {
		if r.err != nil {
			result.Errors = append(result.Errors, r.label+": "+r.err.Error())
			result.Capsules = append(result.Capsules, port.CapsuleResult{Label: r.label})
			continue
		}
		if r.result == nil {
			continue
		}
		r.result.errors = dedupeContractErrors(r.result.errors, seenContractErrors)
		result.Capsules = append(result.Capsules, r.result.toPublic(r.label))
		for _, v := range r.result.violations {
			result.Violations = append(result.Violations, r.label+": "+v.Message)
		}
		for _, e := range r.result.errors {
			result.Errors = append(result.Errors, r.label+": "+e.Message)
		}
	}
	return &result
}

func dedupeContractErrors(errors []port.Violation, seen map[string]bool) []port.Violation {
	filtered := errors[:0]
	for _, err := range errors {
		key := contractErrorKey(err)
		if key != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		filtered = append(filtered, err)
	}
	return filtered
}

func contractErrorKey(err port.Violation) string {
	if filepath.Base(err.File) != port.ContractFile {
		return ""
	}
	return err.File + "\x00" + err.Message
}

func matchCapsulesToLanguages(entries []service.CapsuleEntry, languages []port.Language) []capsuleEntry {
	langMap := make(map[string]port.Language, len(languages))
	for _, lang := range languages {
		langMap[lang.Name()] = lang
	}
	var capsules []capsuleEntry
	for _, e := range entries {
		if lang := langMap[e.LangName]; lang != nil {
			capsules = append(capsules, capsuleEntry{capsule: e.Capsule, lang: lang})
		}
	}
	return capsules
}

func nestedCapsuleDirs(capsules []capsuleEntry, capsuleDir string) []string {
	prefix := capsuleDir + string(filepath.Separator)
	var nested []string
	for _, c := range capsules {
		if strings.HasPrefix(c.capsule.Dir, prefix) {
			nested = append(nested, c.capsule.Dir)
		}
	}
	return nested
}

func checkCapsule(ctx context.Context, fsys port.FileSystem, capsule port.Capsule, lang port.Language, repo port.GraphRepository, contractDir string, nestedDirs []string) (*capsuleResult, error) {
	ctrCtx, err := loadCapsuleContract(fsys, repo, contractDir, capsule.Dir)
	if err != nil {
		return nil, err
	}
	if len(ctrCtx.loadErr) > 0 {
		return &capsuleResult{errors: ctrCtx.loadErr}, nil
	}
	if !ctrCtx.hasRootContract && !hasScopedContract(fsys, capsule.Dir) {
		return nil, nil
	}
	chk := newCapsuleChecker(fsys, capsule, lang, repo, contractDir, ctrCtx, nestedDirs)
	if err := chk.walk(ctx, fsys, capsule.Dir); err != nil {
		return nil, err
	}
	chk.validateAll()
	if chk.res.hasOverlapError || chk.res.hasDuplicateGlobError {
		chk.res.filesEncountered = 0
		chk.res.filesScanned = 0
		chk.res.relations = 0
		chk.res.violations = nil
	}
	if chk.res.hasInvalidGlobError {
		chk.res.violations = nil
	}
	sort.Slice(chk.res.errors, func(i, j int) bool {
		return chk.res.errors[i].Message < chk.res.errors[j].Message
	})
	sort.Slice(chk.res.violations, func(i, j int) bool {
		return chk.res.violations[i].Message < chk.res.violations[j].Message
	})
	return chk.res, nil
}

func makeContractLoadErrors(contractPath string, err error) []port.Violation {
	var violations []port.Violation

	// Check if it's a chain of ParseErrorWithNext (from multiple validation errors).
	var chain *mermaid.ParseErrorWithNext
	if errors.As(err, &chain) {
		for cur := chain; cur != nil; {
			v := port.Violation{
				Rule:     "contract-load-error",
				Severity: "error",
				Source:   "baft",
				File:     contractPath,
			}
			if cur.Raw != "" {
				v.Message = fmt.Sprintf("unrecognized mermaid line: %s (%s:%d)", strings.TrimSpace(cur.Raw), contractPath, cur.Line)
			} else if cur.Line > 0 {
				v.Message = fmt.Sprintf("%s (%s:%d)", cur.Msg, contractPath, cur.Line)
			} else {
				v.Message = fmt.Sprintf("%s (%s)", cur.Msg, contractPath)
			}
			if cur.Line > 0 {
				v.Line = cur.Line
			}
			violations = append(violations, v)
			if cur.Next == nil {
				break
			}
			var nextChain *mermaid.ParseErrorWithNext
			if errors.As(cur.Next, &nextChain) {
				cur = nextChain
			} else {
				break
			}
		}
		return violations
	}

	// Single ParseError.
	var pe *mermaid.ParseError
	if errors.As(err, &pe) {
		v := port.Violation{
			Rule:     "contract-load-error",
			Severity: "error",
			Source:   "baft",
			File:     contractPath,
		}
		if pe.Raw != "" {
			v.Message = fmt.Sprintf("unrecognized mermaid line: %s (%s:%d)", strings.TrimSpace(pe.Raw), contractPath, pe.Line)
		} else if pe.Line > 0 {
			v.Message = fmt.Sprintf("%s (%s:%d)", pe.Msg, contractPath, pe.Line)
		} else {
			v.Message = fmt.Sprintf("%s (%s)", pe.Msg, contractPath)
		}
		if pe.Line > 0 {
			v.Line = pe.Line
		}
		return []port.Violation{v}
	}

	v := port.Violation{
		Rule:     "contract-load-error",
		Severity: "error",
		Source:   "baft",
		File:     contractPath,
	}
	v.Message = err.Error()
	if !strings.Contains(v.Message, contractPath) {
		v.Message = fmt.Sprintf("%s (%s)", v.Message, contractPath)
	}
	return []port.Violation{v}
}

func loadCapsuleContract(fsys port.FileSystem, repo port.GraphRepository, contractDir, capsuleDir string) (contractContext, error) {
	var ctx contractContext
	contractPath := service.FindContract(fsys, contractDir, capsuleDir)
	ctx.contractPathAbs, _ = filepath.Abs(contractPath)
	raw, err := fsys.ReadFile(contractPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ctx, nil
		}
		ctx.loadErr = makeContractLoadErrors(ctx.contractPathAbs, err)
		return ctx, nil
	}
	ctx.hasRootContract = true
	ctx.rootGraph, err = repo.Load(string(raw))
	if err != nil {
		ctx.loadErr = makeContractLoadErrors(ctx.contractPathAbs, err)
		return ctx, nil
	}
	return ctx, nil
}
