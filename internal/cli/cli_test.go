package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/languages/jvm"
)

type run struct {
	code   int
	stdout string
	stderr string
}

func exec(t *testing.T, stdin string, args ...string) run {
	t.Helper()
	var out, errOut strings.Builder
	a := &app{
		in:      strings.NewReader(stdin),
		out:     &out,
		errOut:  &errOut,
		docs:    os.DirFS("../.."),
		version: "1.2.3",
	}
	return run{a.run(args), out.String(), errOut.String()}
}

func writeFiles(t *testing.T, root string, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// capsule writes a Go capsule whose contract forbids a -> b.
func capsule(t *testing.T) string {
	t.Helper()
	return writeFiles(t, t.TempDir(), map[string]string{
		"go.mod":  "module example.com/x\n",
		"BAFT.md": "```mermaid\nflowchart TD\n    a[\"a\"]\n    b[\"b\"]\n```\n",
		"a/a.go":  "package a\n\nimport \"example.com/x/b\"\n\nvar _ = b.V\n",
		"b/b.go":  "package b\n\nvar V = 1\n",
	})
}

func TestLangFlagAcceptsBothSyntaxes(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"check", "--lang=go", root},
		{"check", "--lang", "go", root},
		{"check", "-lang=go", root},
		{"check", root, "--lang=go"},
	} {
		if got := exec(t, "", args...); got.code != exitOK {
			t.Errorf("%v: want exit %d, got %d (%s)", args, exitOK, got.code, got.stderr)
		}
	}
}

func TestJVMAliases(t *testing.T) {
	for _, names := range [][]string{{"jvm"}, {"java"}, {"kotlin"}, {"java", "kotlin", "jvm"}} {
		langs, err := resolveLangs(names)
		if err != nil {
			t.Fatalf("%v: %v", names, err)
		}
		if len(langs) != 1 || langs[0].Name() != jvm.Name {
			t.Errorf("%v: got %d adapters (%v), want one %q", names, len(langs), langs, jvm.Name)
		}
	}
	if langs, err := resolveLangs(nil); err != nil || len(langs) != len(languageNames) {
		t.Errorf("default set: got %d adapters, %v", len(langs), err)
	}
}

func TestUsageErrors(t *testing.T) {
	cases := [][]string{
		{"nope"},
		{"check", "--lang=klingon"},
		{"check", "--reporter=yaml"},
		{"check", "--langgo"},
		{"check", "--nope"},
		{"check", "one", "two"},
		{"dump", "--color-palette=neon"},
		{"restyle", "--stdin"},
		{"restyle", "--path=BAFT.md"},
		{"restyle", "--stdin", "--path=BAFT.md", "."},
		{"integrate", "--verify-compatible"},
		{"integrate", "extra"},
		{"manual", "extra"},
	}
	for _, args := range cases {
		got := exec(t, "", args...)
		if got.code != exitUsage {
			t.Errorf("%v: want exit %d, got %d (%s)", args, exitUsage, got.code, got.stderr)
		}
		if got.stderr == "" {
			t.Errorf("%v: want an error on stderr", args)
		}
	}
}

func TestHelpIsDocumentedForEveryCommand(t *testing.T) {
	for _, cmd := range []string{"check", "dump", "restyle", "integrate"} {
		got := exec(t, "", cmd, "--help")
		if got.code != exitOK {
			t.Fatalf("%s --help: want exit 0, got %d", cmd, got.code)
		}
		if !strings.HasPrefix(got.stdout, "Usage: baft "+cmd) {
			t.Fatalf("%s --help: unexpected help text %q", cmd, got.stdout)
		}
		// Every documented flag must be defined, or parsing rejects it.
		for _, flagName := range regexp.MustCompile(`--[a-z-]+`).FindAllString(got.stdout, -1) {
			if flagName == "--help" {
				continue
			}
			probe := exec(t, "", cmd, flagName+"=x")
			if strings.Contains(probe.stderr, "not defined") {
				t.Errorf("%s: %s is documented but not defined", cmd, flagName)
			}
		}
	}
}

func TestRootHelpAndVersion(t *testing.T) {
	if got := exec(t, ""); got.code != exitOK || !strings.Contains(got.stdout, "Usage: baft <command>") {
		t.Errorf("bare invocation: %+v", got)
	}
	if got := exec(t, "", "--version"); got.code != exitOK || strings.TrimSpace(got.stdout) != "1.2.3" {
		t.Errorf("--version: %+v", got)
	}
	if got := exec(t, "", "manual"); got.code != exitOK || got.stdout == "" {
		t.Errorf("manual: %+v", got)
	}
}

func TestZeroCapsulesWarnsAndSucceeds(t *testing.T) {
	got := exec(t, "", "check", t.TempDir())
	if got.code != exitOK {
		t.Fatalf("want exit 0, got %d", got.code)
	}
	if !strings.Contains(got.stderr, "nothing was checked") {
		t.Fatalf("want a zero-capsule warning, got %q", got.stderr)
	}
}

func TestViolationsExitOne(t *testing.T) {
	got := exec(t, "", "check", "--lang=go", capsule(t))
	if got.code != exitFail {
		t.Fatalf("want exit %d, got %d (%s)", exitFail, got.code, got.stdout)
	}
	if strings.Contains(got.stdout, "\033") {
		t.Fatalf("want no ANSI escapes when stdout is not a terminal: %q", got.stdout)
	}
}

func TestReporters(t *testing.T) {
	root := capsule(t)

	var doc struct {
		SchemaVersion int `json:"schemaVersion"`
		Capsules      []struct {
			Label      string `json:"label"`
			Violations []struct {
				Rule string `json:"rule"`
			} `json:"violations"`
		} `json:"capsules"`
	}
	got := exec(t, "", "check", "--reporter=json", root)
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("json reporter: %v (%s)", err, got.stdout)
	}
	if doc.SchemaVersion == 0 || len(doc.Capsules) != 1 || doc.Capsules[0].Label == "" {
		t.Fatalf("json reporter: unexpected document %+v", doc)
	}
	if len(doc.Capsules[0].Violations) == 0 || doc.Capsules[0].Violations[0].Rule == "" {
		t.Fatalf("json reporter: missing violations %+v", doc)
	}

	diagnostics := exec(t, "", "check", "--reporter=diagnostics", root).stdout
	for _, alias := range []string{"vsce", "intellij"} {
		if out := exec(t, "", "check", "--reporter="+alias, root).stdout; out != diagnostics {
			t.Errorf("reporter %s: want the diagnostics output, got %q", alias, out)
		}
	}
	var payload struct {
		Violations []struct {
			Rule string `json:"rule"`
		} `json:"violations"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(diagnostics), &payload); err != nil {
		t.Fatalf("diagnostics reporter: %v (%s)", err, diagnostics)
	}
	if len(payload.Violations) == 0 || payload.Violations[0].Rule == "" {
		t.Errorf("diagnostics reporter: missing violations in %q", diagnostics)
	}
}

func TestRestyleStdin(t *testing.T) {
	contract := "```mermaid\nflowchart TD\n    a[\"a\"]\n```\n"
	got := exec(t, contract, "restyle", "--stdin", "--path", "BAFT.md")
	if got.code != exitOK || !strings.Contains(got.stdout, "flowchart TD") {
		t.Fatalf("restyle --stdin: %+v", got)
	}
}

// dump's stdout is the review surface for an amendment: it names every node and
// edge it adds, and --dry-run reports them without touching the tree.
func TestDumpNamesAdditionsAndDryRunWritesNothing(t *testing.T) {
	root := writeFiles(t, capsule(t), map[string]string{
		"a/a.go": "package a\n\nimport (\n\t\"example.com/x/b\"\n\t\"example.com/x/c\"\n)\n\nvar _ = b.V\nvar _ = c.V\n",
		"c/c.go": "package c\n\nvar V = 1\n",
	})
	contract := filepath.Join(root, "BAFT.md")
	before, err := os.ReadFile(contract)
	if err != nil {
		t.Fatal(err)
	}

	dry := exec(t, "", "dump", "--lang=go", "--dry-run", root)
	for _, want := range []string{"[would amend]", "(+1 nodes, +2 edges)", "+ node c", "+ edge a --> b", "+ edge a --> c"} {
		if dry.code != exitOK || !strings.Contains(dry.stdout, want) {
			t.Fatalf("dry run: want %q, got %+v", want, dry)
		}
	}
	if after, err := os.ReadFile(contract); err != nil || string(after) != string(before) {
		t.Fatalf("dry run rewrote the contract: %s (%v)", after, err)
	}

	wet := exec(t, "", "dump", "--lang=go", root)
	if wet.code != exitOK || !strings.Contains(wet.stdout, "[amended] ") {
		t.Fatalf("dump: %+v", wet)
	}
	if got := exec(t, "", "check", "--lang=go", root); got.code != exitOK {
		t.Fatalf("check after dump: %+v", got)
	}
}

// A repo with no contract yet reports what dump would create, and creates nothing.
func TestDumpDryRunOnUntrackedRepoCreatesNothing(t *testing.T) {
	root := writeFiles(t, t.TempDir(), map[string]string{
		"go.mod": "module example.com/x\n",
		"b/b.go": "package b\n\nvar V = 1\n",
	})

	got := exec(t, "", "dump", "--lang=go", "--dry-run", root)
	if got.code != exitOK || !strings.Contains(got.stdout, "[would create] ") {
		t.Fatalf("dump --dry-run: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "BAFT.md")); !os.IsNotExist(err) {
		t.Fatalf("dry run created a contract (%v)", err)
	}
}
