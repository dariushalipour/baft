package check

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

type fileWork struct {
	abs      string
	rel      string
	scopeDir string
}

// targetInfo holds information about a resolved import target.
type targetInfo struct {
	abs        string
	inScopeKey string
	targetRel  string
}

// CrossScopeContext encapsulates the parameters required for cross-scope resolution,
// preventing bloated method signatures in the ResolutionStrategy interface.
type CrossScopeContext struct {
	SrcAbs          string
	FileRel         string
	Spec            port.ImportSpec
	Src             string
	TargetAbs       string
	TargetRel       string
	CfgPath         string
	ScopeGraph      *graph.GraphIndex
	ScopeDir        string
	Fsys            port.FileSystem
	Capsule         *port.Capsule
	HasRootContract bool
	RootGraphIndex  *graph.GraphIndex
	ScopeCache      *scopeCache
	ContractPathAbs string
}

// ResolutionStrategy abstracts node resolution between namespace mode and
// non-namespace mode, eliminating branching logic from the capsule checker.
type ResolutionStrategy interface {
	// ResolveSourceNode resolves the source node ID for a given file.
	ResolveSourceNode(abs, scopeDir string, scopeGraph *graph.GraphIndex) (nodeID string)
	// ResolveImport resolves the list of target files and their metadata for an import spec.
	ResolveImport(spec port.ImportSpec, fileRel string, scopeDir string, fsys port.FileSystem, capsule *port.Capsule, lang port.Language) ([]targetInfo, error)
	// ResolveNode resolves a node ID from a file absolute path and its
	// associated namespace/key within a specific graph.
	ResolveNode(fileAbs, targetKey string, scopeGraph *graph.GraphIndex) string
	// ResolveCrossScope handles cross-scope violation checks.
	ResolveCrossScope(ctx *CrossScopeContext) []port.Violation
	// IsFileGlob reports whether the pattern is a file-shaped glob in the
	// current resolution mode.
	IsFileGlob(pattern string) bool
	// ShouldReportNoNodeViolation returns true if a missing node for a file
	// should be reported as a violation. In namespace mode, files without a
	// namespace are silently skipped. In path mode, all files must match a node.
	ShouldReportNoNodeViolation(abs string) bool
	// ShouldFailOnInvalidGlob returns true if a file-shaped node pattern
	// should immediately fail the check. In namespace mode, file-shaped nodes
	// are fundamentally incompatible with resolution, so the check fails.
	// In path mode, the error is reported alongside any violations.
	ShouldFailOnInvalidGlob() bool
}

// StrategyFactory produces the appropriate ResolutionStrategy for a given set of
// files and a root graph. It encapsulates the namespace indexing logic, so the
// capsuleChecker remains completely agnostic of resolution mode.
type StrategyFactory struct {
	fsys port.FileSystem
	lang port.Language
}

func NewStrategyFactory(fsys port.FileSystem, lang port.Language) *StrategyFactory {
	return &StrategyFactory{fsys: fsys, lang: lang}
}

// BuildStrategyFromGraph returns a ResolutionStrategy based solely on the graph's
// NamespaceMode flag, without building file indexes. It is intended for validation
// passes that only need IsFileGlob and ShouldFailOnInvalidGlob.
func (sf *StrategyFactory) BuildStrategyFromGraph(g *graph.Graph) ResolutionStrategy {
	if g != nil && g.NamespaceMode {
		return &namespaceResolutionStrategy{
			fileNSMap: make(map[string]string),
			nsMap:     make(map[string][]string),
		}
	}
	return &pathResolutionStrategy{}
}

// BuildStrategy returns a ResolutionStrategy for the given files and root graph,
// building namespace indexes when namespace mode is enabled. The second return
// value reports namespace mode with no namespace declared by any scanned file;
// the caller surfaces that as a diagnostic rather than falling back to path mode.
func (sf *StrategyFactory) BuildStrategy(files []fileWork, rootGraph *graph.Graph) (ResolutionStrategy, bool) {
	if rootGraph == nil || !rootGraph.NamespaceMode {
		return &pathResolutionStrategy{}, false
	}

	fileNSMap := make(map[string]string, len(files))
	nsMap := make(map[string][]string)
	for _, fw := range files {
		ns, err := sf.lang.GetFileNamespace(sf.fsys, fw.abs)
		if err != nil || ns == "" {
			continue
		}
		fileNSMap[fw.abs] = ns
		nsMap[ns] = append(nsMap[ns], fw.abs)
	}

	return &namespaceResolutionStrategy{fileNSMap: fileNSMap, nsMap: nsMap}, len(nsMap) == 0
}

type pathResolutionStrategy struct{}

func (s *pathResolutionStrategy) ResolveSourceNode(abs, scopeDir string, scopeGraph *graph.GraphIndex) string {
	scopeRel := relToSlash(scopeDir, abs)
	return scopeGraph.NodeForPath(scopeRel)
}

func (s *pathResolutionStrategy) ResolveImport(spec port.ImportSpec, fileRel string, scopeDir string, fsys port.FileSystem, capsule *port.Capsule, lang port.Language) ([]targetInfo, error) {
	targetPath, internal := lang.ResolveInternalTarget(fsys, spec, *capsule, fileRel)
	if !internal {
		return nil, nil
	}
	targetAbs := absPath(capsule.Dir, targetPath)
	return []targetInfo{{
		abs:        targetAbs,
		inScopeKey: relToSlash(scopeDir, targetAbs),
		targetRel:  relToSlash(capsule.Dir, targetAbs),
	}}, nil
}

func (s *pathResolutionStrategy) ResolveNode(fileAbs, targetKey string, scopeGraph *graph.GraphIndex) string {
	return scopeGraph.NodeForPath(targetKey)
}

// resolveCrossScope walks the tiers that may know about both ends of a
// cross-scope import — the source scope, then its ancestor contracts, then the
// capsule root — and reports the first tier that resolves both endpoints.
// accept filters ancestor and root graphs by mode; nodes resolves the endpoints
// in a tier's graph, given the directory that tier's globs are relative to.
func resolveCrossScope(
	ctx *CrossScopeContext,
	targetRel string,
	accept func(*graph.Graph) bool,
	nodes func(g *graph.GraphIndex, dir string) (src, dst string),
) []port.Violation {
	tier := func(g *graph.GraphIndex, dir, contractPath string, srcFallback string) ([]port.Violation, bool) {
		src, dst := nodes(g, dir)
		if src == "" {
			src = srcFallback
		}
		if src == "" || dst == "" {
			return nil, false
		}
		return checkRelation(g.Graph(), ctx.SrcAbs, ctx.FileRel, ctx.Spec, src, targetRel, dst, contractPath), true
	}

	if v, resolved := tier(ctx.ScopeGraph, ctx.ScopeDir, ctx.CfgPath, ctx.Src); resolved {
		return v
	}
	for _, anc := range ancestorContracts(ctx.Fsys, ctx.ScopeDir, ctx.Capsule.Dir, ctx.ScopeCache) {
		if !accept(anc.graphIndex.Graph()) {
			continue
		}
		if v, resolved := tier(anc.graphIndex, anc.dir, anc.contractPath, ""); resolved {
			return v
		}
	}
	if ctx.HasRootContract && ctx.RootGraphIndex != nil && accept(ctx.RootGraphIndex.Graph()) {
		if v, _ := tier(ctx.RootGraphIndex, ctx.Capsule.Dir, ctx.ContractPathAbs, ""); v != nil {
			return v
		}
	}
	return nil
}

func (s *pathResolutionStrategy) ResolveCrossScope(ctx *CrossScopeContext) []port.Violation {
	return resolveCrossScope(ctx, ctx.TargetRel,
		func(*graph.Graph) bool { return true },
		func(g *graph.GraphIndex, dir string) (string, string) {
			dstRel := relToSlash(dir, ctx.TargetAbs)
			if dir == ctx.ScopeDir && escapesScope(dstRel) {
				return "", ""
			}
			return g.NodeForPath(relToSlash(dir, ctx.SrcAbs)), g.NodeForPath(dstRel)
		})
}

func (s *pathResolutionStrategy) IsFileGlob(p string) bool { return graph.IsFileGlob(p) }

func (s *pathResolutionStrategy) ShouldReportNoNodeViolation(abs string) bool {
	return true
}

func (s *pathResolutionStrategy) ShouldFailOnInvalidGlob() bool { return false }

type namespaceResolutionStrategy struct {
	fileNSMap map[string]string
	nsMap     map[string][]string
}

func (s *namespaceResolutionStrategy) ResolveSourceNode(abs, scopeDir string, scopeGraph *graph.GraphIndex) string {
	fileNS, ok := s.fileNSMap[abs]
	if !ok || fileNS == "" {
		return ""
	}
	return scopeGraph.NodeForNamespace(fileNS)
}

func (s *namespaceResolutionStrategy) ResolveImport(spec port.ImportSpec, fileRel string, scopeDir string, fsys port.FileSystem, capsule *port.Capsule, lang port.Language) ([]targetInfo, error) {
	targetAbsList, ok := s.nsMap[spec.Namespace]
	if !ok {
		return nil, nil
	}
	infos := make([]targetInfo, 0, len(targetAbsList))
	for _, abs := range targetAbsList {
		inScopeKey := spec.Namespace
		if ns, ok := s.fileNSMap[abs]; ok && ns != "" {
			inScopeKey = ns
		}
		infos = append(infos, targetInfo{
			abs:        abs,
			inScopeKey: inScopeKey,
			targetRel:  spec.Namespace,
		})
	}
	return infos, nil
}

func (s *namespaceResolutionStrategy) ResolveNode(fileAbs, targetKey string, scopeGraph *graph.GraphIndex) string {
	return scopeGraph.NodeForNamespace(targetKey)
}

func (s *namespaceResolutionStrategy) ResolveCrossScope(ctx *CrossScopeContext) []port.Violation {
	srcNS := s.namespaceOf(ctx.SrcAbs, ctx.Src)
	targetNS := s.namespaceOf(ctx.TargetAbs, ctx.Spec.Namespace)
	return resolveCrossScope(ctx, targetNS,
		func(g *graph.Graph) bool { return g.NamespaceMode },
		func(g *graph.GraphIndex, _ string) (string, string) {
			return g.NodeForNamespace(srcNS), g.NodeForNamespace(targetNS)
		})
}

func (s *namespaceResolutionStrategy) namespaceOf(abs, fallback string) string {
	if ns := s.fileNSMap[abs]; ns != "" {
		return ns
	}
	return fallback
}

// IsFileGlob treats only a path-shaped pattern as file-shaped: a namespace
// wildcard such as "MyApp.Api.*" is matched by the graph index.
func (s *namespaceResolutionStrategy) IsFileGlob(pattern string) bool {
	return strings.Contains(pattern, "/")
}

func (s *namespaceResolutionStrategy) ShouldReportNoNodeViolation(abs string) bool {
	return s.fileNSMap[abs] != ""
}

func (s *namespaceResolutionStrategy) ShouldFailOnInvalidGlob() bool { return true }

func (ch *capsuleChecker) walk(ctx context.Context, capsuleDir string) error {
	contractDirSep := ch.contractDirAbs + string(filepath.Separator)
	var nestedSep []string
	for _, nested := range ch.nestedCapsuleDirs {
		nestedSep = append(nestedSep, nested+string(filepath.Separator))
	}

	var filesToCheck []fileWork

	err := service.WalkAllFiles(ctx, ch.fsys, capsuleDir, ch.lang, func(abs, rel string) error {
		ch.walked = append(ch.walked, abs)
		if abs != ch.contractDirAbs && !strings.HasPrefix(abs, contractDirSep) {
			return nil
		}
		for _, nsep := range nestedSep {
			if strings.HasPrefix(abs, nsep) {
				if err := ctx.Err(); err != nil {
					return err
				}
				return nil
			}

		}
		scopeDir := ch.trackingScope(abs)
		if scopeDir != ch.capsule.Dir {
			ch.scoped = true
		}
		filesToCheck = append(filesToCheck, fileWork{abs: abs, rel: rel, scopeDir: scopeDir})
		return nil
	})
	if err != nil {
		return err
	}

	if len(filesToCheck) == 0 || (!ch.hasRootContract && !ch.scoped) {
		return nil
	}

	// Build resolution strategy via factory, which handles namespace indexing
	// internally when appropriate.
	var rootGraph *graph.Graph
	if ch.rootGraphIndex != nil {
		rootGraph = ch.rootGraphIndex.Graph()
	}
	strategy, noNamespaces := ch.strategyFactory.BuildStrategy(filesToCheck, rootGraph)
	ch.strategy = strategy
	if noNamespaces {
		ch.res.errors = append(ch.res.errors, makeNamespaceModeNoNamespacesError(ch.contractPathAbs))
	}

	results := runParallel(ctx, filesToCheck, func(in <-chan fileWork, emit func(fileCheckResult) bool) {
		var acc fileCheckResult
		for fw := range in {
			res := ch.checkFileResult(fw.abs, fw.rel, fw.scopeDir)
			if res.err != nil {
				emit(res)
				return
			}
			acc.merge(res)
		}
		emit(acc)
	})

	for res := range results {
		if res.err != nil {
			return res.err
		}
		ch.mergeFileResult(res)
	}

	return nil
}

type fileCheckResult struct {
	filesEncountered int
	filesScanned     int
	relations        int
	violations       []port.Violation
	err              error
}

func (r *fileCheckResult) merge(o fileCheckResult) {
	r.filesEncountered += o.filesEncountered
	r.filesScanned += o.filesScanned
	r.relations += o.relations
	r.violations = append(r.violations, o.violations...)
}

func (ch *capsuleChecker) checkFileResult(abs, fileRel string, scopeDir string) fileCheckResult {
	cfgPath, scopeGraph := ch.resolveScope(scopeDir)
	if scopeGraph == nil {
		return fileCheckResult{}
	}

	filesEncountered := 1

	src := ch.strategy.ResolveSourceNode(abs, scopeDir, scopeGraph)
	if src == "" {
		if !ch.strategy.ShouldReportNoNodeViolation(abs) {
			return fileCheckResult{filesEncountered: filesEncountered}
		}
		return fileCheckResult{filesEncountered: filesEncountered, violations: ch.handleNoNodeResult(abs, fileRel, scopeDir, cfgPath)}
	}

	imports, err := ch.parseCache.loadOrParse(ch, abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return fileCheckResult{err: err}
		}
		return fileCheckResult{filesEncountered: filesEncountered}
	}

	filesScanned := 1
	var relations int
	violations := make([]port.Violation, 0, len(imports))
	for _, spec := range imports {
		scopeRel := relToSlash(scopeDir, abs)
		r, v := ch.checkImportResult(spec, abs, fileRel, scopeRel, cfgPath, scopeGraph, src, scopeDir)
		relations += r
		if len(v) > 0 {
			violations = append(violations, v...)
		}
	}
	return fileCheckResult{
		filesEncountered: filesEncountered,
		filesScanned:     filesScanned,
		relations:        relations,
		violations:       violations,
	}
}

func (ch *capsuleChecker) mergeFileResult(res fileCheckResult) {
	ch.res.filesEncountered += res.filesEncountered
	ch.res.filesScanned += res.filesScanned
	ch.res.relations += res.relations
	ch.res.violations = append(ch.res.violations, res.violations...)
}

func (ch *capsuleChecker) handleNoNodeResult(abs, fileRel, scopeDir, cfgPath string) []port.Violation {
	scopeRel := ch.scopeRel(scopeDir, abs)
	noNode := makeNoNodeViolation(abs, scopeRel, cfgPath)
	imports, err := ch.parseCache.loadOrParse(ch, abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return []port.Violation{noNode}
		}
		return []port.Violation{noNode}
	}
	violations := make([]port.Violation, 0, 1+len(imports))
	violations = append(violations, noNode)
	for _, spec := range imports {
		targetPath, internal := ch.lang.ResolveInternalTarget(ch.fsys, spec, ch.capsule, fileRel)
		if !internal {
			continue
		}
		targetAbs := absPath(ch.capsule.Dir, targetPath)
		if !ch.targetVisible(targetAbs) {
			continue
		}
		violations = append(violations, makeImportNoNodeViolation(abs, scopeRel, spec, cfgPath))
	}
	return violations
}

func (ch *capsuleChecker) checkImportResult(spec port.ImportSpec, abs, fileRel, scopeRel, cfgPath string, scopeGraph *graph.GraphIndex, src, scopeDir string) (int, []port.Violation) {
	targets, err := ch.strategy.ResolveImport(spec, fileRel, scopeDir, ch.fsys, &ch.capsule, ch.lang)
	if err != nil {
		return 0, []port.Violation{{Message: err.Error(), Rule: "Internal Error"}}
	}
	if len(targets) == 0 {
		return 0, nil
	}

	var violations []port.Violation
	relations := 0
	seenInScope := false
	seenCrossScope := false

	for _, t := range targets {
		if !ch.targetVisible(t.abs) {
			continue
		}
		// One import is one relation, however many files share the namespace.
		relations = 1
		targetScope := ch.trackingScope(t.abs)
		if scopeDir == targetScope {
			if seenInScope {
				continue
			}
			seenInScope = true
			dst := ch.strategy.ResolveNode(t.abs, t.inScopeKey, scopeGraph)
			if dst == "" {
				violations = append(violations, makeImportNoNodeViolation(abs, scopeRel, spec, cfgPath))
			} else {
				g := scopeGraph.Graph()
				if dst == src {
					if g.IsEndophobic(src) {
						violations = append(violations, makeEndophobicViolation(abs, scopeRel, spec, t.inScopeKey, src, cfgPath))
					}
				} else {
					violations = append(violations, checkRelation(g, abs, scopeRel, spec, src, t.inScopeKey, dst, cfgPath)...)
				}
			}
		} else {
			if seenCrossScope {
				continue
			}
			seenCrossScope = true
			v := ch.strategy.ResolveCrossScope(&CrossScopeContext{
				SrcAbs:          abs,
				FileRel:         fileRel,
				Spec:            spec,
				Src:             src,
				TargetAbs:       t.abs,
				TargetRel:       t.targetRel,
				CfgPath:         cfgPath,
				ScopeGraph:      scopeGraph,
				ScopeDir:        scopeDir,
				Fsys:            ch.fsys,
				Capsule:         &ch.capsule,
				HasRootContract: ch.hasRootContract,
				RootGraphIndex:  ch.rootGraphIndex,
				ScopeCache:      ch.scopeCache,
				ContractPathAbs: ch.contractPathAbs,
			})
			if len(v) > 0 {
				violations = append(violations, v...)
			}
		}
	}

	return relations, violations
}

func (ch *capsuleChecker) scopeRel(scopeDir, abs string) string {
	return relToSlash(scopeDir, abs)
}

func escapesScope(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, "../")
}

func (ch *capsuleChecker) resolveScope(scopeDir string) (string, *graph.GraphIndex) {
	if scopeDir == ch.capsule.Dir {
		if !ch.hasRootContract {
			return "", nil
		}
		return ch.contractPathAbs, ch.rootGraphIndex
	}
	entry, err := ch.scopeCache.load(scopeDir)
	if err != nil {
		return "", nil
	}
	return entry.contractPath, entry.graphIndex
}

func (ch *capsuleChecker) validateAll() {
	if ch.hasRootContract && ch.rootGraphIndex != nil {
		g := ch.rootGraphIndex.Graph()
		ch.applyContractValidation(validateContractGraph(ch.contractPathAbs, g, ch.witnessKeys))
		ch.validateLanguageGraph(g, ch.contractPathAbs)
	}
	ch.scopeCache.iterate(func(entry *scopeEntry) {
		if len(entry.loadErr) > 0 {
			ch.res.errors = append(ch.res.errors, entry.loadErr...)
		}
		if entry.graphIndex != nil {
			g := entry.graphIndex.Graph()
			ch.applyContractValidation(validateContractGraph(entry.contractPath, g, ch.witnessKeys))
			ch.validateLanguageGraph(g, entry.contractPath)
		}
	})
}

// witnessKeys returns every file the walk saw as a path relative to the
// contract at cfgPath, the candidates for a node-overlap witness. It reuses the
// walk's own file list, so no extra tree traversal is needed.
func (ch *capsuleChecker) witnessKeys(cfgPath string) []string {
	baseDir := filepath.Dir(cfgPath)
	keys := make([]string, 0, len(ch.walked))
	for _, abs := range ch.walked {
		if rel := relToSlash(baseDir, abs); !escapesScope(rel) {
			keys = append(keys, rel)
		}
	}
	return keys
}

func (ch *capsuleChecker) applyContractValidation(result ContractValidationResult) {
	ch.res.errors = append(ch.res.errors, result.Errors...)
	if result.HasOverlapError {
		ch.res.hasOverlapError = true
	}
	if result.HasDuplicateGlobError {
		ch.res.hasDuplicateGlobError = true
	}
	if result.HasInvalidGlobError {
		ch.res.hasInvalidGlobError = true
	}
}

func (ch *capsuleChecker) validateLanguageGraph(g *graph.Graph, cfgPath string) {
	if !ch.lang.SupportsFileGlobs() {
		strat := ch.strategyFactory.BuildStrategyFromGraph(g)
		for id, glob := range g.Nodes {
			if strat.IsFileGlob(glob) {
				ch.res.errors = append(ch.res.errors, makeFileGlobUnsupportedError(id, cfgPath, g.NodeLines[id], glob))
				if strat.ShouldFailOnInvalidGlob() {
					ch.res.hasInvalidGlobError = true
				}
			}
		}
	}
}

type scopeCache struct {
	mu   sync.RWMutex
	m    map[string]*scopeEntry
	fsys port.FileSystem
	repo port.GraphRepository
}

type scopeEntry struct {
	graphIndex   *graph.GraphIndex
	contractPath string
	loadErr      []port.Violation
}

func newScopeCache(fsys port.FileSystem, repo port.GraphRepository) *scopeCache {
	return &scopeCache{m: make(map[string]*scopeEntry), fsys: fsys, repo: repo}
}

func (sc *scopeCache) load(scopeDir string) (*scopeEntry, error) {
	sc.mu.RLock()
	if e, ok := sc.m[scopeDir]; ok {
		sc.mu.RUnlock()
		return e, nil
	}
	sc.mu.RUnlock()

	contractPath := filepath.Join(scopeDir, port.ContractFile)
	e := &scopeEntry{contractPath: contractPath}
	data, err := sc.fsys.ReadFile(contractPath)
	if err != nil {
		e.loadErr = makeContractLoadErrors(contractPath, err)
	} else {
		g, loadErr := sc.repo.Load(string(data))
		if loadErr != nil {
			e.loadErr = makeContractLoadErrors(contractPath, loadErr)
		} else {
			e.graphIndex = graph.NewGraphIndex(g)
		}
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()
	if existing, ok := sc.m[scopeDir]; ok {
		return existing, nil
	}
	sc.m[scopeDir] = e
	return e, nil
}

func (sc *scopeCache) iterate(fn func(entry *scopeEntry)) {
	sc.mu.Lock()
	entries := make([]*scopeEntry, 0, len(sc.m))
	for k := range sc.m {
		entries = append(entries, sc.m[k])
	}
	sc.mu.Unlock()

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].contractPath < entries[j].contractPath
	})
	for _, e := range entries {
		fn(e)
	}
}

type ancestorContract struct {
	dir          string
	graphIndex   *graph.GraphIndex
	contractPath string
}

func ancestorContracts(fsys port.FileSystem, scopeDir, capsuleDir string, sc *scopeCache) []ancestorContract {
	var result []ancestorContract
	walkAncestorDirs(scopeDir, capsuleDir, func(parentDir string) bool {
		if _, err := fsys.Stat(filepath.Join(parentDir, port.ContractFile)); err != nil {
			return false
		}
		entry, serr := sc.load(parentDir)
		if serr != nil || entry.graphIndex == nil {
			return true
		}
		result = append(result, ancestorContract{dir: parentDir, graphIndex: entry.graphIndex, contractPath: entry.contractPath})
		return false
	})
	return result
}

func walkAncestorDirs(scopeDir, capsuleDir string, fn func(parentDir string) bool) {
	dir := scopeDir
	for dir != capsuleDir {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if fn(parent) {
			break
		}
		dir = parent
	}
}

type parseCache struct {
	m sync.Map
}

func newParseCache() *parseCache {
	return &parseCache{}
}

func (pc *parseCache) loadOrParse(ch *capsuleChecker, abs string) ([]port.ImportSpec, error) {
	if loaded, ok := pc.m.Load(abs); ok {
		entry := loaded.(*port.ParsedImports)
		return entry.Imports, nil
	}

	imports, err := ch.lang.ParseImports(ch.fsys, abs)
	if err != nil {
		return nil, err
	}
	full := &port.ParsedImports{Imports: imports, Hash: ""}
	if loaded, ok := pc.m.LoadOrStore(abs, full); ok {
		entry := loaded.(*port.ParsedImports)
		return entry.Imports, nil
	}
	return imports, nil
}
