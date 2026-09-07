// Package check verifies that code imports respect architecture rules
// declared in contract files across one or more capsules.
//
// Algorithm:
//
//  1. Discover capsules (modules with manifest files like go.mod)
//  2. For each capsule, find the root contract file and any scoped contracts
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
	"sort"
	"strings"
	"sync"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

type capsuleResult struct {
	graphIndex            *graph.GraphIndex
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
	if r.graphIndex != nil {
		g := r.graphIndex.Graph()
		cr.Nodes = len(g.Nodes)
		cr.Edges = r.graphIndex.EdgeCount()
	}
	return cr
}

type contractContext struct {
	rootGraphIndex  *graph.GraphIndex
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
	strategyFactory   *StrategyFactory
	strategy          ResolutionStrategy
	scopeMemo         sync.Map // dir -> tracking scope
	visibleMemo       sync.Map // abs -> target visibility
	files             []fileWork
	scoped            bool
	contractContext
}

// trackingScope memoizes service.TrackingScope per directory: every file and
// import target in a directory shares the same answer, which costs a stat
// chain to compute.
func (ch *capsuleChecker) trackingScope(abs string) string {
	dir := filepath.Dir(abs)
	if v, ok := ch.scopeMemo.Load(dir); ok {
		return v.(string)
	}
	scope := service.TrackingScope(ch.fsys, abs, ch.capsule.Dir)
	ch.scopeMemo.Store(dir, scope)
	return scope
}

// targetVisible memoizes port.IsTargetVisible, which stats and reads
// directories and is asked about the same targets by every importer.
func (ch *capsuleChecker) targetVisible(abs string) bool {
	if v, ok := ch.visibleMemo.Load(abs); ok {
		return v.(bool)
	}
	visible := port.IsTargetVisible(ch.fsys, abs)
	ch.visibleMemo.Store(abs, visible)
	return visible
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
		res:               &capsuleResult{graphIndex: ctx.rootGraphIndex},
		fsys:              fsys,
		capsule:           capsule,
		lang:              lang,
		contractDir:       contractDir,
		contractDirAbs:    contractDirAbs,
		strategyFactory:   NewStrategyFactory(fsys, lang),
		strategy:          &pathResolutionStrategy{},
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

// RunCapsule checks a single capsule against its contracts. It skips capsule
// discovery and the walk of unrelated capsules, so callers that already know
// which capsule they care about pay for that capsule only. fsys must already
// carry the caller's ignore rules; nestedDirs lists capsule dirs nested inside
// this one.
func RunCapsule(fsys port.FileSystem, rootDir string, capsule port.Capsule, lang port.Language, repo port.GraphRepository, nestedDirs []string) (*port.CapsuleResult, error) {
	res, err := checkCapsule(context.Background(), fsys, capsule, lang, repo, rootDir, nestedDirs)
	if err != nil || res == nil {
		return nil, err
	}
	public := res.toPublic(port.Label(capsule))
	return &public, nil
}

func RunWithContext(ctx context.Context, fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery) *port.CheckResult {
	entries, err := discovery.Discover(ctx, fsys, rootDir)
	if err != nil {
		return &port.CheckResult{Errors: []string{"discovery: " + err.Error()}}
	}

	capsules := matchCapsulesToLanguages(entries, languages)
	if len(capsules) == 0 {
		return &port.CheckResult{}
	}

	sort.Slice(capsules, func(i, j int) bool {
		return port.Label(capsules[i].capsule) < port.Label(capsules[j].capsule)
	})

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

	results := runParallel(ctx, workItems, func(in <-chan capsuleWork, emit func(capsuleResultItem) bool) {
		for work := range in {
			capsuleRes, err := checkCapsule(ctx, fsys, work.capsule, work.lang, repo, rootDir, work.nestedDirs)
			if !emit(capsuleResultItem{index: work.index, label: work.label, result: capsuleRes, err: err}) {
				return
			}
		}
	})

	collected := make([]capsuleResultItem, 0, len(workItems))
	for r := range results {
		collected = append(collected, r)
	}

	sort.Slice(collected, func(i, j int) bool {
		return collected[i].index < collected[j].index
	})

	var result port.CheckResult
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
			msg := r.label + ": " + v.Message
			if v.Severity == "warning" {
				result.Warnings = append(result.Warnings, msg)
			} else {
				result.Violations = append(result.Violations, msg)
			}
		}
		for _, e := range r.result.errors {
			result.Errors = append(result.Errors, r.label+": "+e.Message)
		}
	}
	if err := ctx.Err(); err != nil {
		return &port.CheckResult{Errors: []string{err.Error()}}
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
	if len(ctrCtx.loadErr) > 0 && ctrCtx.rootGraphIndex == nil {
		return &capsuleResult{errors: ctrCtx.loadErr}, nil
	}
	chk := newCapsuleChecker(fsys, capsule, lang, repo, contractDir, ctrCtx, nestedDirs)
	if err := chk.walk(ctx, fsys, capsule.Dir); err != nil {
		return nil, err
	}
	if !ctrCtx.hasRootContract && !chk.scoped {
		return nil, nil
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
	chk.res.errors = append(chk.res.errors, ctrCtx.loadErr...)
	sort.Slice(chk.res.errors, func(i, j int) bool {
		return chk.res.errors[i].Message < chk.res.errors[j].Message
	})
	sort.Slice(chk.res.violations, func(i, j int) bool {
		return chk.res.violations[i].Message < chk.res.violations[j].Message
	})
	return chk.res, nil
}

func makeContractLoadErrors(contractPath string, err error) []port.Violation {
	v := port.Violation{
		Rule:     "contract-load-error",
		Severity: "error",
		Source:   "baft",
		File:     contractPath,
		Message:  err.Error(),
	}
	var pe *port.ParseError
	if errors.As(err, &pe) {
		located := *pe
		located.File = contractPath
		v.Message, v.Line = located.Error(), pe.Line
	} else if !strings.Contains(v.Message, contractPath) {
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
	g, loadErr := repo.Load(string(raw))
	if loadErr != nil {
		ctx.loadErr = makeContractLoadErrors(ctx.contractPathAbs, loadErr)
	} else {
		ctx.rootGraphIndex = graph.NewGraphIndex(g)
	}
	return ctx, nil
}
