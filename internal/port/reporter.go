package port

import "io"

type Writer = io.Writer

// Violation severity is "error" (fails the check) or "warning" (reported only).
type Violation struct {
	Rule      string `json:"rule"`
	Severity  string `json:"severity"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	ColumnEnd int    `json:"columnEnd,omitempty"`
	LineEnd   int    `json:"lineEnd,omitempty"`
}

type CheckResultRenderer interface {
	Render(result *CheckResult) string
}

type CheckResult struct {
	Capsules   []CapsuleResult `json:"capsules,omitempty"`
	Violations []string        `json:"violations,omitempty"`
	Warnings   []string        `json:"warnings,omitempty"`
	// Errors are the failures that aborted the check itself (discovery,
	// cancellation, a capsule that could not be read). Contract mistakes are
	// per-file diagnostics and live in CapsuleResult.Errors instead.
	Errors []string `json:"errors,omitempty"`
}

// Failed reports whether the check should exit non-zero.
func (r *CheckResult) Failed() bool {
	if len(r.Violations) > 0 || len(r.Errors) > 0 {
		return true
	}
	for _, c := range r.Capsules {
		if len(c.Errors) > 0 {
			return true
		}
	}
	return false
}

type CapsuleResult struct {
	Label            string      `json:"label"`
	FilesEncountered int         `json:"filesEncountered"`
	FilesScanned     int         `json:"filesScanned"`
	Nodes            int         `json:"nodes"`
	Edges            int         `json:"edges"`
	Relations        int         `json:"relations"`
	Violations       []Violation `json:"violations,omitempty"`
	Errors           []Violation `json:"errors,omitempty"`
}
