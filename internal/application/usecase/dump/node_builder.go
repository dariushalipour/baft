package dump

import (
	"path/filepath"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
)

// ensureNodeForFile returns the node that owns absPath, creating it when the
// contract has none. mergeDirs is set while drafting a fresh contract, where a
// whole directory may collapse into one node; amendments never widen an
// existing contract that way.
func ensureNodeForFile(nodes map[string]string, cc capsuleCtx, contractDir string, absPath string, cfg draftConfig, mergeDirs bool) (string, error) {
	if scopeDir := service.TrackingScope(cc.fsys, absPath, cc.capsule.Dir); scopeDir != contractDir {
		return ensureDirNode(nodes, contractDir, scopeDir)
	}

	if cfg.namespaceMode {
		if ns, err := cc.lang.GetFileNamespace(cc.fsys, absPath); err == nil && ns != "" {
			return ensureExactNode(nodes, ns, ns), nil
		}
	}

	rel, err := filepath.Rel(contractDir, absPath)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if existingID := existingOwningNodeForPath(nodes, rel); existingID != "" {
		return existingID, nil
	}

	if !cc.lang.SupportsFileGlobs() {
		dirPath := absPath
		if _, statErr := cc.fsys.Stat(absPath); statErr != nil {
			dirPath = filepath.Dir(absPath)
		}
		return ensureDirNode(nodes, contractDir, dirPath)
	}

	dir := filepath.Dir(rel)
	if mergeDirs && dir != "." && dir != "" && shouldMergeNodeForFile(cc.fsys, contractDir, cc.capsule, cc.lang, rel, cfg) {
		return ensureMergedDirNode(nodes, contractDir, filepath.Join(contractDir, filepath.FromSlash(dir)))
	}
	return ensureExactNode(nodes, rel, rel), nil
}

func ensureExactNode(nodes map[string]string, id, glob string) string {
	if _, ok := nodes[id]; ok {
		return id
	}
	if existingID := existingNodeIDForGlob(nodes, glob); existingID != "" {
		return existingID
	}
	nodes[id] = glob
	return id
}

func ensureDirNode(nodes map[string]string, contractDir string, absPath string) (string, error) {
	id, err := relNodeID(contractDir, absPath)
	if err != nil {
		return "", err
	}
	glob := "."
	if id != "." {
		glob = id
	}
	return ensureExactNode(nodes, id, glob), nil
}

func ensureMergedDirNode(nodes map[string]string, contractDir string, absPath string) (string, error) {
	id, err := relNodeID(contractDir, absPath)
	if err != nil {
		return "", err
	}
	return ensureExactNode(nodes, id, mergedDirGlob(id)), nil
}

func relNodeID(contractDir, absPath string) (string, error) {
	rel, err := filepath.Rel(contractDir, absPath)
	if err != nil {
		return "", err
	}
	return graph.NodeKeyForDir(filepath.ToSlash(rel)), nil
}

func existingOwningNodeForPath(nodes map[string]string, rel string) string {
	if len(nodes) == 0 {
		return ""
	}
	return graph.NewGraphIndex(graph.NewGraph(nodes, nil, nil, nil)).NodeForPath(rel)
}

func existingNodeIDForGlob(nodes map[string]string, glob string) string {
	best := ""
	for id, existing := range nodes {
		if existing == glob && (best == "" || id < best) {
			best = id
		}
	}
	return best
}
