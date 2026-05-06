package check

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dariushalipour/strata/internal/application/service"
	"github.com/dariushalipour/strata/internal/domain/graph"
	"github.com/dariushalipour/strata/internal/port"
)

func (ch *capsuleChecker) walk(fsys port.FileSystem, capsuleDir string) error {
	return service.WalkAllFiles(fsys, capsuleDir, ch.lang, func(abs, rel string) error {
		if abs != ch.configDirAbs && !strings.HasPrefix(abs, ch.configDirAbs+string(filepath.Separator)) {
			return nil
		}
		for _, nested := range ch.nestedCapsuleDirs {
			if strings.HasPrefix(abs, nested+string(filepath.Separator)) {
				return nil
			}
		}
		scopeDir := service.GoverningScope(fsys, abs, ch.capsule.Dir)
		return ch.checkFile(fsys, abs, rel, scopeDir)
	})
}

func (ch *capsuleChecker) checkFile(fsys port.FileSystem, abs, fileRel string, scopeDir string) error {
	cfgPath, scopeGraph := ch.resolveScope(scopeDir)
	if scopeGraph == nil {
		return nil
	}

	scopeRel := relToSlash(scopeDir, abs)
	ch.res.filesEncountered++

	src := scopeGraph.NodeForPath(scopeRel)
	if src == "" {
		return ch.handleNoNode(fsys, abs, fileRel, scopeRel, cfgPath)
	}

	imports, err := ch.lang.ParseImports(fsys, abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		ch.res.filesScanned++
		for _, spec := range imports {
			ch.checkImport(spec, abs, fileRel, scopeRel, cfgPath, scopeGraph, src, scopeDir)
		}
	}
	return nil
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

func (ch *capsuleChecker) checkCrossScope(srcAbs, fileRel string, spec port.ImportSpec, src, targetPath, cfgPath string, scopeGraph *graph.Graph, scopeDir string) []port.Violation {
	targetAbs := absPath(ch.capsule.Dir, targetPath)

	targetRel := relToSlash(scopeDir, targetAbs)
	dst := scopeGraph.NodeForPath(targetRel)
	if dst != "" {
		if !scopeGraph.Allows(src, dst) {
			return []port.Violation{makeRelationViolation(srcAbs, fileRel, spec, src, targetRel, dst, cfgPath)}
		}
		return nil
	}

	for _, anc := range ancestorConfigs(ch.fsys, scopeDir, ch.capsule.Dir, ch.scopeCache) {
		srcRel := relToSlash(anc.dir, srcAbs)
		dstRel := relToSlash(anc.dir, targetAbs)
		srcA := anc.graph.NodeForPath(srcRel)
		dstA := anc.graph.NodeForPath(dstRel)
		if srcA != "" && dstA != "" {
			if !anc.graph.Allows(srcA, dstA) {
				return []port.Violation{makeRelationViolation(srcAbs, fileRel, spec, srcA, targetPath, dstA, anc.cfgPath)}
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
				return []port.Violation{makeRelationViolation(srcAbs, fileRel, spec, srcParent, parentTargetRel, dstParent, ch.configPathAbs)}
			}
			return nil
		}
	}

	if !ch.intermediateConfigExists(scopeDir) {
		return []port.Violation{makeImportNoNodeViolation(srcAbs, fileRel, spec, cfgPath)}
	}
	return nil
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
		ch.validateGraph(entry.graph, entry.cfgPath)
	})
}

func (ch *capsuleChecker) validateGraph(g *graph.Graph, cfgPath string) {
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

func (ch *capsuleChecker) findOverlappingNodes(g *graph.Graph, cfgPath string) {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a, b := ids[i], ids[j]
			aGlob, bGlob := g.Nodes[a], g.Nodes[b]
			if graph.IsFileGlob(aGlob) || graph.IsFileGlob(bGlob) {
				continue
			}
			if !graph.GlobsOverlap(aGlob, bGlob) {
				continue
			}
			if witness := ch.findWitnessFile(aGlob, bGlob); witness != "" {
				ch.res.errors = append(ch.res.errors, makeOverlapError(a, b, cfgPath, g.NodeLines[a], g.NodeLines[b], witness))
				ch.res.hasOverlapError = true
			}
		}
	}
}

func (ch *capsuleChecker) findWitnessFile(aGlob, bGlob string) string {
	var witness string
	_ = service.WalkAllFiles(ch.fsys, ch.capsule.Dir, ch.lang, func(abs, rel string) error {
		if witness != "" {
			return fs.SkipDir
		}
		key := graph.NodeKeyForDir(rel)
		if graph.MatchDirGlob(aGlob, key) && graph.MatchDirGlob(bGlob, key) {
			witness = relToSlash(ch.capsule.Dir, abs)
			return fs.SkipDir
		}
		return nil
	})
	return witness
}

type scopeCache struct {
	mu   sync.Mutex
	m    map[string]*scopeEntry
	fsys port.FileSystem
	repo port.GraphRepository
}

type scopeEntry struct {
	graph   *graph.Graph
	cfgPath string
}

func newScopeCache(fsys port.FileSystem, repo port.GraphRepository) *scopeCache {
	return &scopeCache{m: make(map[string]*scopeEntry), fsys: fsys, repo: repo}
}

func (sc *scopeCache) load(scopeDir string) (*scopeEntry, error) {
	sc.mu.Lock()
	if e, ok := sc.m[scopeDir]; ok {
		sc.mu.Unlock()
		return e, nil
	}
	sc.mu.Unlock()

	cfg := filepath.Join(scopeDir, port.ConfigFile)
	data, err := sc.fsys.ReadFile(cfg)
	if err != nil {
		return nil, err
	}
	g, err := sc.repo.Load(string(data))
	if err != nil {
		return nil, err
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()
	if existing, ok := sc.m[scopeDir]; ok {
		return existing, nil
	}
	e := &scopeEntry{graph: g, cfgPath: cfg}
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
		if serr != nil {
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
