package check

import (
	"context"
	"io/fs"
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

// fileWork represents a single file to be checked.
type fileWork struct {
	abs      string
	rel      string
	scopeDir string
}

func (ch *capsuleChecker) walk(ctx context.Context, fsys port.FileSystem, capsuleDir string) error {
	// Pre-compute separator strings to avoid repeated concatenation.
	configDirSep := ch.configDirAbs + string(filepath.Separator)
	var nestedSep []string
	for _, nested := range ch.nestedCapsuleDirs {
		nestedSep = append(nestedSep, nested+string(filepath.Separator))
	}

	// Collect all files to check first, then process in parallel.
	// This avoids holding filesystem locks during the parallel phase.
	var filesToCheck []fileWork

	err := service.WalkAllFiles(fsys, capsuleDir, ch.lang, func(abs, rel string) error {
		if abs != ch.configDirAbs && !strings.HasPrefix(abs, configDirSep) {
			return nil
		}
		for _, nsep := range nestedSep {
			if strings.HasPrefix(abs, nsep) {
				return nil
			}
		}
		scopeDir := service.GoverningScope(fsys, abs, ch.capsule.Dir)
		filesToCheck = append(filesToCheck, fileWork{abs: abs, rel: rel, scopeDir: scopeDir})
		return nil
	})
	if err != nil {
		return err
	}

	if len(filesToCheck) == 0 {
		return nil
	}

	// Use a worker pool to process files in parallel.
	// The book recommends bounded concurrency via worker pools
	// to avoid overwhelming the system with too many goroutines.
	numWorkers := min(runtime.NumCPU(), len(filesToCheck))
	workChan := make(chan fileWork, len(filesToCheck))
	results := make(chan fileCheckResult, len(filesToCheck))

	for _, fw := range filesToCheck {
		workChan <- fw
	}
	close(workChan)

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fw := range workChan {
				select {
				case results <- ch.checkFileResult(fsys, fw.abs, fw.rel, fw.scopeDir):
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Aggregate results from all workers
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

func (ch *capsuleChecker) checkFile(fsys port.FileSystem, abs, fileRel string, scopeDir string) error {
	res := ch.checkFileResult(fsys, abs, fileRel, scopeDir)
	if res.err != nil {
		return res.err
	}
	ch.mergeFileResult(res)
	return nil
}

func (ch *capsuleChecker) checkFileResult(fsys port.FileSystem, abs, fileRel string, scopeDir string) fileCheckResult {
	cfgPath, scopeGraph := ch.resolveScope(scopeDir)
	if scopeGraph == nil {
		return fileCheckResult{}
	}

	scopeRel := relToSlash(scopeDir, abs)
	filesEncountered := 1

	src := scopeGraph.NodeForPath(scopeRel)
	if src == "" {
		violations := ch.handleNoNodeResult(fsys, abs, fileRel, scopeRel, cfgPath)
		return fileCheckResult{
			filesEncountered: filesEncountered,
			violations:       violations,
		}
	}

	imports, err := ch.parseCache.loadOrParse(ch, abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return fileCheckResult{err: err}
		}
	} else {
		filesScanned := 1
		var relations int
		// Pre-allocate with estimated capacity to avoid repeated allocations.
		violations := make([]port.Violation, 0, len(imports))
		for _, spec := range imports {
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

	return fileCheckResult{
		filesEncountered: filesEncountered,
	}
}

func (ch *capsuleChecker) mergeFileResult(res fileCheckResult) {
	ch.res.filesEncountered += res.filesEncountered
	ch.res.filesScanned += res.filesScanned
	ch.res.relations += res.relations
	ch.res.violations = append(ch.res.violations, res.violations...)
}

func (ch *capsuleChecker) handleNoNode(fsys port.FileSystem, abs, fileRel, scopeRel, cfgPath string) error {
	ch.res.violations = append(ch.res.violations, makeNoNodeViolation(abs, scopeRel, cfgPath))
	imports, err := ch.lang.ParseImports(fsys, abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		for _, spec := range imports {
			_, internal := ch.lang.ResolveInternalTarget(fsys, spec, ch.capsule, fileRel)
			if internal {
				ch.res.violations = append(ch.res.violations, makeImportNoNodeViolation(abs, scopeRel, spec, cfgPath))
			}
		}
	}
	return nil
}

func (ch *capsuleChecker) handleNoNodeResult(fsys port.FileSystem, abs, fileRel, scopeRel, cfgPath string) []port.Violation {
	noNode := makeNoNodeViolation(abs, scopeRel, cfgPath)
	imports, err := ch.parseCache.loadOrParse(ch, abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return []port.Violation{noNode}
		}
		return []port.Violation{noNode}
	}
	// Pre-allocate with capacity for the no-node violation plus imports.
	violations := make([]port.Violation, 0, 1+len(imports))
	violations = append(violations, noNode)
	for _, spec := range imports {
		_, internal := ch.lang.ResolveInternalTarget(fsys, spec, ch.capsule, fileRel)
		if internal {
			violations = append(violations, makeImportNoNodeViolation(abs, scopeRel, spec, cfgPath))
		}
	}
	return violations
}

func (ch *capsuleChecker) checkImport(spec port.ImportSpec, abs, fileRel, scopeRel, cfgPath string, scopeGraph *graph.Graph, src, scopeDir string) {
	targetPath, internal := ch.lang.ResolveInternalTarget(ch.fsys, spec, ch.capsule, fileRel)
	if !internal {
		return
	}
	ch.res.relations++

	targetAbs := absPath(ch.capsule.Dir, targetPath)
	targetScope := service.GoverningScope(ch.fsys, targetAbs, ch.capsule.Dir)

	if scopeDir == targetScope {
		ch.checkInScope(abs, scopeRel, cfgPath, scopeDir, scopeGraph, spec, src, targetAbs)
	} else {
		v := ch.checkCrossScope(abs, fileRel, spec, src, targetPath, cfgPath, scopeGraph, scopeDir)
		ch.res.violations = append(ch.res.violations, v...)
	}
}

func (ch *capsuleChecker) checkImportResult(spec port.ImportSpec, abs, fileRel, scopeRel, cfgPath string, scopeGraph *graph.Graph, src, scopeDir string) (int, []port.Violation) {
	targetPath, internal := ch.lang.ResolveInternalTarget(ch.fsys, spec, ch.capsule, fileRel)
	if !internal {
		return 0, nil
	}

	targetAbs := absPath(ch.capsule.Dir, targetPath)
	targetScope := service.GoverningScope(ch.fsys, targetAbs, ch.capsule.Dir)

	if scopeDir == targetScope {
		return 1, ch.checkInScopeResult(abs, scopeRel, cfgPath, scopeDir, scopeGraph, spec, src, targetAbs)
	} else {
		return 1, ch.checkCrossScope(abs, fileRel, spec, src, targetPath, cfgPath, scopeGraph, scopeDir)
	}
}

func (ch *capsuleChecker) checkInScope(abs, scopeRel, cfgPath, scopeDir string, scopeGraph *graph.Graph, spec port.ImportSpec, src string, targetAbs string) {
	scopeTargetRel := relToSlash(scopeDir, targetAbs)
	dst := scopeGraph.NodeForPath(scopeTargetRel)
	if dst == "" {
		ch.res.violations = append(ch.res.violations, makeImportNoNodeViolation(abs, scopeRel, spec, cfgPath))
		return
	}
	if dst == src {
		if scopeGraph.IsEndophobic(src) {
			ch.res.violations = append(ch.res.violations, makeEndophobicViolation(abs, scopeRel, spec, scopeTargetRel, src, cfgPath))
		}
		return
	}
	if !scopeGraph.Allows(src, dst) {
		ch.res.violations = append(ch.res.violations, makeRelationViolation(abs, scopeRel, spec, src, scopeTargetRel, dst, cfgPath))
	}
}

func (ch *capsuleChecker) checkInScopeResult(abs, scopeRel, cfgPath, scopeDir string, scopeGraph *graph.Graph, spec port.ImportSpec, src string, targetAbs string) []port.Violation {
	scopeTargetRel := relToSlash(scopeDir, targetAbs)
	dst := scopeGraph.NodeForPath(scopeTargetRel)
	if dst == "" {
		v := makeImportNoNodeViolation(abs, scopeRel, spec, cfgPath)
		return []port.Violation{v}
	}
	if dst == src {
		if scopeGraph.IsEndophobic(src) {
			v := makeEndophobicViolation(abs, scopeRel, spec, scopeTargetRel, src, cfgPath)
			return []port.Violation{v}
		}
		return nil
	}
	if !scopeGraph.Allows(src, dst) {
		v := makeRelationViolation(abs, scopeRel, spec, src, scopeTargetRel, dst, cfgPath)
		return []port.Violation{v}
	}
	return nil
}

func (ch *capsuleChecker) checkCrossScope(srcAbs, fileRel string, spec port.ImportSpec, src, targetPath, cfgPath string, scopeGraph *graph.Graph, scopeDir string) []port.Violation {
	targetAbs := absPath(ch.capsule.Dir, targetPath)

	targetRel := relToSlash(scopeDir, targetAbs)
	if !escapesScope(targetRel) {
		dst := scopeGraph.NodeForPath(targetRel)
		if dst != "" {
			if !scopeGraph.Allows(src, dst) {
				v := makeRelationViolation(srcAbs, fileRel, spec, src, targetRel, dst, cfgPath)
				return []port.Violation{v}
			}
			return nil
		}
	}

	for _, anc := range ancestorConfigs(ch.fsys, scopeDir, ch.capsule.Dir, ch.scopeCache) {
		srcRel := relToSlash(anc.dir, srcAbs)
		dstRel := relToSlash(anc.dir, targetAbs)
		srcA := anc.graph.NodeForPath(srcRel)
		dstA := anc.graph.NodeForPath(dstRel)
		if srcA != "" && dstA != "" {
			if !anc.graph.Allows(srcA, dstA) {
				v := makeRelationViolation(srcAbs, fileRel, spec, srcA, targetPath, dstA, anc.cfgPath)
				return []port.Violation{v}
			}
			return nil
		}
	}

	if ch.hasRootConfig {
		srcParent := ch.rootGraph.NodeForPath(relToSlash(ch.capsule.Dir, srcAbs))
		dstParent := ch.rootGraph.NodeForPath(relToSlash(ch.capsule.Dir, targetAbs))
		if srcParent != "" && dstParent != "" {
			if !ch.rootGraph.Allows(srcParent, dstParent) {
				parentTargetRel := relToSlash(ch.capsule.Dir, targetAbs)
				v := makeRelationViolation(srcAbs, fileRel, spec, srcParent, parentTargetRel, dstParent, ch.configPathAbs)
				return []port.Violation{v}
			}
			return nil
		}
	}

	return nil
}

func escapesScope(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, "../")
}

func (ch *capsuleChecker) resolveScope(scopeDir string) (string, *graph.Graph) {
	if scopeDir == ch.capsule.Dir {
		if !ch.hasRootConfig {
			return "", nil
		}
		return ch.configPathAbs, ch.rootGraph
	}
	entry, err := ch.scopeCache.load(scopeDir)
	if err != nil {
		return "", nil
	}
	return entry.cfgPath, entry.graph
}

func (ch *capsuleChecker) intermediateConfigExists(scopeDir string) bool {
	dir := scopeDir
	for dir != ch.capsule.Dir {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if _, err := ch.fsys.Stat(filepath.Join(parent, port.ConfigFile)); err == nil {
			return true
		}
		dir = parent
	}
	return false
}

func (ch *capsuleChecker) validateAll() {
	if ch.hasRootConfig {
		ch.validateGraph(ch.rootGraph, ch.configPathAbs)
	}
	ch.scopeCache.iterate(func(entry *scopeEntry) {
		if len(entry.loadErr) > 0 {
			ch.res.errors = append(ch.res.errors, entry.loadErr...)
			return
		}
		ch.validateGraph(entry.graph, entry.cfgPath)
	})
}

func (ch *capsuleChecker) validateGraph(g *graph.Graph, cfgPath string) {
	duplicateGlobErrors := duplicateNodeGlobErrors(g, cfgPath)
	if len(duplicateGlobErrors) > 0 {
		ch.res.hasDuplicateGlobError = true
	}
	for _, err := range duplicateGlobErrors {
		ch.res.errors = append(ch.res.errors, err)
	}
	if !ch.lang.SupportsFileGlobs() {
		for id, glob := range g.Nodes {
			if graph.IsFileGlob(glob) {
				ch.res.errors = append(ch.res.errors, makeFileGlobUnsupportedError(id, cfgPath, g.NodeLines[id], glob))
			}
		}
	}
	for id, glob := range g.Nodes {
		for _, msg := range graph.ValidateNodeGlob(glob) {
			ch.res.errors = append(ch.res.errors, makeInvalidNodeGlobError(id, cfgPath, g.NodeLines[id], glob, msg))
		}
	}
	ch.findOverlappingNodes(g, cfgPath)
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

	errs := make([]port.Violation, 0, len(globs))
	for _, glob := range globs {
		ids := byGlob[glob]
		line := g.NodeLines[ids[1]]
		errs = append(errs, makeDuplicateNodeGlobError(cfgPath, line, glob, ids))
	}
	return errs
}

func (ch *capsuleChecker) findOverlappingNodes(g *graph.Graph, cfgPath string) {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	scopeDir := filepath.Dir(cfgPath)

	// Collect candidate pairs that have overlapping globs (cheap check).
	var candidatePairs [][2]string
	for i := 0; i < len(ids); i++ {
		aGlob := g.Nodes[ids[i]]
		if graph.IsFileGlob(aGlob) {
			continue
		}
		for j := i + 1; j < len(ids); j++ {
			bGlob := g.Nodes[ids[j]]
			if aGlob == bGlob {
				continue
			}
			if graph.IsFileGlob(bGlob) {
				continue
			}
			if !graph.GlobsOverlap(aGlob, bGlob) {
				continue
			}
			candidatePairs = append(candidatePairs, [2]string{ids[i], ids[j]})
		}
	}

	// Parallelize witness file search across candidate pairs.
	// The book recommends bounded concurrency for independent tasks.
	numWorkers := min(runtime.NumCPU(), len(candidatePairs))
	if numWorkers < 1 {
		numWorkers = 1
	}

	type overlapResult struct {
		witness string
		i       string
		j       string
	}
	overlapChan := make(chan overlapResult, len(candidatePairs))

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, pair := range candidatePairs {
				a, b := pair[0], pair[1]
				aGlob, bGlob := g.Nodes[a], g.Nodes[b]
				if witness := ch.findWitnessFile(aGlob, bGlob, scopeDir); witness != "" {
					overlapChan <- overlapResult{witness: witness, i: a, j: b}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(overlapChan)
	}()

	for r := range overlapChan {
		ch.res.errors = append(ch.res.errors, makeOverlapError(r.i, r.j, cfgPath, g.NodeLines[r.i], g.NodeLines[r.j], r.witness))
		ch.res.hasOverlapError = true
	}
}

// findWitnessFile checks if a single file matches both globs.
func (ch *capsuleChecker) findWitnessFile(aGlob, bGlob string, baseDir string) string {
	var witness string
	_ = service.WalkAllFiles(ch.fsys, baseDir, ch.lang, func(abs, rel string) error {
		if witness != "" {
			return fs.SkipDir
		}
		key := graph.NodeKeyForDir(rel)
		if graph.MatchDirGlob(aGlob, key) && graph.MatchDirGlob(bGlob, key) {
			witness = relToSlash(baseDir, abs)
			return fs.SkipDir
		}
		return nil
	})
	return witness
}

type scopeCache struct {
	mu   sync.RWMutex // Use RWMutex so concurrent cache hits don't block each other.
	m    map[string]*scopeEntry
	fsys port.FileSystem
	repo port.GraphRepository
}

type scopeEntry struct {
	graph   *graph.Graph
	cfgPath string
	loadErr []port.Violation
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

	cfg := filepath.Join(scopeDir, port.ConfigFile)
	e := &scopeEntry{cfgPath: cfg}
	data, err := sc.fsys.ReadFile(cfg)
	if err != nil {
		e.loadErr = makeConfigLoadErrors(cfg, err)
	} else {
		g, loadErr := sc.repo.Load(string(data))
		if loadErr != nil {
			e.loadErr = makeConfigLoadErrors(cfg, loadErr)
		} else {
			e.graph = g
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
		return entries[i].cfgPath < entries[j].cfgPath
	})
	for _, e := range entries {
		fn(e)
	}
}

type ancestorConfig struct {
	dir     string
	graph   *graph.Graph
	cfgPath string
}

func ancestorConfigs(fsys port.FileSystem, scopeDir, capsuleDir string, sc *scopeCache) []ancestorConfig {
	var result []ancestorConfig
	walkAncestorDirs(scopeDir, capsuleDir, func(parentDir string) bool {
		if _, err := fsys.Stat(filepath.Join(parentDir, port.ConfigFile)); err != nil {
			return false
		}
		entry, serr := sc.load(parentDir)
		if serr != nil || len(entry.loadErr) > 0 || entry.graph == nil {
			return true
		}
		result = append(result, ancestorConfig{dir: parentDir, graph: entry.graph, cfgPath: entry.cfgPath})
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

func hasScopedConfig(fsys port.FileSystem, capsuleDir string) bool {
	found := false
	_ = fsys.WalkDir(capsuleDir, func(abs string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		if d.Name() == port.ConfigFile && abs != filepath.Join(capsuleDir, port.ConfigFile) {
			found = true
		}
		return nil
	})
	return found
}

// parseCache provides thread-safe caching of ParseImports results.
// This avoids re-parsing the same file multiple times during check operations.
type parseCache struct {
	m sync.Map // sync.Map is more efficient for read-heavy workloads with many concurrent goroutines.
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
	// Store in cache for future lookups using LoadOrStore to avoid duplicate parsing.
	full := &port.ParsedImports{Imports: imports, Hash: ""}
	if loaded, ok := pc.m.LoadOrStore(abs, full); ok {
		// Another goroutine stored it first.
		entry := loaded.(*port.ParsedImports)
		return entry.Imports, nil
	}
	return imports, nil
}
