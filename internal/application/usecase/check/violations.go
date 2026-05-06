package check

import (
	"fmt"
	"path/filepath"

	"github.com/dariushalipour/strata/internal/port"
)

func importPrefix(fileRel string, spec port.ImportSpec, src, targetRel, dst string) string {
	return fmt.Sprintf("%s:%d:%d (%s) → %s (%s)", fileRel, spec.Line, spec.Col, src, targetRel, dst)
}

func formatRelation(fileRel string, spec port.ImportSpec, src, targetRel, dst, cfgPath string) string {
	return fmt.Sprintf("%s — relation not allowed (add edge in %s or move the file)", importPrefix(fileRel, spec, src, targetRel, dst), cfgPath)
}

func formatNoNode(scopeRel, cfgPath string) string {
	return fmt.Sprintf("%s is governed but matches no node in %s", scopeRel, cfgPath)
}

func formatImportNoNode(scopeRel string, spec port.ImportSpec, cfgPath string) string {
	return fmt.Sprintf("%s:%d:%d: import %q matches no node in %s", scopeRel, spec.Line, spec.Col, spec.Path, cfgPath)
}

func formatEndophobic(fileRel string, spec port.ImportSpec, targetRel, src, cfgPath string) string {
	return fmt.Sprintf("%s — %s is endophobic (%s)", importPrefix(fileRel, spec, src, targetRel, src), src, cfgPath)
}

func relToSlash(base, target string) string {
	rel, _ := filepath.Rel(base, target)
	return filepath.ToSlash(rel)
}

func absPath(dir, rel string) string {
	p := filepath.Join(dir, filepath.FromSlash(rel))
	p, _ = filepath.Abs(p)
	return p
}

func formatOverlap(a, b, aCfg, bCfg, aLine, bLine, witness string) string {
	return fmt.Sprintf("node %q (%s:%s) and node %q (%s:%s) overlap — file %s matches both globs", a, aCfg, aLine, b, bCfg, bLine, witness)
}

func makeRelationViolation(fileAbs, fileRel string, spec port.ImportSpec, src, targetRel, dst, cfgPath string) port.Violation {
	return port.Violation{
		Rule:      "import-not-allowed",
		Severity:  "error",
		Source:    "strata",
		Message:   formatRelation(fileRel, spec, src, targetRel, dst, cfgPath),
		File:      fileAbs,
		Line:      spec.Line,
		Column:    spec.Col,
		ColumnEnd: spec.ColEnd,
	}
}

func makeNoNodeViolation(fileAbs, scopeRel, cfgPath string) port.Violation {
	return port.Violation{
		Rule:     "no-node",
		Severity: "error",
		Source:   "strata",
		Message:  formatNoNode(scopeRel, cfgPath),
		File:     fileAbs,
	}
}

func makeImportNoNodeViolation(fileAbs, scopeRel string, spec port.ImportSpec, cfgPath string) port.Violation {
	return port.Violation{
		Rule:      "import-no-node",
		Severity:  "error",
		Source:    "strata",
		Message:   formatImportNoNode(scopeRel, spec, cfgPath),
		File:      fileAbs,
		Line:      spec.Line,
		Column:    spec.Col,
		ColumnEnd: spec.ColEnd,
	}
}

func makeEndophobicViolation(fileAbs, fileRel string, spec port.ImportSpec, targetRel, src, cfgPath string) port.Violation {
	return port.Violation{
		Rule:      "endophobic",
		Severity:  "error",
		Source:    "strata",
		Message:   formatEndophobic(fileRel, spec, targetRel, src, cfgPath),
		File:      fileAbs,
		Line:      spec.Line,
		Column:    spec.Col,
		ColumnEnd: spec.ColEnd,
	}
}

func makeFileGlobUnsupportedError(id, cfgPath string, line int, glob string) port.Violation {
	return port.Violation{
		Rule:     "file-glob-unsupported",
		Severity: "error",
		Source:   "strata",
		Message:  fmt.Sprintf("%s (%s:%d) references %s — file-shaped nodes require a language that supports file globs", id, cfgPath, line, glob),
		File:     cfgPath,
		Line:     line,
	}
}

func makeInvalidNodeGlobError(id, cfgPath string, line int, glob, msg string) port.Violation {
	return port.Violation{
		Rule:     "invalid-node-glob",
		Severity: "error",
		Source:   "strata",
		Message:  fmt.Sprintf("%s (%s:%d) references %s — %s", id, cfgPath, line, glob, msg),
		File:     cfgPath,
		Line:     line,
	}
}

func makeOverlapError(a, b, cfgPath string, aLine, bLine int, witness string) port.Violation {
	return port.Violation{
		Rule:     "node-overlap",
		Severity: "error",
		Source:   "strata",
		Message:  formatOverlap(a, b, cfgPath, cfgPath, fmt.Sprintf("%d", aLine), fmt.Sprintf("%d", bLine), witness),
		File:     cfgPath,
		Line:     aLine,
	}
}
