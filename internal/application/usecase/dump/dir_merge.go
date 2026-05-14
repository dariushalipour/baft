package dump

import (
	"path/filepath"

	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

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

		if targetAbs != contractDirClean && !startsWith(targetAbs, contractDirClean+string(filepath.Separator)) {
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

func mergedDirGlob(rel string) string {
	return rel + "/*.*"
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


