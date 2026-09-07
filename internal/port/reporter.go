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
	Errors     []string        `json:"errors,omitempty"`
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
