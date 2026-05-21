package kotlin

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/port"
)

type Language struct{}

func (Language) Name() string { return "kotlin" }

func (Language) IsScannableFile(rel string) bool {
	return strings.HasSuffix(rel, ".kt")
}

var importRe = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)(?:\.\*)?`)

func (Language) ParseImports(fsys port.FileSystem, absPath string) ([]port.ImportSpec, error) {
	data, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	indices := importRe.FindAllSubmatchIndex(data, -1)
	out := make([]port.ImportSpec, 0, len(indices))
	lineOffsets := makeLineOffsets(data)

	for _, m := range indices {
		importPath := strings.TrimSuffix(string(data[m[2]:m[3]]), ".*")
		line, col := offsetToLineCol(lineOffsets, data, m[2])
		out = append(out, port.ImportSpec{Path: importPath, Line: line, Col: col, ColEnd: col + len(importPath)})
	}
	return out, nil
}

// makeLineOffsets precomputes the byte offset of each line start for O(1) line/col lookup.
func makeLineOffsets(data []byte) []int {
	offsets := make([]int, 0, bytes.Count(data, []byte{'\n'})+1)
	offsets = append(offsets, 0)
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

// offsetToLineCol converts a byte offset to line/col using precomputed line offsets.
// Both line and col are 1-indexed.
func offsetToLineCol(lineOffsets []int, data []byte, offset int) (int, int) {
	if offset > len(data) {
		offset = len(data)
	}
	if offset < 0 {
		offset = 0
	}
	idx := sort.Search(len(lineOffsets), func(i int) bool {
		return lineOffsets[i] > offset
	})
	line := idx - 1
	if line < 0 {
		line = 0
	}
	return line + 1, offset - lineOffsets[line] + 1
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
	parts := strings.Split(fileRel, "/")
	if len(parts) >= 3 && parts[0] == "src" {
		return strings.Join(parts[:3], "/")
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
func (Language) Register(d port.CapsuleDiscovery) {
	d.Register("kotlin", port.ManifestInfo{
		Names: []string{"build.gradle.kts", "build.gradle"},
		ParseFunc: func(fsys port.FileSystem, path string) (string, error) {
			// ParseFunc only needs the directory containing the manifest.
			// findBaseCapsule walks source dirs from that directory.
			dir := filepath.Dir(path)
			return findBaseCapsule(fsys, dir)
		},
		BaseIgnoreEntries: []string{"build", ".gradle", ".kotlin", "gradle", "generated", "kapt", "ksp", "buildSrc", "scripts", "*Test.kt", "*_test.kt"},
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
	first := relPaths[0]
	last := relPaths[len(relPaths)-1]

	parts := strings.Split(first, "/")
	var common []string
	for _, p := range parts {
		if len(common) == 0 {
			if !strings.HasPrefix(last, p+"/") && !strings.HasPrefix(last, p) {
				break
			}
		} else {
			candidate := strings.Join(append(common, p), "/")
			if !strings.HasPrefix(last, candidate+"/") && !strings.HasPrefix(last, candidate) {
				break
			}
		}
		common = append(common, p)
	}

	if len(common) == 0 {
		return "", fmt.Errorf("cannot determine base capsule")
	}

	return strings.Join(common, "."), nil
}
