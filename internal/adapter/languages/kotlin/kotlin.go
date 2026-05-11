package kotlin

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "kotlin" }

func (Language) IsGovernedFile(rel string) bool {
	if !strings.HasSuffix(rel, ".kt") {
		return false
	}
	base := filepath.Base(rel)
	if strings.HasSuffix(base, "Test.kt") || strings.HasSuffix(base, "_test.kt") {
		return false
	}
	if strings.HasSuffix(base, ".kt.kt") {
		return false
	}
	for _, skip := range generatedMarkers {
		if strings.Contains(rel, skip) {
			return false
		}
	}
	for _, prefix := range kotlinSourcePrefixes {
		if strings.HasPrefix(rel, prefix+"/") {
			return true
		}
	}
	return false
}

var generatedMarkers = []string{
	"/generated/",
	"/kapt/",
	"/ksp/",
	"/buildSrc/",
}

var importRe = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)(?:\.\*)?`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	indices := importRe.FindAllSubmatchIndex(data, -1)
	out := make([]port.ImportSpec, 0, len(indices))
	for _, m := range indices {
		importPath := strings.TrimSuffix(string(data[m[2]:m[3]]), ".*")
		line, col := byteOffsetToLineCol(data, m[2])
		out = append(out, port.ImportSpec{Path: importPath, Line: line, Col: col, ColEnd: col + len(importPath)})
	}
	return out, nil
}

func byteOffsetToLineCol(data []byte, offset int) (int, int) {
	if offset > len(data) {
		offset = len(data)
	}
	line, col := 1, 1
	for i := 0; i < offset; i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func (Language) ResolveInternalTarget(_ port.FileSystem, spec port.ImportSpec, c port.Capsule, fileRel string) (string, bool) {
	basePkg := c.CapsuleID
	if basePkg == "" {
		return "", false
	}
	if !isInternalCapsule(spec.Path, basePkg) {
		return "", false
	}
	srcPrefix := resolveSourcePrefix(fileRel)
	basePath := strings.Replace(basePkg, ".", "/", -1)
	rest := strings.TrimPrefix(spec.Path, basePkg)
	rest = strings.TrimPrefix(rest, ".")
	if rest == "" {
		return filepath.ToSlash(filepath.Join(srcPrefix, basePath)), true
	}
	relPath := strings.Replace(rest, ".", "/", -1)
	return filepath.ToSlash(filepath.Join(srcPrefix, basePath, relPath)), true
}

func resolveSourcePrefix(fileRel string) string {
	for _, prefix := range kotlinSourcePrefixes {
		if strings.HasPrefix(fileRel, prefix+"/") {
			return prefix
		}
	}
	return "src/main/kotlin"
}

func isInternalCapsule(spec, basePkg string) bool {
	if spec == basePkg {
		return true
	}
	if len(spec) <= len(basePkg) {
		return false
	}
	if !strings.HasPrefix(spec, basePkg) {
		return false
	}
	next := spec[len(basePkg)]
	return next == '.' && len(spec) > len(basePkg)+1 && spec[len(spec)-1] != '.'
}

func (Language) SupportsFileGlobs() bool { return false }

func RegisterDiscovery(d *service.CapsuleDiscovery) {
	d.Register("kotlin", service.ManifestInfo{
		Names: []string{"build.gradle.kts", "build.gradle"},
		ParseFunc: func(fsys port.FileSystem, path string) (string, error) {
			// ParseFunc only needs the directory containing the manifest.
			// findBaseCapsule walks source dirs from that directory.
			dir := filepath.Dir(path)
			return findBaseCapsule(fsys, dir)
		},
	})
}

func findBaseCapsule(fsys port.FileSystem, projectRoot string) (string, error) {
	srcDirs := []string{
		filepath.Join(projectRoot, "src/main/kotlin"),
		filepath.Join(projectRoot, "src/main/java"),
		filepath.Join(projectRoot, "src/jvmMain/kotlin"),
		filepath.Join(projectRoot, "src/jvmMain/java"),
		filepath.Join(projectRoot, "src/commonMain/kotlin"),
		filepath.Join(projectRoot, "src/androidMain/kotlin"),
		filepath.Join(projectRoot, "src/androidUnitTest/kotlin"),
		filepath.Join(projectRoot, "src/iosMain/kotlin"),
		filepath.Join(projectRoot, "src/iosArm64Main/kotlin"),
		filepath.Join(projectRoot, "src/iosSimulatorArm64Main/kotlin"),
		filepath.Join(projectRoot, "src/macosMain/kotlin"),
		filepath.Join(projectRoot, "src/macosX64Main/kotlin"),
		filepath.Join(projectRoot, "src/macosArm64Main/kotlin"),
		filepath.Join(projectRoot, "src/linuxMain/kotlin"),
		filepath.Join(projectRoot, "src/linuxX64Main/kotlin"),
		filepath.Join(projectRoot, "src/darwinMain/kotlin"),
		filepath.Join(projectRoot, "src/nativeMain/kotlin"),
		filepath.Join(projectRoot, "src/jsMain/kotlin"),
		filepath.Join(projectRoot, "src/mingwMain/kotlin"),
		filepath.Join(projectRoot, "src/mingwX64Main/kotlin"),
	}
	var chosenSrc string
	for _, sd := range srcDirs {
		if _, err := fsys.Stat(sd); err == nil {
			chosenSrc = sd
			break
		}
	}
	if chosenSrc == "" {
		return "", nil
	}

	var relPaths []string
	err := fsys.WalkDir(chosenSrc, func(abs string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(abs, ".kt") {
			return nil
		}
		rel, _ := filepath.Rel(chosenSrc, abs)
		rel = filepath.ToSlash(rel)
		dir := filepath.Dir(rel)
		if dir != "." {
			relPaths = append(relPaths, dir)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	if len(relPaths) == 0 {
		return "", fmt.Errorf("no .kt files found in %s", chosenSrc)
	}

	sort.Strings(relPaths)
	parts := strings.Split(relPaths[0], "/")

	var common []string
	for _, p := range parts {
		candidate := append([]string(nil), common...)
		candidate = append(candidate, p)
		prefix := strings.Join(candidate, "/") + "/"
		candStr := strings.Join(candidate, "/")
		ok := true
		for _, path := range relPaths {
			if !strings.HasPrefix(path, prefix) && path != candStr {
				ok = false
				break
			}
		}
		if !ok {
			break
		}
		common = append(common, p)
	}

	if len(common) == 0 {
		return "", fmt.Errorf("cannot determine base capsule")
	}

	return strings.Join(common, "."), nil
}

var kotlinSourcePrefixes = []string{
	"src/main/kotlin",
	"src/main/java",
	"src/jvmMain/kotlin",
	"src/jvmMain/java",
	"src/jvmTest/kotlin",
	"src/commonMain/kotlin",
	"src/commonTest/kotlin",
	"src/androidMain/kotlin",
	"src/androidUnitTest/kotlin",
	"src/androidAndroidTest/kotlin",
	"src/androidInstrumentedTest/kotlin",
	"src/iosMain/kotlin",
	"src/iosTest/kotlin",
	"src/iosArm64Main/kotlin",
	"src/iosArm64Test/kotlin",
	"src/iosSimulatorArm64Main/kotlin",
	"src/iosSimulatorArm64Test/kotlin",
	"src/macosMain/kotlin",
	"src/macosTest/kotlin",
	"src/macosX64Main/kotlin",
	"src/macosX64Test/kotlin",
	"src/macosArm64Main/kotlin",
	"src/macosArm64Test/kotlin",
	"src/linuxMain/kotlin",
	"src/linuxTest/kotlin",
	"src/linuxX64Main/kotlin",
	"src/linuxX64Test/kotlin",
	"src/darwinMain/kotlin",
	"src/darwinTest/kotlin",
	"src/nativeMain/kotlin",
	"src/nativeTest/kotlin",
	"src/jsMain/kotlin",
	"src/jsTest/kotlin",
	"src/mingwMain/kotlin",
	"src/mingwTest/kotlin",
	"src/mingwX64Main/kotlin",
	"src/mingwX64Test/kotlin",
}
