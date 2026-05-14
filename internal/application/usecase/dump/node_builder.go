package dump

import (
	"path/filepath"
	"sort"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

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

func ensureMergedDirNode(nodes map[string]string, contractDir string, absPath string) (string, bool, error) {
	rel, err := filepath.Rel(contractDir, absPath)
	if err != nil {
		return "", false, err
	}
	rel = filepath.ToSlash(rel)
	id := graph.NodeKeyForDir(rel)
	return ensureExactNode(nodes, id, mergedDirGlob(id))
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

func boundaryNodeForDraft(nodes map[string]string, fsys port.FileSystem, capsule port.Capsule, contractDir string, lang port.Language, absPath string, cfg draftConfig) (string, bool, error) {
	scopeDir := service.TrackingScope(fsys, absPath, capsule.Dir)
	if scopeDir != contractDir {
		return ensureDirNode(nodes, contractDir, scopeDir)
	}
	return ensureNodeForFile(nodes, fsys, capsule, filepath.Join(contractDir, port.ContractFile), lang, absPath, cfg)
}
