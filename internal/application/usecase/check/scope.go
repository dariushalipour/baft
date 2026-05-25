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

type fileWork struct {
	abs      string
	rel      string
	scopeDir string
}

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
				return nil
			}
		}
		scopeDir := service.TrackingScope(fsys, abs, ch.capsule.Dir)
		filesToCheck = append(filesToCheck, fileWork{abs: abs, rel: rel, scopeDir: scopeDir})
		return nil
	})
	if err != nil {
		return err
	}

	if len(filesToCheck) == 0 {
		return nil
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

func (ch *capsuleChecker) handleNoNodeResult(fsys port.FileSystem, abs, fileRel, scopeRel, cfgPath string) []port.Violation {
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
		if !port.IsTargetVisible(ch.fsys, targetAbs) {
			continue
		}
		violations = append(violations, makeImportNoNodeViolation(abs, scopeRel, spec, cfgPath))
	}
	return violations
}

func (ch *capsuleChecker) checkImportResult(spec port.ImportSpec, abs, fileRel, scopeRel, cfgPath string, scopeGraph *graph.GraphIndex, src, scopeDir string) (int, []port.Violation) {
	targetPath, internal := ch.lang.ResolveInternalTarget(ch.fsys, spec, ch.capsule, fileRel)
	if !internal {
		return 0, nil
	}

	targetAbs := absPath(ch.capsule.Dir, targetPath)

	if !port.IsTargetVisible(ch.fsys, targetAbs) {
		return 0, nil
	}

	targetScope := service.TrackingScope(ch.fsys, targetAbs, ch.capsule.Dir)

	if scopeDir == targetScope {
		return 1, ch.checkInScopeResult(abs, scopeRel, cfgPath, scopeDir, scopeGraph, spec, src, targetAbs)
	} else {
		return 1, ch.checkCrossScope(abs, fileRel, spec, src, targetPath, cfgPath, scopeGraph, scopeDir)
	}
}

func (ch *capsuleChecker) checkInScopeResult(abs, scopeRel, cfgPath, scopeDir string, scopeIndex *graph.GraphIndex, spec port.ImportSpec, src string, targetAbs string) []port.Violation {
	g := scopeIndex.Graph()
	scopeTargetRel := relToSlash(scopeDir, targetAbs)
	dst := scopeIndex.NodeForPath(scopeTargetRel)
	if dst == "" {
		v := makeImportNoNodeViolation(abs, scopeRel, spec, cfgPath)
		return []port.Violation{v}
	}
	if dst == src {
		if g.IsEndophobic(src) {
			v := makeEndophobicViolation(abs, scopeRel, spec, scopeTargetRel, src, cfgPath)
			return []port.Violation{v}
		}
		return nil
	}
	if !g.Allows(src, dst) {
		v := makeRelationViolation(abs, scopeRel, spec, src, scopeTargetRel, dst, cfgPath)
		return []port.Violation{v}
	}
	return nil
}

func (ch *capsuleChecker) checkCrossScope(srcAbs, fileRel string, spec port.ImportSpec, src, targetPath, cfgPath string, scopeIndex *graph.GraphIndex, scopeDir string) []port.Violation {
	targetAbs := absPath(ch.capsule.Dir, targetPath)

	targetRel := relToSlash(scopeDir, targetAbs)
	if !escapesScope(targetRel) {
		dst := scopeIndex.NodeForPath(targetRel)
		if dst != "" {
			if !scopeIndex.Graph().Allows(src, dst) {
				v := makeRelationViolation(srcAbs, fileRel, spec, src, targetRel, dst, cfgPath)
				return []port.Violation{v}
			}
			return nil
		}
	}

	for _, anc := range ancestorContracts(ch.fsys, scopeDir, ch.capsule.Dir, ch.scopeCache) {
		srcRel := relToSlash(anc.dir, srcAbs)
		dstRel := relToSlash(anc.dir, targetAbs)
		srcA := anc.graphIndex.NodeForPath(srcRel)
		dstA := anc.graphIndex.NodeForPath(dstRel)
		if srcA != "" && dstA != "" {
			if !anc.graphIndex.Graph().Allows(srcA, dstA) {
				v := makeRelationViolation(srcAbs, fileRel, spec, srcA, targetPath, dstA, anc.contractPath)
				return []port.Violation{v}
			}
			return nil
		}
	}

	if ch.hasRootContract {
		srcParent := ch.rootGraphIndex.NodeForPath(relToSlash(ch.capsule.Dir, srcAbs))
		dstParent := ch.rootGraphIndex.NodeForPath(relToSlash(ch.capsule.Dir, targetAbs))
		if srcParent != "" && dstParent != "" {
			if !ch.rootGraphIndex.Graph().Allows(srcParent, dstParent) {
				parentTargetRel := relToSlash(ch.capsule.Dir, targetAbs)
				v := makeRelationViolation(srcAbs, fileRel, spec, srcParent, parentTargetRel, dstParent, ch.contractPathAbs)
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
		ch.applyContractValidation(validateContractGraph(ch.fsys, ch.lang, ch.contractPathAbs, g))
		ch.validateLanguageGraph(g, ch.contractPathAbs)
	}
	ch.scopeCache.iterate(func(entry *scopeEntry) {
		if len(entry.loadErr) > 0 {
			ch.res.errors = append(ch.res.errors, entry.loadErr...)
		}
		if entry.graphIndex != nil {
			g := entry.graphIndex.Graph()
			ch.applyContractValidation(validateContractGraph(ch.fsys, ch.lang, entry.contractPath, g))
			ch.validateLanguageGraph(g, entry.contractPath)
		}
	})
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
		for id, glob := range g.Nodes {
			if graph.IsFileGlob(glob) {
				ch.res.errors = append(ch.res.errors, makeFileGlobUnsupportedError(id, cfgPath, g.NodeLines[id], glob))
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

func hasScopedContract(fsys port.FileSystem, capsuleDir string) bool {
	found := false
	_ = fsys.WalkDir(context.Background(), capsuleDir, func(abs string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		if d.Name() == port.ContractFile && abs != filepath.Join(capsuleDir, port.ContractFile) {
			found = true
		}
		return nil
	})
	return found
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
