package dump

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

type fileImport struct {
	abs     string
	rel     string
	imports []port.ImportSpec
}

func resolveTargetNodeKey(fsys port.FileSystem, absPath string, rel string, lang port.Language) string {
	if lang.SupportsFileGlobs() {
		return nodeKey(rel, true)
	}
	info, err := fsys.Stat(absPath)
	if err == nil && info.IsDir() {
		return nodeKey(rel, false)
	}
	dirPath := filepath.Dir(rel)
	dirAbs := filepath.Dir(absPath)
	dirInfo, dirErr := fsys.Stat(dirAbs)
	if dirErr == nil && dirInfo.IsDir() {
		return nodeKey(filepath.ToSlash(dirPath), false)
	}
	return nodeKey(rel, false)
}

func dumpCapsule(cc capsuleCtx, contractDir string, cfg draftConfig) (*ContractDump, error) {
	fsys, p, lang := cc.fsys, cc.capsule, cc.lang
	nodes := map[string]string{}
	edges := map[string]map[string]bool{}
	filesEncountered := 0
	filesScanned := 0
	var fileRecords []fileRecord
	var allFiles []fileImport

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
		allFiles = append(allFiles, fileImport{abs: abs, rel: rel, imports: imports})
		return nil
	}

	err := service.WalkCapsule(context.Background(), fsys, contractDir, lang, walkFn)
	if err != nil {
		return nil, err
	}

	if len(allFiles) == 0 {
		return nil, fmt.Errorf("capsule at %s has no scannable files to dump", contractDir)
	}

	isNamespaceMode := cfg.namespaceMode
	var namespaceMap map[string]string
	if isNamespaceMode {
		namespaceMap = buildNamespaceMap(fsys, allFiles, lang)
	}

	for _, fi := range allFiles {
		fileRel := fi.rel
		if !filepath.IsAbs(fileRel) {
			fileRel, _ = filepath.Rel(p.Dir, filepath.Join(contractDir, fileRel))
		}
		fileRel = filepath.ToSlash(fileRel)

		if shouldMergeContractDir(contractDir, p, lang, cfg) {
			fileRecords = append(fileRecords, fileRecord{rel: fileRel, imports: fi.imports})
			continue
		}

		var srcID string
		if isNamespaceMode {
			if ns, ok := namespaceMap[fi.abs]; ok {
				srcID = ns
			} else {
				continue
			}
		} else {
			srcID = nodeKey(fi.rel, lang.SupportsFileGlobs())
		}
		nodes[srcID] = srcID

		for _, spec := range fi.imports {
			var dstID string
			if isNamespaceMode {
				if spec.Namespace == "" {
					continue
				}
				targetAbs, ok := resolveTargetByNamespace(fsys, spec, p, fileRel, lang)
				if !ok {
					continue
				}
				if !port.IsTargetVisible(fsys, targetAbs) {
					continue
				}
				dstID = spec.Namespace
			} else {
				targetPath, internal := lang.ResolveInternalTarget(fsys, spec, p, fileRel)
				if !internal {
					continue
				}
				targetAbs := targetPath
				if !filepath.IsAbs(targetAbs) {
					targetAbs = filepath.Join(p.Dir, targetAbs)
				}
				targetAbs = filepath.Clean(targetAbs)
				if !port.IsTargetVisible(fsys, targetAbs) {
					continue
				}
				contractDirClean := filepath.Clean(contractDir)
				if targetAbs != contractDirClean && !strings.HasPrefix(targetAbs, contractDirClean+string(filepath.Separator)) {
					continue
				}
				dstRel, _ := filepath.Rel(contractDirClean, targetAbs)
				dstID = resolveTargetNodeKey(fsys, targetAbs, dstRel, lang)
			}

			if srcID == dstID {
				continue
			}

			nodes[dstID] = dstID
			if edges[srcID] == nil {
				edges[srcID] = map[string]bool{}
			}
			edges[srcID][dstID] = true
		}
	}

	if len(fileRecords) > 0 {
		nodes, edges = mergeDirectoryNodes(fsys, fileRecords, p, lang, cfg)
	}
	if contractDir == p.Dir {
		if err := addBoundaryRelations(cc, contractDir, nodes, edges, cfg, namespaceMap); err != nil {
			return nil, err
		}
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("capsule at %s has no scannable files to dump", contractDir)
	}

	g := graph.NewGraph(nodes, edges, nil, nil)
	g.NamespaceMode = isNamespaceMode
	if !lang.SupportsFileGlobs() {
		if isNamespaceMode {
			displays := make(map[string]string, len(nodes))
			for ns := range nodes {
				parts := strings.Split(ns, ".")
				displays[ns] = parts[len(parts)-1]
			}
			g.NodeDisplays = displays
		} else {
			g.NodeDisplays = cloneNodes(nodes)
		}
	}

	contractPath := filepath.Join(contractDir, port.ContractFile)
	content := cc.repo.Save(g, cfg.saveOpts)
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

func buildNamespaceMap(fsys port.FileSystem, files []fileImport, lang port.Language) map[string]string {
	m := make(map[string]string)
	for _, fi := range files {
		ns, err := lang.GetFileNamespace(fsys, fi.abs)
		if err == nil && ns != "" {
			m[fi.abs] = ns
		}
	}
	return m
}

func resolveTargetByNamespace(fsys port.FileSystem, spec port.ImportSpec, c port.Capsule, fileRel string, lang port.Language) (string, bool) {
	targetPath, internal := lang.ResolveInternalTarget(fsys, spec, c, fileRel)
	if !internal {
		return "", false
	}
	targetAbs := targetPath
	if !filepath.IsAbs(targetAbs) {
		targetAbs = filepath.Join(c.Dir, targetAbs)
	}
	return filepath.Clean(targetAbs), true
}

func addBoundaryRelations(cc capsuleCtx, contractDir string, nodes map[string]string, edges map[string]map[string]bool, cfg draftConfig, namespaceMap map[string]string) error {
	fsys, capsule, lang := cc.fsys, cc.capsule, cc.lang
	return fsys.WalkDir(context.Background(), contractDir, func(abs string, d os.DirEntry) error {
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

		// Get source namespace for namespace mode (from pre-built map to avoid re-reading files)
		var srcNS string
		if cfg.namespaceMode {
			srcNS = namespaceMap[abs]
			if srcNS == "" {
				return nil
			}
		}

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

			var srcID, dstID string
			if cfg.namespaceMode {
				if spec.Namespace == "" {
					continue
				}
				srcID = srcNS
				// Use the resolved target file's actual namespace, not the raw import string.
				// This correctly handles cases where the import's namespace differs from the
				// target file's declared namespace (e.g., alias imports).
				if targetNS, ok := namespaceMap[targetAbs]; ok {
					dstID = targetNS
				} else {
					continue
				}
			} else {
				if srcID, err = ensureNodeForFile(nodes, cc, contractDir, abs, cfg, true); err != nil {
					return err
				}
				if dstID, err = ensureNodeForFile(nodes, cc, contractDir, targetAbs, cfg, true); err != nil {
					return err
				}
			}
			if srcID == "" || dstID == "" || srcID == dstID {
				continue
			}
			nodes[srcID] = srcID
			nodes[dstID] = dstID
			if edges[srcID] == nil {
				edges[srcID] = map[string]bool{}
			}
			edges[srcID][dstID] = true
		}
		return nil
	})
}
