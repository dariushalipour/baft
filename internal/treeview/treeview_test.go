package treeview

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name   string
		doc    string
		expect []Entry
	}{
		{
			name: "single file in subdirectory",
			doc: `/Users/jane/baft
└─ src/
   └─ app.ts`,
			expect: []Entry{
				{BaseDir: "/Users/jane/baft", RelPath: "src/app.ts"},
			},
		},
		{
			name: "multiple files at different levels",
			doc: `/home/user/project
├─ src/
│  ├─ main.ts
│  └─ utils/
│     └─ helpers.ts`,
			expect: []Entry{
				{BaseDir: "/home/user/project", RelPath: "src/main.ts"},
				{BaseDir: "/home/user/project", RelPath: "src/utils/helpers.ts"},
			},
		},
		{
			name: "multiple files at root level",
			doc: `/project
├─ main.go
└─ README.md`,
			expect: []Entry{
				{BaseDir: "/project", RelPath: "main.go"},
				{BaseDir: "/project", RelPath: "README.md"},
			},
		},
		{
			name: "only directories skipped",
			doc: `/project
└─ src/
   └─ utils/`,
			expect: []Entry{},
		},
		{
			name:   "empty doc",
			doc:    "",
			expect: nil,
		},
		{
			name:   "only base dir line",
			doc:    "/project",
			expect: []Entry{},
		},
		{
			name: "deep nesting",
			doc: `/a
└─ b/
   └─ c/
      └─ d/
         └─ e/
            └─ file.txt`,
			expect: []Entry{
				{BaseDir: "/a", RelPath: "b/c/d/e/file.txt"},
			},
		},
		{
			name: "mixed tree characters",
			doc: `/project
├─ src/
│  ├─ main.ts
│  └─ util.ts
└─ test/
   └─ main_test.ts`,
			expect: []Entry{
				{BaseDir: "/project", RelPath: "src/main.ts"},
				{BaseDir: "/project", RelPath: "src/util.ts"},
				{BaseDir: "/project", RelPath: "test/main_test.ts"},
			},
		},
		{
			name: "single file at root",
			doc: `/project
└─ app.ts`,
			expect: []Entry{
				{BaseDir: "/project", RelPath: "app.ts"},
			},
		},
		{
			name: "blanks and whitespace-only lines",
			doc: `/project

└─ src/

   └─ app.ts

`,
			expect: []Entry{
				{BaseDir: "/project", RelPath: "src/app.ts"},
			},
		},
		{
			name: "sibling directories with files",
			doc: `/project
└─ src/
   ├─ utils/
   │  └─ helpers.ts
   └─ main.ts`,
			expect: []Entry{
				{BaseDir: "/project", RelPath: "src/utils/helpers.ts"},
				{BaseDir: "/project", RelPath: "src/main.ts"},
			},
		},
		{
			name: "three levels of mixed branching",
			doc: `/project
├─ a/
│  ├─ b/
│  │  └─ deep.ts
│  └─ c.ts
└─ d.ts`,
			expect: []Entry{
				{BaseDir: "/project", RelPath: "a/b/deep.ts"},
				{BaseDir: "/project", RelPath: "a/c.ts"},
				{BaseDir: "/project", RelPath: "d.ts"},
			},
		},
		{
			name: "complex capsule layout",
			doc: `/Users/jane/baft
├─ billing/
│  ├─ go.mod
│  ├─ BAFT.md
│  ├─ application/
│  │  └─ order.go
│  └─ domain/
│     └─ order.go
├─ auth/
│  ├─ go.mod
│  ├─ BAFT.md
│  ├─ api/
│  │  └─ handler.go
│  └─ domain/
│     └─ auth.go
`,
			expect: []Entry{
				{BaseDir: "/Users/jane/baft", RelPath: "billing/go.mod"},
				{BaseDir: "/Users/jane/baft", RelPath: "billing/BAFT.md"},
				{BaseDir: "/Users/jane/baft", RelPath: "billing/application/order.go"},
				{BaseDir: "/Users/jane/baft", RelPath: "billing/domain/order.go"},
				{BaseDir: "/Users/jane/baft", RelPath: "auth/go.mod"},
				{BaseDir: "/Users/jane/baft", RelPath: "auth/BAFT.md"},
				{BaseDir: "/Users/jane/baft", RelPath: "auth/api/handler.go"},
				{BaseDir: "/Users/jane/baft", RelPath: "auth/domain/auth.go"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.doc)

			if len(tt.expect) == 0 && len(got) == 0 {
				return
			}
			if len(tt.expect) != len(got) {
				t.Fatalf("expected %d entries, got %d: %v", len(tt.expect), len(got), got)
			}

			for i, exp := range tt.expect {
				if got[i].BaseDir != exp.BaseDir {
					t.Errorf("entry %d: expected BaseDir %q, got %q", i, exp.BaseDir, got[i].BaseDir)
				}
				if got[i].RelPath != exp.RelPath {
					t.Errorf("entry %d: expected RelPath %q, got %q", i, exp.RelPath, got[i].RelPath)
				}
			}
		})
	}
}

func TestExtractBaseDir(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"absolute path", "/Users/jane/baft", "/Users/jane/baft"},
		{"with leading spaces", "  /project", "/project"},
		{"with trailing spaces", "/project  ", "/project"},
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBaseDir(tt.line)
			if got != tt.want {
				t.Errorf("extractBaseDir(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestDecodeTreeLine(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		depth  int
		expect string
	}{
		{"root branch right", "├─ billing/", 0, "billing/"},
		{"root branch last", "└─ auth/", 0, "auth/"},
		{"slot continuation + branch", "│  ├─ go.mod", 1, "go.mod"},
		{"slot continuation + last", "│  └─ domain/", 1, "domain/"},
		{"two continuations + branch", "│  │  ├─ handler.go", 2, "handler.go"},
		{"deep nesting", "│     └─ order.go", 2, "order.go"},
		{"root file no tree", "go.mod", 0, "go.mod"},
		{"empty line", "", 0, ""},
		{"whitespace only", "   ", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			depth, name := decodeTreeLine(tt.line)
			if depth != tt.depth {
				t.Errorf("decodeTreeLine(%q): expected depth %d, got %d", tt.line, tt.depth, depth)
			}
			if name != tt.expect {
				t.Errorf("decodeTreeLine(%q): expected name %q, got %q", tt.line, tt.expect, name)
			}
		})
	}
}
