// Package jvm implements the language adapter shared by Java and Kotlin: one
// Gradle/Maven capsule compiles both source sets, so one adapter scans both.
package jvm

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/dariushalipour/baft/internal/adapter/languages/internal/lineoffsets"
	"github.com/dariushalipour/baft/internal/adapter/languages/internal/namespaces"
	"github.com/dariushalipour/baft/internal/port"
)

// Name is the language name Java and Kotlin sources both resolve to.
const Name = "jvm"

var exts = []string{".java", ".kt"}

// Language must be used by pointer: resolving an import probes the filesystem
// for source roots, and the caches below turn that from once per import into
// once per capsule.
type Language struct {
	roots sync.Map // capsule dir -> []string of capsule-relative source roots
	held  sync.Map // capsule-relative "<dir>\x00<rel>" -> bool
}

func (l *Language) Name() string { return Name }

func (l *Language) IsScannableFile(rel string) bool {
	for _, ext := range exts {
		if strings.HasSuffix(rel, ext) {
			return true
		}
	}
	return false
}

var importRe = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)(?:\.\*)?`)
var packageRe = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)`)

func (l *Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	indices := importRe.FindAllSubmatchIndex(data, -1)
	out := make([]port.ImportSpec, 0, len(indices))
	lineOffsets := lineoffsets.MakeLineOffsets(data)

	for _, m := range indices {
		importPath := strings.TrimSuffix(string(data[m[2]:m[3]]), ".*")
		line, col := lineoffsets.OffsetToLineCol(lineOffsets, data, m[2])
		out = append(out, port.ImportSpec{Path: importPath, Namespace: importPath, Line: line, Col: col, ColEnd: col + len(importPath)})
	}
	return out, nil
}

func (l *Language) GetFileNamespace(fsys port.FileSystem, absPath string) (string, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	m := packageRe.FindSubmatch(data)
	if m == nil {
		return "", nil
	}
	return string(m[1]), nil
}

func (l *Language) ResolveInternalTarget(fsys port.FileSystem, spec port.ImportSpec, c port.Capsule, fileRel string) (string, bool) {
	basePkg := c.CapsuleID
	if basePkg == "" || !namespaces.IsInternal(spec.Path, basePkg) {
		return "", false
	}
	rel := strings.Replace(spec.Path, ".", "/", -1)
	return filepath.ToSlash(filepath.Join(l.sourcePrefix(fsys, c.Dir, fileRel, rel), rel)), true
}

// sourcePrefix returns the src/<set>/<lang> root that holds rel. One capsule
// compiles every source set together, so a Java file may import a class living
// under src/main/kotlin and vice versa: the importing file's own root wins,
// then any root holding the target, then any root holding its package.
func (l *Language) sourcePrefix(fsys port.FileSystem, capsuleDir, fileRel, rel string) string {
	own := declaredPrefix(fileRel)
	roots := []string{own}
	for _, root := range l.sourceRoots(fsys, capsuleDir) {
		if root != own {
			roots = append(roots, root)
		}
	}
	for _, probe := range []string{rel, path.Dir(rel)} {
		for _, root := range roots {
			if l.holds(fsys, filepath.Join(capsuleDir, root), probe) {
				return root
			}
		}
	}
	return own
}

// declaredPrefix is the source root the importing file itself sits in.
func declaredPrefix(fileRel string) string {
	parts := strings.Split(fileRel, "/")
	if len(parts) >= 3 && parts[0] == "src" {
		return strings.Join(parts[:3], "/")
	}
	if strings.HasSuffix(fileRel, ".kt") {
		return "src/main/kotlin"
	}
	return "src/main/java"
}

// sourceRoots lists the capsule-relative source roots, memoised per capsule.
func (l *Language) sourceRoots(fsys port.FileSystem, capsuleDir string) []string {
	if cached, ok := l.roots.Load(capsuleDir); ok {
		return cached.([]string)
	}
	var out []string
	for _, dir := range sourceDirs(fsys, capsuleDir) {
		if rel, err := filepath.Rel(capsuleDir, dir); err == nil {
			out = append(out, filepath.ToSlash(rel))
		}
	}
	cached, _ := l.roots.LoadOrStore(capsuleDir, out)
	return cached.([]string)
}

// holds reports whether dir contains rel as a directory or as a source file,
// memoised because every import of the same package asks the same question.
func (l *Language) holds(fsys port.FileSystem, dir, rel string) bool {
	key := dir + "\x00" + rel
	if cached, ok := l.held.Load(key); ok {
		return cached.(bool)
	}
	found := exists(fsys, filepath.Join(dir, rel))
	l.held.Store(key, found)
	return found
}

func exists(fsys port.FileSystem, target string) bool {
	if _, err := fsys.Stat(target); err == nil {
		return true
	}
	for _, ext := range exts {
		if _, err := fsys.Stat(target + ext); err == nil {
			return true
		}
	}
	return false
}

func (l *Language) SupportsFileGlobs() bool { return false }

func (l *Language) Register(d port.CapsuleDiscovery) {
	d.Register(Name, port.ManifestInfo{
		Names: []string{"build.gradle.kts", "build.gradle", "pom.xml"},
		ParseFunc: func(fsys port.FileSystem, manifest string) (string, error) {
			return findBaseCapsule(fsys, filepath.Dir(manifest))
		},
		BaseIgnoreEntries: []string{"*Test.java", "*Tests.java", "*TestCase.java", "*Test.kt", "*_test.kt"},
	})
}

// findBaseCapsule prefers the package prefix shared by every source set. Sets
// that share none — a JVM target next to a JS one, say — must not sink the
// whole capsule, so it falls back to src/main and then to no base package at
// all, which still yields a capsule with nothing internal.
func findBaseCapsule(fsys port.FileSystem, projectRoot string) (string, error) {
	srcDirs := sourceDirs(fsys, projectRoot)
	for _, dirs := range [][]string{srcDirs, mainDirs(srcDirs)} {
		if prefix, err := namespaces.CommonPrefix(fsys, dirs, exts); err == nil {
			return prefix, nil
		}
	}
	return "", nil
}

// mainDirs keeps only the primary source sets, src/main/{java,kotlin}.
func mainDirs(srcDirs []string) []string {
	var out []string
	for _, dir := range srcDirs {
		if filepath.Base(filepath.Dir(dir)) == "main" {
			out = append(out, dir)
		}
	}
	return out
}

// testSetRe matches source-set names whose "test" is a whole word — test,
// androidUnitTest, testFixtures — sparing production names like "attestation".
var testSetRe = regexp.MustCompile(`(^test|Test)([A-Z]|s?$)`)

// sourceDirs returns the existing src/<sourceSet>/{java,kotlin} directories,
// covering plain Gradle/Maven layouts and Kotlin multiplatform source sets.
func sourceDirs(fsys port.FileSystem, projectRoot string) []string {
	src := filepath.Join(projectRoot, "src")
	sets, err := fsys.ReadDir(src)
	if err != nil {
		return nil
	}
	var out []string
	for _, set := range sets {
		if !set.IsDir() || testSetRe.MatchString(set.Name()) {
			continue
		}
		for _, lang := range []string{"java", "kotlin"} {
			dir := filepath.Join(src, set.Name(), lang)
			if _, err := fsys.Stat(dir); err == nil {
				out = append(out, dir)
			}
		}
	}
	return out
}
