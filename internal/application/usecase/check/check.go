// Package check verifies that code imports respect architecture rules
// declared in BAFT.md config files across one or more capsules.
//
// Algorithm:
//
//  1. Discover capsules (modules with manifest files like go.mod)
//  2. For each capsule, find the root BAFT.md and any scoped configs
//  3. Walk every governed file, resolve its imports to internal targets
//  4. For each import, determine the governing scope and check the
//     relation against the appropriate graph, walking up ancestor scopes
//     when source and target are in different scopes
//  5. Validate that all graphs use only node types supported by the language
//  6. Aggregate violations and errors into a CheckResult
package check

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

type capsuleResult struct {
	graph                 *graph.Graph
	filesEncountered      int
	filesScanned          int
	relations             int
	violations            []port.Violation
	errors                []port.Violation
	hasOverlapError       bool
	hasDuplicateGlobError bool
}

func (r *capsuleResult) toPublic(label string) port.CapsuleResult {
	cr := port.CapsuleResult{
		Label:            label,
		FilesEncountered: r.filesEncountered,
		FilesScanned:     r.filesScanned,
		Relations:        r.relations,
		Violations:       r.violations,
		Errors:           r.errors,
	}
	if r.graph != nil {
		cr.Nodes = len(r.graph.Nodes)
		cr.Edges = r.graph.EdgeCount()
	}
	return cr
}

type configContext struct {
	rootGraph     *graph.Graph
	hasRootConfig bool
	configPathAbs string
	loadErr       *port.Violation
}

type capsuleChecker struct {
	res               *capsuleResult
	fsys              port.FileSystem
	capsule           port.Capsule
	lang              port.Language
	configDir         string
	configDirAbs      string
	scopeCache        *scopeCache
	nestedCapsuleDirs []string
	configContext
}

func newCapsuleChecker(
	fsys port.FileSystem,
	capsule port.Capsule,
	lang port.Language,
	repo port.GraphRepository,
	configDir string,
	ctx configContext,
	nestedCapsuleDirs []string,
) *capsuleChecker {
	configDirAbs, _ := filepath.Abs(configDir)
	return &capsuleChecker{
		res:               &capsuleResult{graph: ctx.rootGraph},
		fsys:              fsys,
		capsule:           capsule,
		lang:              lang,
		configDir:         configDir,
		configDirAbs:      configDirAbs,
		scopeCache:        newScopeCache(fsys, repo),
		nestedCapsuleDirs: nestedCapsuleDirs,
		configContext:     ctx,
	}
}

type capsuleEntry struct {
	capsule port.Capsule
	lang    port.Language
}

func Run(fsys port.FileSystem, rootDir string, languages []port.Language, repo port.GraphRepository, discovery *service.CapsuleDiscovery) *port.CheckResult {
	entries, err := discovery.Discover(fsys, rootDir)
	if err != nil {
		return &port.CheckResult{Errors: []string{"discovery: " + err.Error()}}
	}

	capsules := matchCapsulesToLanguages(entries, languages)
	if len(capsules) == 0 {
		return &port.CheckResult{}
	}

	sort.Slice(capsules, func(i, j int) bool {
		return port.Label(capsules[i].capsule, rootDir) < port.Label(capsules[j].capsule, rootDir)
	})

	var result port.CheckResult
	seenConfigErrors := map[string]bool{}
	for _, c := range capsules {
		label := port.Label(c.capsule, rootDir)
		nestedDirs := nestedCapsuleDirs(capsules, c.capsule.Dir)
		capsuleRes, err := checkCapsule(fsys, c.capsule, c.lang, repo, rootDir, nestedDirs)
		if err != nil {
			result.Errors = append(result.Errors, label+": "+err.Error())
			result.Capsules = append(result.Capsules, port.CapsuleResult{Label: label})
			continue
		}
		if capsuleRes == nil {
			continue
		}
		capsuleRes.errors = dedupeConfigErrors(capsuleRes.errors, seenConfigErrors)
		result.Capsules = append(result.Capsules, capsuleRes.toPublic(label))
		for _, v := range capsuleRes.violations {
			result.Violations = append(result.Violations, label+": "+v.Message)
		}
		for _, e := range capsuleRes.errors {
			result.Errors = append(result.Errors, label+": "+e.Message)
		}
	}
	return &result
}

func dedupeConfigErrors(errors []port.Violation, seen map[string]bool) []port.Violation {
	filtered := errors[:0]
	for _, err := range errors {
		key := configErrorKey(err)
		if key != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		filtered = append(filtered, err)
	}
	return filtered
}

func configErrorKey(err port.Violation) string {
	if filepath.Base(err.File) != port.ConfigFile {
		return ""
	}
	return err.File + "\x00" + err.Message
}

func matchCapsulesToLanguages(entries []service.CapsuleEntry, languages []port.Language) []capsuleEntry {
	langMap := make(map[string]port.Language, len(languages))
	for _, lang := range languages {
		langMap[lang.Name()] = lang
	}
	var capsules []capsuleEntry
	for _, e := range entries {
		if lang := langMap[e.LangName]; lang != nil {
			capsules = append(capsules, capsuleEntry{capsule: e.Capsule, lang: lang})
		}
	}
	return capsules
}

func nestedCapsuleDirs(capsules []capsuleEntry, capsuleDir string) []string {
	var nested []string
	for _, c := range capsules {
		if c.capsule.Dir != capsuleDir && strings.HasPrefix(c.capsule.Dir, capsuleDir+string(filepath.Separator)) {
			nested = append(nested, c.capsule.Dir)
		}
	}
	return nested
}

func checkCapsule(fsys port.FileSystem, capsule port.Capsule, lang port.Language, repo port.GraphRepository, configDir string, nestedDirs []string) (*capsuleResult, error) {
	ctx, err := loadCapsuleConfig(fsys, repo, configDir, capsule.Dir)
	if err != nil {
		return nil, err
	}
	if ctx.loadErr != nil {
		return &capsuleResult{errors: []port.Violation{*ctx.loadErr}}, nil
	}
	if !ctx.hasRootConfig && !hasScopedConfig(fsys, capsule.Dir) {
		return nil, nil
	}
	chk := newCapsuleChecker(fsys, capsule, lang, repo, configDir, ctx, nestedDirs)
	if err := chk.walk(fsys, capsule.Dir); err != nil {
		return nil, err
	}
	chk.validateAll()
	if chk.res.hasOverlapError || chk.res.hasDuplicateGlobError {
		chk.res.filesEncountered = 0
		chk.res.filesScanned = 0
		chk.res.relations = 0
		chk.res.violations = nil
	}
	sort.Slice(chk.res.errors, func(i, j int) bool {
		return chk.res.errors[i].Message < chk.res.errors[j].Message
	})
	sort.Slice(chk.res.violations, func(i, j int) bool {
		return chk.res.violations[i].Message < chk.res.violations[j].Message
	})
	return chk.res, nil
}

func makeConfigLoadError(cfgPath string, err error) port.Violation {
	v := port.Violation{
		Rule:     "config-load-error",
		Severity: "error",
		Source:   "baft",
		File:     cfgPath,
	}

	var pe *mermaid.ParseError
	if errors.As(err, &pe) {
		if pe.Raw != "" {
			v.Message = fmt.Sprintf("unrecognized mermaid line: %s (%s:%d)", strings.TrimSpace(pe.Raw), cfgPath, pe.Line)
		} else if pe.Line > 0 {
			v.Message = fmt.Sprintf("%s (%s:%d)", pe.Msg, cfgPath, pe.Line)
		} else {
			v.Message = fmt.Sprintf("%s (%s)", pe.Msg, cfgPath)
		}
		if pe.Line > 0 {
			v.Line = pe.Line
		}
		return v
	}

	v.Message = err.Error()
	if !strings.Contains(v.Message, cfgPath) {
		v.Message = fmt.Sprintf("%s (%s)", v.Message, cfgPath)
	}
	return v
}

func loadCapsuleConfig(fsys port.FileSystem, repo port.GraphRepository, configDir, capsuleDir string) (configContext, error) {
	var ctx configContext
	configPath := service.FindConfig(fsys, configDir, capsuleDir)
	ctx.configPathAbs, _ = filepath.Abs(configPath)
	raw, err := fsys.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ctx, nil
		}
		loadErr := makeConfigLoadError(ctx.configPathAbs, err)
		ctx.loadErr = &loadErr
		return ctx, nil
	}
	ctx.hasRootConfig = true
	ctx.rootGraph, err = repo.Load(string(raw))
	if err != nil {
		loadErr := makeConfigLoadError(ctx.configPathAbs, err)
		ctx.loadErr = &loadErr
		return ctx, nil
	}
	return ctx, nil
}
