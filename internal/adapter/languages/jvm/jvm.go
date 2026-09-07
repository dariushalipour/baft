// Package jvm implements the language adapter shared by Java and Kotlin: one
// Gradle/Maven capsule compiles both source sets, so one adapter scans both.
package jvm

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/languages/internal/lineoffsets"
	"github.com/dariushalipour/baft/internal/adapter/languages/internal/namespaces"
	"github.com/dariushalipour/baft/internal/port"
)

// Name is the language name both "java" and "kotlin" resolve to.
const Name = "jvm"

var exts = []string{".java", ".kt"}

type Language struct{}

func (Language) Name() string { return Name }

func (Language) IsScannableFile(rel string) bool {
	return strings.HasSuffix(rel, ".java") || strings.HasSuffix(rel, ".kt")
}

var importRe = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)(?:\.\*)?`)
var packageRe = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
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

func (Language) GetFileNamespace(fsys port.FileSystem, absPath string) (string, error) {
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

func (Language) ResolveInternalTarget(fsys port.FileSystem, spec port.ImportSpec, c port.Capsule, fileRel string) (string, bool) {
	basePkg := c.CapsuleID
	if basePkg == "" || !namespaces.IsInternal(spec.Path, basePkg) {
		return "", false
	}
	rel := strings.Replace(spec.Path, ".", "/", -1)
	return filepath.ToSlash(filepath.Join(sourcePrefix(fsys, c.Dir, fileRel, rel), rel)), true
}

// sourcePrefix returns the src/<set>/<lang> root that holds rel. One capsule
// compiles every source set together, so a Java file may import a class living
// under src/main/kotlin and vice versa: the importing file's own root wins,
// then any root holding the target, then any root holding its package.
func sourcePrefix(fsys port.FileSystem, capsuleDir, fileRel, rel string) string {
	own := declaredPrefix(fileRel)
	if holds(fsys, filepath.Join(capsuleDir, own), rel) {
		return own
	}
	roots := append([]string{own}, otherPrefixes(fsys, capsuleDir, own)...)
	for _, probe := range []string{rel, path.Dir(rel)} {
		for _, root := range roots {
			if holds(fsys, filepath.Join(capsuleDir, root), probe) {
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

func otherPrefixes(fsys port.FileSystem, capsuleDir, own string) []string {
	var out []string
	for _, dir := range sourceDirs(fsys, capsuleDir) {
		rel, err := filepath.Rel(capsuleDir, dir)
		if err == nil && filepath.ToSlash(rel) != own {
			out = append(out, filepath.ToSlash(rel))
		}
	}
	return out
}

// holds reports whether dir contains rel as a directory or as a source file.
func holds(fsys port.FileSystem, dir, rel string) bool {
	target := filepath.Join(dir, rel)
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

func (Language) SupportsFileGlobs() bool { return false }

func (Language) Register(d port.CapsuleDiscovery) {
	d.Register(Name, port.ManifestInfo{
		Names: []string{"build.gradle.kts", "build.gradle", "pom.xml"},
		ParseFunc: func(fsys port.FileSystem, manifest string) (string, error) {
			return findBaseCapsule(fsys, filepath.Dir(manifest))
		},
		BaseIgnoreEntries: []string{"*Test.java", "*Tests.java", "*TestCase.java", "*Test.kt", "*_test.kt"},
	})
}

func findBaseCapsule(fsys port.FileSystem, projectRoot string) (string, error) {
	srcDirs := sourceDirs(fsys, projectRoot)
	if len(srcDirs) == 0 {
		return "", nil
	}
	return namespaces.CommonPrefix(fsys, srcDirs, exts)
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
