package draft

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
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

// DraftResult holds the outcome of a draft run.
type DraftResult struct {
	Capsules []CapsuleDraft
	Errors   []DraftError
}

// DraftError records a non-fatal error encountered while drafting a capsule.
type DraftError struct {
	Label string
	Err   error
}

func (d DraftError) Error() string {
	return fmt.Sprintf("%s: %s", d.Label, d.Err)
}

// CapsuleDraft holds the outcome for a single capsule draft.
type CapsuleDraft struct {
	Label            string
	FilesEncountered int
	FilesScanned     int
	Nodes            int
	Edges            int
	ContractPath     string
}

// fileRecord holds import data for a single file during capsule-root drafting
// with file-glob languages, before directory-level merging.
type fileRecord struct {
	rel     string
	imports []port.ImportSpec
}

// Draft walks all capsules for every supplied language, parses every
// import in every scannable file, and writes a comprehensive BAFT.md
// that reflects the current dependency reality at maximum granularity.
func Run(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery) (*DraftResult, error) {
	return RunWith(fsys, rootDir, languages, repo, discovery, os.Stderr)
}

func RunWith(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery, logWriter io.Writer) (*DraftResult, error) {
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

	sort.Slice(all, func(i, j int) bool { return port.Label(all[i].capsule) < port.Label(all[j].capsule) })

	result := &DraftResult{}

	for _, e := range all {
		startDir := e.capsule.Dir
		if strings.HasPrefix(rootDir, e.capsule.Dir+string(filepath.Separator)) || rootDir == e.capsule.Dir {
			startDir = rootDir
		}
		contractDir, exists := service.FindOrCreateContractDir(wrapped, startDir, e.capsule.Dir)
		if exists {
			continue
		}
		label := port.Label(e.capsule)
		capsuleRes, err := draftCapsule(wrapped, e.capsule, e.lang, repo, rootDir, contractDir)
		if err != nil {
			de := DraftError{Label: label, Err: err}
			result.Errors = append(result.Errors, de)
			fmt.Fprintf(logWriter, "draft: %s: %s\n", label, err)
			continue
		}
		result.Capsules = append(result.Capsules, *capsuleRes)
	}

	return result, nil
}

func draftCapsule(fsys port.FileSystem, p port.Capsule, lang port.Language, repo port.GraphRepository, rootDir string, contractDir string) (*CapsuleDraft, error) {
	nodes := map[string]string{}
	edges := map[string]map[string]bool{}
	filesEncountered := 0
	filesScanned := 0

	// For capsule-root drafts with file-glob languages, collect per-file data
	// so we can merge same-directory files into a single directory-level node.
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

		// When drafting from capsule root with a file-glob language,
		// collect per-file records for later merging.
		if contractDir == p.Dir && lang.SupportsFileGlobs() {
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

	var err error
	if contractDir == p.Dir {
		err = service.WalkAllFiles(fsys, contractDir, lang, walkFn)
	} else {
		err = service.WalkCapsule(fsys, contractDir, lang, walkFn)
	}
	if err != nil {
		return nil, err
	}

	// If we collected file records (capsule-root + file-glob language),
	// merge same-directory files into directory-level nodes.
	if len(fileRecords) > 0 {
		nodes, edges = mergeDirectoryNodes(fsys, fileRecords, p, lang)
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("capsule at %s has no scannable files to draft", contractDir)
	}

	g := graph.NewGraph(nodes, edges)

	contractPath := filepath.Join(contractDir, port.ContractFile)
	content := repo.Save(g)
	if err := fsys.WriteFile(contractPath, []byte(content), 0o644); err != nil {
		return nil, err
	}

	return &CapsuleDraft{
		Label:            port.Label(p),
		FilesEncountered: filesEncountered,
		FilesScanned:     filesScanned,
		Nodes:            len(nodes),
		Edges:            edgeCount(edges),
		ContractPath:     contractPath,
	}, nil
}

// mergeDirectoryNodes takes per-file records and merges files that share
// the same parent directory into a single directory-level node with a /**
// glob.  Files whose directory is the capsule root itself stay as individual
// file nodes.  Outgoing edges from merged files are promoted to the
// directory-level node.
func mergeDirectoryNodes(fsys port.FileSystem, records []fileRecord, p port.Capsule, lang port.Language) (map[string]string, map[string]map[string]bool) {
	nodes := map[string]string{}
	edges := map[string]map[string]bool{}

	// Group file records by parent directory.
	dirFiles := map[string][]fileRecord{}
	for _, r := range records {
		dir := filepath.Dir(r.rel)
		dirFiles[dir] = append(dirFiles[dir], r)
	}

	// Build a map from directory to whether it has multiple files (should be merged).
	dirIsMerged := map[string]bool{}
	for dir, files := range dirFiles {
		if dir != "." && dir != "" && len(files) > 1 {
			dirIsMerged[dir] = true
		}
	}

	contractDirClean := filepath.Clean(p.Dir)

	for dir, files := range dirFiles {
		// If the directory is the capsule root, keep files as individual nodes.
		if dir == "." || dir == "" {
			for _, f := range files {
				srcID := graph.NodeKeyForFile(f.rel)
				nodes[srcID] = srcID
				resolveFileImports(fsys, f.imports, f.rel, p, lang, contractDirClean, nodes, edges, srcID, dirIsMerged)
			}
			continue
		}

		// Multiple files in the same subdirectory → merge into one directory node.
		if len(files) > 1 {
			dirGlob := dir + "/**"
			dirID := graph.NodeKeyForDir(dir)
			nodes[dirID] = dirGlob
			for _, f := range files {
				resolveFileImports(fsys, f.imports, f.rel, p, lang, contractDirClean, nodes, edges, dirID, dirIsMerged)
			}
			continue
		}

		// Single file in subdirectory → keep as individual file node.
		f := files[0]
		srcID := graph.NodeKeyForFile(f.rel)
		nodes[srcID] = srcID
		resolveFileImports(fsys, f.imports, f.rel, p, lang, contractDirClean, nodes, edges, srcID, dirIsMerged)
	}

	return nodes, edges
}

// resolveFileImports resolves each import spec for a file and adds nodes/edges.
// dirIsMerged maps directories whose files are merged into a single directory-level node.
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

		// Skip ignored/baftignored targets.
		if !port.IsTargetVisible(fsys, targetAbs) {
			continue
		}

		if targetAbs != contractDirClean && !strings.HasPrefix(targetAbs, contractDirClean+string(filepath.Separator)) {
			continue
		}
		dstRel, _ := filepath.Rel(contractDirClean, targetAbs)

		// If the target file is in a merged directory, use the directory node ID.
		dstDir := filepath.Dir(dstRel)
		var dstID string
		if dirIsMerged[dstDir] {
			dstID = graph.NodeKeyForDir(dstDir)
		} else {
			dstID = nodeKey(dstRel, lang.SupportsFileGlobs())
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
