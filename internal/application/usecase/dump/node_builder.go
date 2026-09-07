package dump

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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

// ensureExactNode returns the node claiming glob, creating one under a free
// draft key when the contract has none. Draft keys are path-shaped; finalNodeIDs
// turns them into the ids the contract file carries.
func ensureExactNode(nodes map[string]string, id, glob string) string {
	if existing, ok := nodes[id]; ok && existing == glob {
		return id
	}
	if existingID := existingNodeIDForGlob(nodes, glob); existingID != "" {
		return existingID
	}
	key := id
	for n := 2; ; n++ {
		if _, taken := nodes[key]; !taken {
			break
		}
		key = id + "~" + strconv.Itoa(n)
	}
	nodes[key] = glob
	return key
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

// nameGraphNodes replaces the draft keys a dump builds nodes under with the ids
// the contract file carries, everywhere they are referenced.
func nameGraphNodes(g *graph.Graph, kept map[string]bool) {
	renames := finalNodeIDs(g.Nodes, kept)
	if len(renames) == 0 {
		return
	}
	g.Nodes = renamedNodeKeys(g.Nodes, renames)
	g.NodeDisplays = renamedNodeKeys(g.NodeDisplays, renames)
	g.Classes = renamedNodeKeys(g.Classes, renames)
	g.Edges = renamedEdges(g.Edges, renames)
	g.NodeOrder = renamedIDs(g.NodeOrder, renames)
	g.EdgeOrder = renamedEdgeKeys(g.EdgeOrder, renames)
	tolerated := make(map[string]bool, len(g.Tolerated))
	for key, dotted := range g.Tolerated {
		tolerated[renamedEdgeKey(key, renames)] = dotted
	}
	g.Tolerated = tolerated
}

// finalNodeIDs maps draft node keys onto the ids the contract file carries. Ids
// in kept were written by the user and never move. Every other node is named
// after the last segment of its path when that segment is an unambiguous
// identifier, and after graph.NodeID of the whole path otherwise — so dumped ids
// read like the code and no two nodes can ever collide on one id.
func finalNodeIDs(nodes map[string]string, kept map[string]bool) map[string]string {
	taken := make(map[string]bool, len(nodes))
	segments := make(map[string]int, len(nodes))
	drafts := make([]string, 0, len(nodes))
	for id := range nodes {
		if kept[id] {
			taken[id] = true
			continue
		}
		drafts = append(drafts, id)
		segments[lastPathSegment(id)]++
	}
	sort.Strings(drafts)

	renames := make(map[string]string, len(drafts))
	for _, draft := range drafts {
		id := lastPathSegment(draft)
		if segments[id] > 1 || !graph.IsNodeID(id) {
			id = graph.NodeID(draft)
		}
		for base, n := id, 2; taken[id]; n++ {
			id = base + "_" + strconv.Itoa(n)
		}
		taken[id] = true
		renames[draft] = id
	}
	return renames
}

func lastPathSegment(id string) string {
	return id[strings.LastIndexByte(id, '/')+1:]
}

func finalID(renames map[string]string, id string) string {
	if final, ok := renames[id]; ok {
		return final
	}
	return id
}

// renamedNodeKeys re-keys a node-indexed map by the final node ids.
func renamedNodeKeys[V any](m map[string]V, renames map[string]string) map[string]V {
	renamed := make(map[string]V, len(m))
	for id, value := range m {
		renamed[finalID(renames, id)] = value
	}
	return renamed
}

func renamedIDs(ids []string, renames map[string]string) []string {
	renamed := make([]string, len(ids))
	for i, id := range ids {
		renamed[i] = finalID(renames, id)
	}
	return renamed
}

func renamedEdgeKeys(keys []string, renames map[string]string) []string {
	renamed := make([]string, len(keys))
	for i, key := range keys {
		renamed[i] = renamedEdgeKey(key, renames)
	}
	return renamed
}

func renamedEdgeKey(key string, renames map[string]string) string {
	src, dst, ok := strings.Cut(key, "\t")
	if !ok {
		return key
	}
	return graph.EdgeKey(finalID(renames, src), finalID(renames, dst))
}

func renamedEdges(edges map[string]map[string]bool, renames map[string]string) map[string]map[string]bool {
	renamed := make(map[string]map[string]bool, len(edges))
	for src, dsts := range edges {
		targets := make(map[string]bool, len(dsts))
		for dst := range dsts {
			targets[finalID(renames, dst)] = true
		}
		renamed[finalID(renames, src)] = targets
	}
	return renamed
}
