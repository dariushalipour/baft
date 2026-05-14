package dump

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

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
	content := repo.Save(g, cfg.saveOpts)
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
