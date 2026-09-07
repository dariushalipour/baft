package check

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	key        string
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
	Lang            port.Language
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
	// ResolveAncestorNode resolves source and target node IDs for ancestor
	// scope checking. srcFallback is used when source is not found.
	ResolveAncestorNode(srcNS, targetNS string, graph *graph.GraphIndex, srcFallback string) (srcNode, dstNode string)
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
		key:        targetPath,
		inScopeKey: relToSlash(scopeDir, targetAbs),
		targetRel:  relToSlash(capsule.Dir, targetAbs),
	}}, nil
}

func (s *pathResolutionStrategy) ResolveNode(fileAbs, targetKey string, scopeGraph *graph.GraphIndex) string {
	return scopeGraph.NodeForPath(targetKey)
}

func (s *pathResolutionStrategy) ResolveAncestorNode(srcNS, targetNS string, graph *graph.GraphIndex, srcFallback string) (srcNode, dstNode string) {
	srcA := graph.NodeForPath(srcNS)
	dstA := graph.NodeForPath(targetNS)
	if srcA == "" {
		srcA = srcFallback
	}
	return srcA, dstA
}

func (s *pathResolutionStrategy) ResolveCrossScope(ctx *CrossScopeContext) []port.Violation {
	if !escapesScope(relToSlash(ctx.ScopeDir, ctx.TargetAbs)) {
		dst := ctx.ScopeGraph.NodeForPath(relToSlash(ctx.ScopeDir, ctx.TargetAbs))
		if dst != "" {
			if !ctx.ScopeGraph.Graph().Allows(ctx.Src, dst) {
				return []port.Violation{makeRelationViolation(ctx.SrcAbs, ctx.FileRel, ctx.Spec, ctx.Src, ctx.TargetRel, dst, ctx.CfgPath)}
			}
			return nil
		}
	}

	for _, anc := range ancestorContracts(ctx.Fsys, ctx.ScopeDir, ctx.Capsule.Dir, ctx.ScopeCache) {
		srcRel := relToSlash(anc.dir, ctx.SrcAbs)
		dstRel := relToSlash(anc.dir, ctx.TargetAbs)
		srcA := anc.graphIndex.NodeForPath(srcRel)
		dstA := anc.graphIndex.NodeForPath(dstRel)
		if srcA != "" && dstA != "" {
			if !anc.graphIndex.Graph().Allows(srcA, dstA) {
				return []port.Violation{makeRelationViolation(ctx.SrcAbs, ctx.FileRel, ctx.Spec, srcA, ctx.TargetRel, dstA, anc.contractPath)}
			}
			return nil
		}
	}

	if ctx.HasRootContract && ctx.RootGraphIndex != nil {
		srcParent := ctx.RootGraphIndex.NodeForPath(relToSlash(ctx.Capsule.Dir, ctx.SrcAbs))
		dstParent := ctx.RootGraphIndex.NodeForPath(relToSlash(ctx.Capsule.Dir, ctx.TargetAbs))
		if srcParent != "" && dstParent != "" {
			if !ctx.RootGraphIndex.Graph().Allows(srcParent, dstParent) {
				return []port.Violation{makeRelationViolation(ctx.SrcAbs, ctx.FileRel, ctx.Spec, srcParent, ctx.TargetRel, dstParent, ctx.ContractPathAbs)}
			}
			return nil
		}
	}

	return nil
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
			key:        spec.Namespace,
			inScopeKey: inScopeKey,
			targetRel:  spec.Namespace,
		})
	}
	return infos, nil
}

func (s *namespaceResolutionStrategy) ResolveNode(fileAbs, targetKey string, scopeGraph *graph.GraphIndex) string {
	return scopeGraph.NodeForNamespace(targetKey)
}

func (s *namespaceResolutionStrategy) ResolveAncestorNode(srcNS, targetNS string, graph *graph.GraphIndex, srcFallback string) (srcNode, dstNode string) {
	srcA := graph.NodeForNamespace(srcNS)
	dstA := graph.NodeForNamespace(targetNS)
	if srcA == "" {
		srcA = srcFallback
	}
	return srcA, dstA
}

func (s *namespaceResolutionStrategy) ResolveCrossScope(ctx *CrossScopeContext) []port.Violation {
	srcNS, ok := s.fileNSMap[ctx.SrcAbs]
	if !ok || srcNS == "" {
		srcNS = ctx.Src
	}
	targetNS, ok := s.fileNSMap[ctx.TargetAbs]
	if !ok || targetNS == "" {
		targetNS = ctx.Spec.Namespace
	}

	// Check current scope first
	dst := ctx.ScopeGraph.NodeForNamespace(targetNS)
	if dst != "" {
		srcCheck := ctx.ScopeGraph.NodeForNamespace(srcNS)
		if srcCheck == "" {
			srcCheck = ctx.Src
		}
		if !ctx.ScopeGraph.Graph().Allows(srcCheck, dst) {
			return []port.Violation{makeRelationViolation(ctx.SrcAbs, ctx.FileRel, ctx.Spec, srcCheck, targetNS, dst, ctx.CfgPath)}
		}
		return nil
	}

	// Walk ancestors — only if ancestor contract is also in namespace mode.
	for _, anc := range ancestorContracts(ctx.Fsys, ctx.ScopeDir, ctx.Capsule.Dir, ctx.ScopeCache) {
		if !anc.graphIndex.Graph().NamespaceMode {
			continue
		}
		srcA := anc.graphIndex.NodeForNamespace(srcNS)
		dstA := anc.graphIndex.NodeForNamespace(targetNS)
		if srcA != "" && dstA != "" {
			if !anc.graphIndex.Graph().Allows(srcA, dstA) {
				return []port.Violation{makeRelationViolation(ctx.SrcAbs, ctx.FileRel, ctx.Spec, srcA, targetNS, dstA, anc.contractPath)}
			}
			return nil
		}
	}

	// Fallback to root contract — only if root contract is in namespace mode.
	if ctx.HasRootContract && ctx.RootGraphIndex.Graph().NamespaceMode {
		srcParent := ctx.RootGraphIndex.NodeForNamespace(srcNS)
		dstParent := ctx.RootGraphIndex.NodeForNamespace(targetNS)
		if srcParent != "" && dstParent != "" {
			if !ctx.RootGraphIndex.Graph().Allows(srcParent, dstParent) {
				return []port.Violation{makeRelationViolation(ctx.SrcAbs, ctx.FileRel, ctx.Spec, srcParent, targetNS, dstParent, ctx.ContractPathAbs)}
			}
			return nil
		}
	}

	return nil
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

func (ch *capsuleChecker) walk(ctx context.Context, fsys port.FileSystem, capsuleDir string) error {
	contractDirSep := ch.contractDirAbs + string(filepath.Separator)
	var nestedSep []string
	for _, nested := range ch.nestedCapsuleDirs {
		nestedSep = append(nestedSep, nested+string(filepath.Separator))
	}

	var filesToCheck []fileWork

	err := service.WalkAllFiles(ctx, fsys, capsuleDir, ch.lang, func(abs, rel string) error {
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

	ch.files = filesToCheck
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

	numWorkers := min(runtime.NumCPU(), len(filesToCheck))
	workChan := make(chan fileWork, numWorkers*2)
	results := make(chan fileCheckResult, numWorkers*2)

	go func() {
		for _, fw := range filesToCheck {
			select {
			case workChan <- fw:
			case <-ctx.Done():
				return
			}
		}
		close(workChan)
	}()

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var acc fileCheckResult
			for fw := range workChan {
				res := ch.checkFileResult(fsys, fw.abs, fw.rel, fw.scopeDir)
				if res.err != nil {
					select {
					case results <- res:
					case <-ctx.Done():
					}
					return
				}
				acc.filesEncountered += res.filesEncountered
				acc.filesScanned += res.filesScanned
				acc.relations += res.relations
				acc.violations = append(acc.violations, res.violations...)
			}
			select {
			case results <- acc:
			case <-ctx.Done():
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

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

func (ch *capsuleChecker) checkFileResult(fsys port.FileSystem, abs, fileRel string, scopeDir string) fileCheckResult {
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
		return fileCheckResult{filesEncountered: filesEncountered, violations: ch.handleNoNodeResult(fsys, abs, fileRel, scopeDir, cfgPath)}
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

func (ch *capsuleChecker) handleNoNodeResult(fsys port.FileSystem, abs, fileRel, scopeDir, cfgPath string) []port.Violation {
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
		targetPath, internal := ch.lang.ResolveInternalTarget(fsys, spec, ch.capsule, fileRel)
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
					if !g.Allows(src, dst) {
						violations = append(violations, makeRelationViolation(abs, scopeRel, spec, src, t.inScopeKey, dst, cfgPath))
					}
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
				Lang:            ch.lang,
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

// witnessKeys returns the walked files as paths relative to the contract at
// cfgPath, the candidates for a node-overlap witness. It reuses the walk's own
// file list, so no extra tree traversal is needed.
func (ch *capsuleChecker) witnessKeys(cfgPath string) []string {
	baseDir := filepath.Dir(cfgPath)
	keys := make([]string, 0, len(ch.files))
	for _, fw := range ch.files {
		if rel := relToSlash(baseDir, fw.abs); !escapesScope(rel) {
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
