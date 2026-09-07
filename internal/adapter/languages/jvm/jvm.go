// Package jvm implements the language adapter shared by Java and Kotlin: one
// Gradle/Maven capsule compiles both source sets, so one adapter scans both.
package jvm

import (
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

func (Language) ResolveInternalTarget(_ port.FileSystem, spec port.ImportSpec, c port.Capsule, fileRel string) (string, bool) {
	basePkg := c.CapsuleID
	if basePkg == "" || !namespaces.IsInternal(spec.Path, basePkg) {
		return "", false
	}
	srcPrefix := resolveSourcePrefix(fileRel)
	basePath := strings.Replace(basePkg, ".", "/", -1)
	rest := strings.TrimPrefix(strings.TrimPrefix(spec.Path, basePkg), ".")
	if rest == "" {
		return filepath.ToSlash(filepath.Join(srcPrefix, basePath)), true
	}
	return filepath.ToSlash(filepath.Join(srcPrefix, basePath, strings.Replace(rest, ".", "/", -1))), true
}

func resolveSourcePrefix(fileRel string) string {
	parts := strings.Split(fileRel, "/")
	if len(parts) >= 3 && parts[0] == "src" {
		return strings.Join(parts[:3], "/")
	}
	if strings.HasSuffix(fileRel, ".kt") {
		return "src/main/kotlin"
	}
	return "src/main/java"
}

func (Language) SupportsFileGlobs() bool { return false }

func (Language) Register(d port.CapsuleDiscovery) {
	d.Register(Name, port.ManifestInfo{
		Names: []string{"build.gradle.kts", "build.gradle", "pom.xml"},
		ParseFunc: func(fsys port.FileSystem, path string) (string, error) {
			return findBaseCapsule(fsys, filepath.Dir(path))
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
		if !set.IsDir() || strings.Contains(strings.ToLower(set.Name()), "test") {
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
