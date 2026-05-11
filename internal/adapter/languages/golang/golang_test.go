package golang

import (
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/port"
)

func TestIsGovernedFile(t *testing.T) {
	l := Language{}
	cases := map[string]bool{
		// Standard governed
		"main.go":                         true,
		"internal/domain/model.go":        true,
		"internal/adapter/http/server.go": true,
		"internal/deep/nested/ok.go":      true,
		"application/order.go":            true,
		"api/handler.go":                  true,
		"domain/model.go":                 true,
		"cmd/tool.go":                     true,
		"pkg/lib.go":                      true,

		// Not governed: test files
		"internal/domain/model_test.go": false,
		"main_test.go":                  false,
		"api/handler_test.go":           false,

		// Not governed: wrong extension
		"internal/domain/model.md": false,
		"README.md":                false,
		"internal/config.yaml":     false,
	}
	for rel, want := range cases {
		t.Run(rel, func(t *testing.T) {
			if got := l.IsGovernedFile(rel); got != want {
				t.Errorf("IsGovernedFile(%q) = %v, want %v", rel, got, want)
			}
		})
	}
}

func TestParseImports(t *testing.T) {
	type tc struct {
		label string
		src   string
		want  []string
	}
	cases := []tc{
		{
			label: "basic grouped",
			src: `package main
import (
	"context"
	"fmt"
	"example.com/myproject/internal/cmd"
	"google.golang.org/grpc"
)
func main() {}
`,
			want: []string{"context", "fmt", "example.com/myproject/internal/cmd", "google.golang.org/grpc"},
		},
		{
			label: "mixed aliases blanks dots",
			src:   "package main\nimport (\n\t\"fmt\"\n\tf2 \"os\"\n\t_ \"database/sql/driver\"\n\t. \"math\"\n)\nfunc main() {}\n",
			want:  []string{"fmt", "os", "database/sql/driver", "math"},
		},
		{
			label: "single import",
			src: `package main
import "fmt"
func main() {}
`,
			want: []string{"fmt"},
		},
		{
			label: "no imports",
			src: `package main
func main() {}
`,
			want: []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			fs := memfs.New()
			fs.WriteFile("/main.go", []byte(c.src), 0o644)
			got, err := Language{}.ParseImports(fs, "/main.go")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %d imports, want %d: %v", len(got), len(c.want), got)
			}
			for i := range c.want {
				if got[i].Path != c.want[i] {
					t.Errorf("[%d] got %q, want %q", i, got[i].Path, c.want[i])
				}
			}
		})
	}
}

func TestParseImportsParseError(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/bad.go", []byte("package main\nimport ("), 0o644)
	_, err := Language{}.ParseImports(fs, "/bad.go")
	if err == nil {
		t.Fatal("expected error for unparseable file")
	}
}

func TestResolveInternalTarget(t *testing.T) {
	l := Language{}
	capsule := port.Capsule{CapsuleID: "example.com/myproject"}

	type tc struct {
		spec     string
		fileRel  string
		wantPath string
		wantIntl bool
	}
	cases := []tc{
		// External packages
		{"context", "main.go", "", false},
		{"fmt", "main.go", "", false},
		{"google.golang.org/grpc", "main.go", "", false},

		// Internal packages
		{"example.com/myproject/internal/cmd", "main.go", "internal/cmd", true},
		{"example.com/myproject/internal/service", "internal/cmd/run.go", "internal/service", true},
		{"example.com/myproject/main.go", "internal/cmd/run.go", "main.go", true},

		// Edge: partial prefix match must NOT match
		{"example.com/myproject-v2/foo", "main.go", "", false},
		{"example.com/myprojectx/foo", "main.go", "", false},

		// Edge: module path with deeper root
		{"example.com/myproject/foo/bar/baz", "main.go", "foo/bar/baz", true},
	}
	for _, c := range cases {
		t.Run(c.spec, func(t *testing.T) {
			gotPath, gotIntl := l.ResolveInternalTarget(nil, port.ImportSpec{Path: c.spec}, capsule, c.fileRel)
			if gotPath != c.wantPath || gotIntl != c.wantIntl {
				t.Errorf("ResolveInternalTarget(%q, file=%q) = (%q, %v), want (%q, %v)",
					c.spec, c.fileRel, gotPath, gotIntl, c.wantPath, c.wantIntl)
			}
		})
	}
}

func TestReadGoModulePath(t *testing.T) {
	type tc struct {
		label   string
		content string
		want    string
		wantErr bool
	}
	cases := []tc{
		{
			label:   "standard",
			content: "module example.com/myproject\n\ngo 1.21\n",
			want:    "example.com/myproject",
		},
		{
			label:   "with leading comment",
			content: "// my module\nmodule example.com/myproject\n\ngo 1.21\n",
			want:    "example.com/myproject",
		},
		{
			label:   "extra whitespace",
			content: "  module   example.com/myproject  \n\ngo 1.21\n",
			want:    "example.com/myproject",
		},
		{
			label:   "deep module path",
			content: "module github.com/org/repo/v2\n\ngo 1.21\n",
			want:    "github.com/org/repo/v2",
		},
		{
			label:   "no module line",
			content: "go 1.21\n",
			wantErr: true,
		},
		{
			label:   "empty file",
			content: "",
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			fs := memfs.New()
			fs.WriteFile("/go.mod", []byte(c.content), 0o644)
			got, err := readGoModulePath(fs, "/go.mod")
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
