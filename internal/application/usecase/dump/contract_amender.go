package dump

import (
	"path/filepath"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/application/usecase/check"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

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
		if fsys != nil {
			// Use os.IsNotExist since we can't call the interface's Stat directly here
			// but we know the error came from ParseImports which wraps os.IsNotExist
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
	if targetAbs != contractDir && !startsWith(targetAbs, contractDir+string(filepath.Separator)) {
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

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
