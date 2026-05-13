package port

import "io"

// Writer is the writer interface used by reporters.
type Writer = io.Writer

// Violation is a single architecture rule violation with full location data.
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

// CheckResultRenderer renders a RunResult to a string.
type CheckResultRenderer interface {
	Render(result *CheckResult) string
}

// CheckResult holds the outcome of a baft check run.
type CheckResult struct {
	Capsules   []CapsuleResult `json:"capsules,omitempty"`
	Violations []string        `json:"violations,omitempty"`
	Warnings   []string        `json:"warnings,omitempty"`
	Errors     []string        `json:"errors,omitempty"`
}

// CapsuleResult holds the outcome for a single capsule.
type CapsuleResult struct {
	Label            string
	FilesEncountered int
	FilesScanned     int
	Nodes            int
	Edges            int
	Relations        int
	Violations       []Violation
	Errors           []Violation
}
