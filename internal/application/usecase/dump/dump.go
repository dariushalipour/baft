package dump

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/fs/ignorefs"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/application/usecase/check"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

// DumpResult holds the outcome of a dump run.
type DumpResult struct {
	Contracts []ContractDump
	Errors    []DumpError
}

// DumpError records a non-fatal error encountered while dumping a capsule.
type DumpError struct {
	Label string
	Err   error
}

func (d DumpError) Error() string {
	return fmt.Sprintf("%s: %s", d.Label, d.Err)
}

// ContractDump holds the outcome for a single contract dump.
type ContractDump struct {
	FilesEncountered int
	FilesScanned     int
	Nodes            int
	Edges            int
	ContractPath     string
	IsNew            bool
	AmendDiff        *AmendDiff
}

// AmendDiff reports the number of nodes and edges added during an amend run.
type AmendDiff struct {
	Nodes int
	Edges int
}

type draftMode int

const (
	draftModeExactFiles draftMode = iota
	draftModeMergedDirs
)

type draftConfig struct {
	mode         draftMode
	expandedDirs map[string]bool
}

type fileRecord struct {
	rel     string
	imports []port.ImportSpec
}

type contractLoadError struct {
	contractPath string
	message      string
	cycleGroups  [][]string
}

func (e *contractLoadError) Error() string {
	return "contract-load-error: " + e.message
}

// Dump walks all capsules for every supplied language, parses every
// import in every scannable file, and writes a comprehensive BAFT.md
// that reflects the current dependency reality at maximum granularity.
func Run(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery) (*DumpResult, error) {
	return RunWith(fsys, rootDir, languages, repo, discovery, os.Stderr)
}

func RunWith(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery, logWriter io.Writer) (*DumpResult, error) {
	// Wrap the filesystem with ignore rules before discovery.
	wrapped, err := ignorefs.Wrap(fsys, ignorefs.Options{
		RootDir:           rootDir,
		BaseIgnoreEntries: discovery.BaseIgnoreEntries(),
	})
	if err != nil {
		if !errors.Is(err, ignorefs.ErrRepoRootUnreachable) {
			return nil, fmt.Errorf("ignorefs: %w", err)
		}
		fmt.Fprintln(logWriter, "warning: not inside a git repository — .gitignore/.baftignore rules from parent directories will not apply")
	}

	type entry struct {
		capsule port.Capsule
		lang    port.Language
	}
	var all []entry
	entries, err := discovery.Discover(wrapped, rootDir)
	if err != nil {
		return nil, err
	}
	langMap := make(map[string]port.Language)
	for _, lang := range languages {
		langMap[lang.Name()] = lang
	}
	for _, e := range entries {
		lang := langMap[e.LangName]
		if lang != nil {
			all = append(all, entry{capsule: e.Capsule, lang: lang})
		}
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("no capsules found at %s", rootDir)
	}

	sort.Slice(all, func(i, j int) bool {
		di := strings.Count(filepath.Clean(all[i].capsule.Dir), string(filepath.Separator))
		dj := strings.Count(filepath.Clean(all[j].capsule.Dir), string(filepath.Separator))
		if di != dj {
			return di > dj
		}
		return port.Label(all[i].capsule) < port.Label(all[j].capsule)
	})

	result := &DumpResult{}

	for _, e := range all {
		startDir := e.capsule.Dir
		if strings.HasPrefix(rootDir, e.capsule.Dir+string(filepath.Separator)) || rootDir == e.capsule.Dir {
			startDir = rootDir
		}
		label := port.Label(e.capsule)
		contractDir, rootExists := service.FindOrCreateContractDir(wrapped, startDir, e.capsule.Dir)
		rootContractPath := filepath.Join(contractDir, port.ContractFile)

		contracts, err := discoverScopedContracts(wrapped, e.capsule.Dir)
		if err != nil {
			de := DumpError{Label: label, Err: err}
			result.Errors = append(result.Errors, de)
			fmt.Fprintf(logWriter, "dump: %s: %s\n", label, err)
			continue
		}

		var scopedPaths []string
		for _, cp := range contracts {
			if cp == rootContractPath {
				continue
			}
			scopedPaths = append(scopedPaths, cp)
		}

		for _, contractPath := range scopedPaths {
			diff, err := amendContract(wrapped, rootDir, e.capsule, e.lang, repo, contractPath, defaultDraftConfig(e.capsule, e.lang, filepath.Dir(contractPath)))
			if err != nil {
				de := DumpError{Label: label, Err: err}
				result.Errors = append(result.Errors, de)
				fmt.Fprintf(logWriter, "dump: %s: %s\n", de.Label, de.Err)
				continue
			}
			if diff != nil {
				result.Contracts = append(result.Contracts, ContractDump{
					ContractPath: contractPath,
					IsNew:        false,
					AmendDiff:    diff,
				})
			}
		}

		if rootExists {
			diff, err := amendContract(wrapped, rootDir, e.capsule, e.lang, repo, rootContractPath, defaultDraftConfig(e.capsule, e.lang, contractDir))
			if err != nil {
				de := makeDumpError(label, err)
				result.Errors = append(result.Errors, de)
				fmt.Fprintf(logWriter, "dump: %s: %s\n", de.Label, de.Err)
				continue
			}
			if diff != nil {
				result.Contracts = append(result.Contracts, ContractDump{
					ContractPath: rootContractPath,
					IsNew:        false,
					AmendDiff:    diff,
				})
			}
			continue
		}
		cfg := defaultDraftConfig(e.capsule, e.lang, contractDir)
		capsuleRes, err := dumpCapsule(wrapped, e.capsule, e.lang, repo, rootDir, contractDir, cfg)
		if err != nil {
			de := DumpError{Label: label, Err: err}
			result.Errors = append(result.Errors, de)
			fmt.Fprintf(logWriter, "dump: %s: %s\n", label, err)
			continue
		}
		diff, err := amendContract(wrapped, rootDir, e.capsule, e.lang, repo, rootContractPath, cfg)
		if err != nil {
			if shouldTrySelectiveExpansion(cfg, err) {
				retryRes, retryDiff, retryErr, handled := retryCycleExpansion(wrapped, rootDir, e.capsule, e.lang, repo, contractDir, rootContractPath, cfg, err)
				if handled {
					if retryErr == nil || isFreshDraftCycle(retryErr) {
						retryRes.IsNew = true
						if retryErr == nil && retryDiff != nil {
							retryRes.AmendDiff = retryDiff
						}
						result.Contracts = append(result.Contracts, *retryRes)
						continue
					}
					de := makeDumpError(label, retryErr)
					result.Errors = append(result.Errors, de)
					fmt.Fprintf(logWriter, "dump: %s: %s\n", de.Label, de.Err)
					continue
				}
			}
			if isFreshDraftCycle(err) {
				capsuleRes.IsNew = true
				result.Contracts = append(result.Contracts, *capsuleRes)
				continue
			}
			de := makeDumpError(label, err)
			result.Errors = append(result.Errors, de)
			fmt.Fprintf(logWriter, "dump: %s: %s\n", de.Label, de.Err)
			continue
		}
		capsuleRes.IsNew = true
		if diff != nil {
			capsuleRes.AmendDiff = diff
		}
		result.Contracts = append(result.Contracts, *capsuleRes)
	}

	return result, nil
}

func dumpCapsule(fsys port.FileSystem, p port.Capsule, lang port.Language, repo port.GraphRepository, rootDir string, contractDir string, cfg draftConfig) (*ContractDump, error) {
	nodes := map[string]string{}
	edges := map[string]map[string]bool{}
	filesEncountered := 0
	filesScanned := 0
	var fileRecords []fileRecord

	walkFn := func(abs, rel string) error {
		imports, err := lang.ParseImports(fsys, abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		filesEncountered++
		filesScanned++

		fileRel := rel
		if !filepath.IsAbs(rel) {
			fileRel, _ = filepath.Rel(p.Dir, filepath.Join(contractDir, rel))
		}
		fileRel = filepath.ToSlash(fileRel)

		if shouldMergeContractDir(contractDir, p, lang, cfg) {
			fileRecords = append(fileRecords, fileRecord{rel: fileRel, imports: imports})
			return nil
		}

		srcID := nodeKey(rel, lang.SupportsFileGlobs())
		nodes[srcID] = srcID

		for _, spec := range imports {
			targetPath, internal := lang.ResolveInternalTarget(fsys, spec, p, fileRel)
			if !internal {
				continue
			}

			targetAbs := targetPath
			if !filepath.IsAbs(targetAbs) {
				targetAbs = filepath.Join(p.Dir, targetAbs)
			}
			targetAbs = filepath.Clean(targetAbs)

			// Skip ignored/baftignored targets.
			if !port.IsTargetVisible(fsys, targetAbs) {
				continue
			}

			contractDirClean := filepath.Clean(contractDir)
			if targetAbs != contractDirClean && !strings.HasPrefix(targetAbs, contractDirClean+string(filepath.Separator)) {
				continue
			}

			dstRel, _ := filepath.Rel(contractDirClean, targetAbs)
			dstID := nodeKey(dstRel, lang.SupportsFileGlobs())
			nodes[dstID] = dstID

			if srcID == dstID {
				continue
			}

			if edges[srcID] == nil {
				edges[srcID] = map[string]bool{}
			}
			edges[srcID][dstID] = true
		}

		return nil
	}

	err := service.WalkCapsule(fsys, contractDir, lang, walkFn)
	if err != nil {
		return nil, err
	}
	if len(fileRecords) > 0 {
		nodes, edges = mergeDirectoryNodes(fsys, fileRecords, p, lang, cfg)
	}
	if contractDir == p.Dir {
		if err := addBoundaryRelations(fsys, p, lang, contractDir, nodes, edges, cfg); err != nil {
			return nil, err
		}
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("capsule at %s has no scannable files to dump", contractDir)
	}

	g := graph.NewGraph(nodes, edges)
	if !lang.SupportsFileGlobs() {
		g.NodeDisplays = cloneNodes(nodes)
	}

	contractPath := filepath.Join(contractDir, port.ContractFile)
	content := repo.Save(g)
	if err := fsys.WriteFile(contractPath, []byte(content), 0o644); err != nil {
		return nil, err
	}

	return &ContractDump{
		FilesEncountered: filesEncountered,
		FilesScanned:     filesScanned,
		Nodes:            len(nodes),
		Edges:            edgeCount(edges),
		ContractPath:     contractPath,
	}, nil
}

func mergeDirectoryNodes(fsys port.FileSystem, records []fileRecord, p port.Capsule, lang port.Language, cfg draftConfig) (map[string]string, map[string]map[string]bool) {
	nodes := map[string]string{}
	edges := map[string]map[string]bool{}

	dirFiles := map[string][]fileRecord{}
	for _, r := range records {
		dir := filepath.Dir(r.rel)
		dirFiles[dir] = append(dirFiles[dir], r)
	}

	dirIsMerged := map[string]bool{}
	for dir, files := range dirFiles {
		if dir != "." && dir != "" && len(files) > 1 && !cfg.isExpandedDir(dir) {
			dirIsMerged[dir] = true
		}
	}

	contractDirClean := filepath.Clean(p.Dir)

	for dir, files := range dirFiles {
		if dir == "." || dir == "" {
			for _, f := range files {
				srcID := graph.NodeKeyForFile(f.rel)
				nodes[srcID] = srcID
				resolveFileImports(fsys, f.imports, f.rel, p, lang, contractDirClean, nodes, edges, srcID, dirIsMerged)
			}
			continue
		}

		if dirIsMerged[dir] {
			dirGlob := mergedDirGlob(dir)
			dirID := graph.NodeKeyForDir(dir)
			nodes[dirID] = dirGlob
			for _, f := range files {
				resolveFileImports(fsys, f.imports, f.rel, p, lang, contractDirClean, nodes, edges, dirID, dirIsMerged)
			}
			continue
		}

		for _, f := range files {
			srcID := graph.NodeKeyForFile(f.rel)
			nodes[srcID] = srcID
			resolveFileImports(fsys, f.imports, f.rel, p, lang, contractDirClean, nodes, edges, srcID, dirIsMerged)
		}
	}

	return nodes, edges
}

func resolveFileImports(fsys port.FileSystem, imports []port.ImportSpec, fileRel string, c port.Capsule, lang port.Language, contractDirClean string, nodes map[string]string, edges map[string]map[string]bool, srcID string, dirIsMerged map[string]bool) {
	for _, spec := range imports {
		targetPath, internal := lang.ResolveInternalTarget(fsys, spec, c, fileRel)
		if !internal {
			continue
		}
		targetAbs := targetPath
		if !filepath.IsAbs(targetAbs) {
			targetAbs = filepath.Join(c.Dir, targetAbs)
		}
		targetAbs = filepath.Clean(targetAbs)

		if !port.IsTargetVisible(fsys, targetAbs) {
			continue
		}

		if targetAbs != contractDirClean && !strings.HasPrefix(targetAbs, contractDirClean+string(filepath.Separator)) {
			continue
		}
		dstRel, _ := filepath.Rel(contractDirClean, targetAbs)

		dstDir := filepath.Dir(dstRel)
		var dstID string
		if dirIsMerged[dstDir] {
			dstID = graph.NodeKeyForDir(dstDir)
			nodes[dstID] = mergedDirGlob(dstDir)
		} else {
			dstID = nodeKey(dstRel, lang.SupportsFileGlobs())
			nodes[dstID] = dstID
		}

		if srcID == dstID {
			continue
		}

		if edges[srcID] == nil {
			edges[srcID] = map[string]bool{}
		}
		edges[srcID][dstID] = true
	}
}

func edgeCount(edges map[string]map[string]bool) int {
	n := 0
	for _, m := range edges {
		n += len(m)
	}
	return n
}

func nodeKey(path string, fileLevel bool) string {
	if fileLevel {
		return graph.NodeKeyForFile(path)
	}
	return graph.NodeKeyForDir(path)
}

func makeDumpError(label string, err error) DumpError {
	var loadErr *contractLoadError
	if errors.As(err, &loadErr) {
		return DumpError{Label: loadErr.contractPath, Err: loadErr}
	}
	return DumpError{Label: label, Err: err}
}

func isFreshDraftCycle(err error) bool {
	var loadErr *contractLoadError
	if !errors.As(err, &loadErr) {
		return false
	}
	return strings.Contains(loadErr.message, "cycle detected")
}

func amendContract(fsys port.FileSystem, rootDir string, capsule port.Capsule, lang port.Language, repo port.GraphRepository, contractPath string, cfg draftConfig) (*AmendDiff, error) {
	raw, err := fsys.ReadFile(contractPath)
	if err != nil {
		return nil, &contractLoadError{contractPath: contractPath, message: err.Error()}
	}
	current, err := repo.Load(string(raw))
	if err != nil {
		return nil, &contractLoadError{contractPath: contractPath, message: summarizeContractLoadError(err), cycleGroups: parseCycleGroups(err.Error())}
	}

	updated, diff, err := applyCheckAmendments(fsys, rootDir, capsule, lang, repo, contractPath, current, cfg)
	if err != nil {
		return nil, err
	}
	if diff == nil {
		return nil, nil
	}

	content := repo.Save(updated)
	if err := fsys.WriteFile(contractPath, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return diff, nil
}

func applyCheckAmendments(fsys port.FileSystem, rootDir string, capsule port.Capsule, lang port.Language, repo port.GraphRepository, contractPath string, current *graph.Graph, cfg draftConfig) (*graph.Graph, *AmendDiff, error) {
	res := runCheckForCapsule(fsys, rootDir, capsule, lang, repo)
	if res == nil || len(res.Violations) == 0 {
		return current, nil, nil
	}

	nodeCountBefore := len(current.Nodes)
	edgeCountBefore := 0
	for _, targets := range current.Edges {
		edgeCountBefore += len(targets)
	}

	nodes := cloneNodes(current.Nodes)
	displays := cloneNodeDisplays(current.NodeDisplays)
	edges := cloneEdges(current.Edges)
	classes := cloneClasses(current.Classes)
	changed := false

	for _, violation := range res.Violations {
		var violationChanged bool
		var err error
		switch violation.Rule {
		case "no-node":
			violationChanged, err = applyNoNodeViolation(nodes, fsys, capsule, contractPath, lang, violation, cfg)
		case "import-no-node":
			violationChanged, err = ensureEdgeForImport(nodes, edges, fsys, capsule, contractPath, lang, violation, cfg)
		case "import-not-allowed":
			violationChanged, err = ensureEdgeForImport(nodes, edges, fsys, capsule, contractPath, lang, violation, cfg)
		}
		if err != nil {
			return nil, nil, err
		}
		if violationChanged {
			changed = true
		}
	}

	if !changed {
		return current, nil, nil
	}

	updated := graph.NewGraph(nodes, edges)
	updated.NodeDisplays = displays
	if !lang.SupportsFileGlobs() {
		for id, glob := range updated.Nodes {
			if _, ok := updated.NodeDisplays[id]; !ok {
				updated.NodeDisplays[id] = glob
			}
		}
	}
	updated.Classes = classes

	diff := &AmendDiff{
		Nodes: len(nodes) - nodeCountBefore,
		Edges: 0,
	}
	for src, targets := range edges {
		if _, ok := current.Edges[src]; !ok {
			diff.Edges += len(targets)
		} else {
			for dst := range targets {
				if !current.Edges[src][dst] {
					diff.Edges++
				}
			}
		}
	}

	return updated, diff, nil
}

func runCheckForCapsule(fsys port.FileSystem, rootDir string, capsule port.Capsule, lang port.Language, repo port.GraphRepository) *port.CapsuleResult {
	discovery := service.NewCapsuleDiscovery()
	lang.Register(discovery)
	result := check.Run(fsys, rootDir, []port.Language{lang}, repo, discovery)
	if result == nil {
		return nil
	}
	label := port.Label(capsule)
	for _, capsuleRes := range result.Capsules {
		if capsuleRes.Label == label {
			res := capsuleRes
			return &res
		}
	}
	return nil
}

func ensureEdgeForImport(nodes map[string]string, edges map[string]map[string]bool, fsys port.FileSystem, capsule port.Capsule, contractPath string, lang port.Language, violation port.Violation, cfg draftConfig) (bool, error) {
	srcID, srcChanged, err := ensureAmendNodeForFile(nodes, fsys, capsule, contractPath, lang, violation.File)
	if err != nil {
		return false, err
	}
	dstID, dstChanged, err := ensureNodeForImportTarget(nodes, fsys, capsule, contractPath, lang, violation, cfg)
	if err != nil {
		return false, err
	}
	if srcID == "" || dstID == "" || srcID == dstID {
		return srcChanged || dstChanged, nil
	}
	if edges[srcID] == nil {
		edges[srcID] = map[string]bool{}
	}
	if edges[srcID][dstID] {
		return srcChanged || dstChanged, nil
	}
	edges[srcID][dstID] = true
	return true, nil
}

func applyNoNodeViolation(nodes map[string]string, fsys port.FileSystem, capsule port.Capsule, contractPath string, lang port.Language, violation port.Violation, cfg draftConfig) (bool, error) {
	contractDir := filepath.Dir(contractPath)
	if service.TrackingScope(fsys, violation.File, capsule.Dir) != contractDir {
		return false, nil
	}
	if _, err := lang.ParseImports(fsys, violation.File); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	_, changed, err := ensureAmendNodeForFile(nodes, fsys, capsule, contractPath, lang, violation.File)
	return changed, err
}

func ensureNodeForImportTarget(nodes map[string]string, fsys port.FileSystem, capsule port.Capsule, contractPath string, lang port.Language, violation port.Violation, cfg draftConfig) (string, bool, error) {
	spec, err := importSpecForViolation(lang, fsys, violation.File, violation.Line, violation.Column)
	if err != nil || spec == nil {
		return "", false, err
	}
	contractDir := filepath.Dir(contractPath)
	fileRel, err := filepath.Rel(capsule.Dir, violation.File)
	if err != nil {
		return "", false, err
	}
	targetPath, internal := lang.ResolveInternalTarget(fsys, *spec, capsule, filepath.ToSlash(fileRel))
	if !internal {
		return "", false, nil
	}
	targetAbs := targetPath
	if !filepath.IsAbs(targetAbs) {
		targetAbs = filepath.Join(capsule.Dir, targetAbs)
	}
	targetAbs = filepath.Clean(targetAbs)
	if targetAbs != contractDir && !strings.HasPrefix(targetAbs, contractDir+string(filepath.Separator)) {
		if scopeDir := service.TrackingScope(fsys, targetAbs, capsule.Dir); scopeDir != contractDir {
			return ensureDirNode(nodes, contractDir, scopeDir)
		}
	}
	return ensureAmendNodeForFile(nodes, fsys, capsule, contractPath, lang, targetAbs)
}

func ensureAmendNodeForFile(nodes map[string]string, fsys port.FileSystem, capsule port.Capsule, contractPath string, lang port.Language, absPath string) (string, bool, error) {
	contractDir := filepath.Dir(contractPath)
	scopeDir := service.TrackingScope(fsys, absPath, capsule.Dir)
	if scopeDir != contractDir {
		return ensureDirNode(nodes, contractDir, scopeDir)
	}

	rel, err := filepath.Rel(contractDir, absPath)
	if err != nil {
		return "", false, err
	}
	rel = filepath.ToSlash(rel)
	if existingID := existingOwningNodeForPath(nodes, rel); existingID != "" {
		return existingID, false, nil
	}
	if !lang.SupportsFileGlobs() {
		return ensureDirNode(nodes, contractDir, absPath)
	}
	return ensureExactNode(nodes, rel, rel)
}

func ensureNodeForFile(nodes map[string]string, fsys port.FileSystem, capsule port.Capsule, contractPath string, lang port.Language, absPath string, cfg draftConfig) (string, bool, error) {
	contractDir := filepath.Dir(contractPath)
	scopeDir := service.TrackingScope(fsys, absPath, capsule.Dir)
	if scopeDir != contractDir {
		return ensureDirNode(nodes, contractDir, scopeDir)
	}

	rel, err := filepath.Rel(contractDir, absPath)
	if err != nil {
		return "", false, err
	}
	rel = filepath.ToSlash(rel)
	if existingID := existingOwningNodeForPath(nodes, rel); existingID != "" {
		return existingID, false, nil
	}
	if !lang.SupportsFileGlobs() {
		return ensureDirNode(nodes, contractDir, absPath)
	}

	dir := filepath.Dir(rel)
	if shouldMergeNodeForFile(fsys, contractDir, capsule, lang, rel, cfg) && dir != "." && dir != "" {
		return ensureMergedDirNode(nodes, contractDir, filepath.Join(contractDir, filepath.FromSlash(dir)))
	}
	return ensureExactNode(nodes, rel, rel)
}

func defaultDraftConfig(capsule port.Capsule, lang port.Language, contractDir string) draftConfig {
	if contractDir == capsule.Dir && lang.SupportsFileGlobs() {
		return draftConfig{mode: draftModeMergedDirs}
	}
	return draftConfig{mode: draftModeExactFiles}
}

func (cfg draftConfig) withExpandedDirs(dirs ...string) draftConfig {
	if len(dirs) == 0 {
		return cfg
	}
	next := draftConfig{mode: cfg.mode}
	if len(cfg.expandedDirs) > 0 {
		next.expandedDirs = make(map[string]bool, len(cfg.expandedDirs)+len(dirs))
		for dir := range cfg.expandedDirs {
			next.expandedDirs[dir] = true
		}
	} else {
		next.expandedDirs = make(map[string]bool, len(dirs))
	}
	for _, dir := range dirs {
		if dir != "" {
			next.expandedDirs[dir] = true
		}
	}
	return next
}

func (cfg draftConfig) isExpandedDir(dir string) bool {
	return cfg.expandedDirs != nil && cfg.expandedDirs[dir]
}

func shouldMergeContractDir(contractDir string, capsule port.Capsule, lang port.Language, cfg draftConfig) bool {
	return cfg.mode == draftModeMergedDirs && contractDir == capsule.Dir && lang.SupportsFileGlobs()
}

func shouldMergeNodeForFile(fsys port.FileSystem, contractDir string, capsule port.Capsule, lang port.Language, rel string, cfg draftConfig) bool {
	if !shouldMergeContractDir(contractDir, capsule, lang, cfg) {
		return false
	}
	dir := filepath.Dir(rel)
	if dir == "." || dir == "" {
		return false
	}
	if cfg.isExpandedDir(dir) {
		return false
	}
	return shouldMergeDir(fsys, contractDir, dir, lang)
}

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

func mergedDirGlob(rel string) string {
	return rel + "/*.*"
}

func ensureMergedDirNode(nodes map[string]string, contractDir string, absPath string) (string, bool, error) {
	rel, err := filepath.Rel(contractDir, absPath)
	if err != nil {
		return "", false, err
	}
	rel = filepath.ToSlash(rel)
	id := graph.NodeKeyForDir(rel)
	return ensureExactNode(nodes, id, mergedDirGlob(id))
}

func ensureDirNode(nodes map[string]string, contractDir string, absPath string) (string, bool, error) {
	rel, err := filepath.Rel(contractDir, absPath)
	if err != nil {
		return "", false, err
	}
	rel = filepath.ToSlash(rel)
	id := graph.NodeKeyForDir(rel)
	glob := "."
	if id != "." {
		glob = id
	}
	return ensureExactNode(nodes, id, glob)
}

func ensureExactNode(nodes map[string]string, id, glob string) (string, bool, error) {
	if existing, ok := nodes[id]; ok {
		if existing == glob {
			return id, false, nil
		}
		return id, false, nil
	}
	for _, existingID := range existingNodeIDsForGlob(nodes, glob) {
		return existingID, false, nil
	}
	nodes[id] = glob
	return id, true, nil
}

func existingOwningNodeForPath(nodes map[string]string, rel string) string {
	if len(nodes) == 0 {
		return ""
	}
	return graph.NewGraph(nodes, nil).NodeForPath(rel)
}

func existingNodeIDsForGlob(nodes map[string]string, glob string) []string {
	ids := make([]string, 0, len(nodes))
	for id, existingGlob := range nodes {
		if existingGlob == glob {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func shouldMergeDir(fsys port.FileSystem, contractDir, relDir string, lang port.Language) bool {
	return scannableFileCount(fsys, contractDir, relDir, lang) > 1
}

func scannableFileCount(fsys port.FileSystem, contractDir, relDir string, lang port.Language) int {
	entries, err := fsys.ReadDir(filepath.Join(contractDir, filepath.FromSlash(relDir)))
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if lang.IsScannableFile(filepath.ToSlash(filepath.Join(relDir, entry.Name()))) {
			count++
		}
	}
	return count
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

func cloneNodes(nodes map[string]string) map[string]string {
	cloned := make(map[string]string, len(nodes))
	for id, glob := range nodes {
		cloned[id] = glob
	}
	return cloned
}

func cloneNodeDisplays(displays map[string]string) map[string]string {
	cloned := make(map[string]string, len(displays))
	for id, glob := range displays {
		cloned[id] = glob
	}
	return cloned
}

func cloneEdges(edges map[string]map[string]bool) map[string]map[string]bool {
	cloned := make(map[string]map[string]bool, len(edges))
	for src, dsts := range edges {
		cloned[src] = make(map[string]bool, len(dsts))
		for dst, allowed := range dsts {
			if allowed {
				cloned[src][dst] = true
			}
		}
	}
	return cloned
}

func cloneClasses(classes map[string]map[string]bool) map[string]map[string]bool {
	cloned := make(map[string]map[string]bool, len(classes))
	for nodeID, nodeClasses := range classes {
		cloned[nodeID] = make(map[string]bool, len(nodeClasses))
		for className, enabled := range nodeClasses {
			if enabled {
				cloned[nodeID][className] = true
			}
		}
	}
	return cloned
}

func addBoundaryRelations(fsys port.FileSystem, capsule port.Capsule, lang port.Language, contractDir string, nodes map[string]string, edges map[string]map[string]bool, cfg draftConfig) error {
	return fsys.WalkDir(contractDir, func(abs string, d os.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		fileRel, err := filepath.Rel(capsule.Dir, abs)
		if err != nil {
			return err
		}
		fileRel = filepath.ToSlash(fileRel)
		if !lang.IsScannableFile(fileRel) {
			return nil
		}

		imports, err := lang.ParseImports(fsys, abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		srcScope := service.TrackingScope(fsys, abs, capsule.Dir)
		for _, spec := range imports {
			targetPath, internal := lang.ResolveInternalTarget(fsys, spec, capsule, fileRel)
			if !internal {
				continue
			}
			targetAbs := targetPath
			if !filepath.IsAbs(targetAbs) {
				targetAbs = filepath.Join(capsule.Dir, targetAbs)
			}
			targetAbs = filepath.Clean(targetAbs)
			if !port.IsTargetVisible(fsys, targetAbs) {
				continue
			}

			dstScope := service.TrackingScope(fsys, targetAbs, capsule.Dir)
			if srcScope == dstScope {
				continue
			}

			srcID, _, err := boundaryNodeForDraft(nodes, fsys, capsule, contractDir, lang, abs, cfg)
			if err != nil {
				return err
			}
			dstID, _, err := boundaryNodeForDraft(nodes, fsys, capsule, contractDir, lang, targetAbs, cfg)
			if err != nil {
				return err
			}
			if srcID == "" || dstID == "" || srcID == dstID {
				continue
			}
			if edges[srcID] == nil {
				edges[srcID] = map[string]bool{}
			}
			edges[srcID][dstID] = true
		}
		return nil
	})
}

func boundaryNodeForDraft(nodes map[string]string, fsys port.FileSystem, capsule port.Capsule, contractDir string, lang port.Language, absPath string, cfg draftConfig) (string, bool, error) {
	scopeDir := service.TrackingScope(fsys, absPath, capsule.Dir)
	if scopeDir != contractDir {
		return ensureDirNode(nodes, contractDir, scopeDir)
	}
	return ensureNodeForFile(nodes, fsys, capsule, filepath.Join(contractDir, port.ContractFile), lang, absPath, cfg)
}

func discoverScopedContracts(fsys port.FileSystem, capsuleDir string) ([]string, error) {
	var contracts []string
	err := fsys.WalkDir(capsuleDir, func(abs string, d os.DirEntry) error {
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
