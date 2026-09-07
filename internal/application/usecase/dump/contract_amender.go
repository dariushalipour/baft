package dump

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/application/usecase/check"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

// amendContract widens an existing contract so that it allows the imports the
// code actually has. It returns nil when nothing had to be added.
func amendContract(cc capsuleCtx, contractPath string, current *graph.Graph, cfg draftConfig) (*AmendDiff, error) {
	validation := check.ValidateContract(cc.fsys, cc.lang, contractPath, current)
	if len(validation.Errors) > 0 {
		return nil, &contractError{
			contractPath: contractPath,
			kind:         contractValidationKind(validation.Errors),
			message:      summarizeContractValidationErrors(validation.Errors),
			cycleGroups:  validation.Cycles,
		}
	}

	updated, diff, err := applyCheckAmendments(cc, contractPath, current, cfg)
	if err != nil || diff == nil {
		return nil, err
	}
	content := cc.repo.Save(updated, cfg.saveOpts)
	if err := cc.fsys.WriteFile(contractPath, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return diff, nil
}

func applyCheckAmendments(cc capsuleCtx, contractPath string, current *graph.Graph, cfg draftConfig) (*graph.Graph, *AmendDiff, error) {
	res, err := check.RunCapsule(cc.fsys, cc.rootDir, cc.capsule, cc.lang, cc.repo, cc.nestedDirs)
	if err != nil || res == nil || len(res.Violations) == 0 {
		return nil, nil, err
	}

	contractDir := filepath.Dir(contractPath)
	nodes := cloneNodes(current.Nodes)
	edges := cloneEdges(current.Edges)

	for _, violation := range res.Violations {
		switch violation.Rule {
		case "no-node":
			err = applyNoNodeViolation(nodes, cc, contractDir, violation, cfg)
		case "import-no-node", "import-not-allowed":
			err = ensureEdgeForImport(nodes, edges, cc, contractDir, violation, cfg)
		}
		if err != nil {
			return nil, nil, err
		}
	}

	diff := amendDiff(current, nodes, edges)
	if diff == nil {
		return nil, nil, nil
	}

	updated := graph.NewGraph(nodes, edges, appendOrder(current.NodeOrder, nodeKeys(nodes)), appendOrder(current.EdgeOrder, edgeKeys(edges)))
	updated.NodeDisplays = cloneNodes(current.NodeDisplays)
	updated.NamespaceMode = current.NamespaceMode
	updated.GlobSeparator = current.GlobSeparator
	updated.Classes = cloneClasses(current.Classes)
	updated.Tolerated = current.Tolerated
	if !cc.lang.SupportsFileGlobs() {
		for id, glob := range updated.Nodes {
			if _, ok := updated.NodeDisplays[id]; !ok {
				updated.NodeDisplays[id] = glob
			}
		}
	}
	nameGraphNodes(updated, keySet(current.Nodes))
	return updated, diff, nil
}

// amendDiff reports what the amendment added, or nil when it added nothing. It
// names nodes by their glob, which is what the user reads, not by the mermaid
// identifier the contract file carries.
func amendDiff(current *graph.Graph, nodes map[string]string, edges map[string]map[string]bool) *AmendDiff {
	var diff AmendDiff
	for id, glob := range nodes {
		if _, ok := current.Nodes[id]; !ok {
			diff.Nodes = append(diff.Nodes, glob)
		}
	}
	for src, dsts := range edges {
		for dst := range dsts {
			if !current.Edges[src][dst] {
				diff.Edges = append(diff.Edges, nodes[src]+" --> "+nodes[dst])
			}
		}
	}
	if len(diff.Nodes) == 0 && len(diff.Edges) == 0 {
		return nil
	}
	sort.Strings(diff.Nodes)
	sort.Strings(diff.Edges)
	return &diff
}

func ensureEdgeForImport(nodes map[string]string, edges map[string]map[string]bool, cc capsuleCtx, contractDir string, violation port.Violation, cfg draftConfig) error {
	srcID, err := ensureNodeForFile(nodes, cc, contractDir, violation.File, cfg, false)
	if err != nil {
		return err
	}
	dstID, err := ensureNodeForImportTarget(nodes, cc, contractDir, violation, cfg)
	if err != nil {
		return err
	}
	if srcID == "" || dstID == "" || srcID == dstID {
		return nil
	}
	if edges[srcID] == nil {
		edges[srcID] = map[string]bool{}
	}
	edges[srcID][dstID] = true
	return nil
}

func applyNoNodeViolation(nodes map[string]string, cc capsuleCtx, contractDir string, violation port.Violation, cfg draftConfig) error {
	if service.TrackingScope(cc.fsys, violation.File, cc.capsule.Dir) != contractDir {
		return nil
	}
	if _, err := cc.lang.ParseImports(cc.fsys, violation.File); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_, err := ensureNodeForFile(nodes, cc, contractDir, violation.File, cfg, false)
	return err
}

func ensureNodeForImportTarget(nodes map[string]string, cc capsuleCtx, contractDir string, violation port.Violation, cfg draftConfig) (string, error) {
	spec, err := importSpecForViolation(cc.lang, cc.fsys, violation.File, violation.Line, violation.Column)
	if err != nil || spec == nil {
		return "", err
	}
	fileRel, err := filepath.Rel(cc.capsule.Dir, violation.File)
	if err != nil {
		return "", err
	}
	targetPath, internal := cc.lang.ResolveInternalTarget(cc.fsys, *spec, cc.capsule, filepath.ToSlash(fileRel))
	if !internal {
		return "", nil
	}
	targetAbs := targetPath
	if !filepath.IsAbs(targetAbs) {
		targetAbs = filepath.Join(cc.capsule.Dir, targetAbs)
	}
	targetAbs = filepath.Clean(targetAbs)
	if targetAbs != contractDir && !strings.HasPrefix(targetAbs, contractDir+string(filepath.Separator)) {
		if scopeDir := service.TrackingScope(cc.fsys, targetAbs, cc.capsule.Dir); scopeDir != contractDir {
			return ensureDirNode(nodes, contractDir, scopeDir)
		}
	}
	if cfg.namespaceMode && spec.Namespace != "" {
		return ensureExactNode(nodes, spec.Namespace, spec.Namespace), nil
	}
	return ensureNodeForFile(nodes, cc, contractDir, targetAbs, cfg, false)
}

// appendOrder keeps the user's declaration order for everything that survived
// and appends whatever the amendment added, sorted.
func appendOrder(existing []string, keys []string) []string {
	present := make(map[string]bool, len(keys))
	for _, key := range keys {
		present[key] = true
	}
	kept := make(map[string]bool, len(existing))
	order := make([]string, 0, len(keys))
	for _, key := range existing {
		if present[key] && !kept[key] {
			kept[key] = true
			order = append(order, key)
		}
	}
	added := make([]string, 0, len(keys)-len(order))
	for _, key := range keys {
		if !kept[key] {
			added = append(added, key)
		}
	}
	sort.Strings(added)
	return append(order, added...)
}

func keySet(nodes map[string]string) map[string]bool {
	keys := make(map[string]bool, len(nodes))
	for id := range nodes {
		keys[id] = true
	}
	return keys
}

func nodeKeys(nodes map[string]string) []string {
	keys := make([]string, 0, len(nodes))
	for id := range nodes {
		keys = append(keys, id)
	}
	return keys
}

func edgeKeys(edges map[string]map[string]bool) []string {
	keys := make([]string, 0, len(edges))
	for src, dsts := range edges {
		for dst := range dsts {
			keys = append(keys, src+"\t"+dst)
		}
	}
	return keys
}
