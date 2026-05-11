package draft

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	ConfigPath       string
}

// Draft walks all capsules for every supplied language, parses every
// import in every governed file, and writes a comprehensive BAFT.md
// that reflects the current dependency reality at maximum granularity.
func Run(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery) (*DraftResult, error) {
	return RunWith(fsys, rootDir, languages, repo, discovery, os.Stderr)
}

func RunWith(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery, logWriter io.Writer) (*DraftResult, error) {
	type entry struct {
		capsule port.Capsule
		lang    port.Language
	}
	var all []entry
	entries, err := discovery.Discover(fsys, rootDir)
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

	sort.Slice(all, func(i, j int) bool { return port.Label(all[i].capsule, rootDir) < port.Label(all[j].capsule, rootDir) })

	result := &DraftResult{}

	for _, e := range all {
		startDir := e.capsule.Dir
		if strings.HasPrefix(rootDir, e.capsule.Dir+string(filepath.Separator)) || rootDir == e.capsule.Dir {
			startDir = rootDir
		}
		configDir, exists := service.FindOrCreateConfigDir(fsys, startDir, e.capsule.Dir)
		if exists {
			continue
		}
		label := port.Label(e.capsule, rootDir)
		capsuleRes, err := draftCapsule(fsys, e.capsule, e.lang, repo, rootDir, configDir)
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

func draftCapsule(fsys port.FileSystem, p port.Capsule, lang port.Language, repo port.GraphRepository, rootDir string, configDir string) (*CapsuleDraft, error) {
	nodes := map[string]string{}
	edges := map[string]map[string]bool{}
	filesEncountered := 0
	filesScanned := 0

	err := service.WalkCapsule(fsys, configDir, lang, func(abs, rel string) error {
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
			fileRel, _ = filepath.Rel(p.Dir, filepath.Join(configDir, rel))
		}
		fileRel = filepath.ToSlash(fileRel)

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
			configDirClean := filepath.Clean(configDir)
			if targetAbs != configDirClean && !strings.HasPrefix(targetAbs, configDirClean+string(filepath.Separator)) {
				continue
			}

			dstRel, _ := filepath.Rel(configDirClean, targetAbs)
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
	})
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("capsule at %s has no governed files to draft", configDir)
	}

	g := graph.NewGraph(nodes, edges)

	configPath := filepath.Join(configDir, port.ConfigFile)
	content := repo.Save(g)
	if err := fsys.WriteFile(configPath, []byte(content), 0644); err != nil {
		return nil, err
	}

	return &CapsuleDraft{
		Label:            port.Label(p, rootDir),
		FilesEncountered: filesEncountered,
		FilesScanned:     filesScanned,
		Nodes:            len(nodes),
		Edges:            edgeCount(edges),
		ConfigPath:       configPath,
	}, nil
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
