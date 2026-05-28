package service

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/languages/csharp"
	"github.com/dariushalipour/baft/internal/port"
	"github.com/dariushalipour/baft/pkg/treeview"
)

func buildFS(rootDir string, tree string, files map[string]string) *memfs.FS {
	for _, e := range treeview.ParseTree(rootDir, tree) {
		abs := filepath.Join(e.BaseDir, e.RelPath)
		if _, ok := files[abs]; !ok {
			files[abs] = ""
		}
	}
	fsys := memfs.New()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, absPath := range paths {
		_ = fsys.WriteFile(absPath, []byte(files[absPath]), 0o644)
	}
	return fsys
}

// --- helpers ---

func parseGoMod(fsys port.FileSystem, path string) (string, error) {
	data, err := fsys.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			name := strings.TrimSpace(line[len("module "):])
			if name == "" {
				return "", fmt.Errorf("empty module name in %s", path)
			}
			return name, nil
		}
	}
	return "", fmt.Errorf("no module line in %s", path)
}

func parsePubspec(fsys port.FileSystem, path string) (string, error) {
	data, err := fsys.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			return strings.TrimSpace(line[len("name:"):]), nil
		}
	}
	return "", fmt.Errorf("no name in %s", path)
}

func goModParser() func(port.FileSystem, string) (string, error) {
	return parseGoMod
}

func emptyParser() func(port.FileSystem, string) (string, error) {
	return func(port.FileSystem, string) (string, error) { return "", nil }
}

func errorParser(id string) func(port.FileSystem, string) (string, error) {
	return func(port.FileSystem, string) (string, error) { return id, fmt.Errorf("parse warning") }
}

// --- tests ---

func TestCapsuleDiscovery(t *testing.T) {
	tests := []struct {
		name     string
		tree     string
		files    map[string]string
		register func(*CapsuleDiscovery)
		root     string
		wantN    int
		wantErr  bool
		wantCaps []struct {
			ID       string
			Dir      string
			LangName string
		}
	}{
		{
			name: "no manifest in the tree yields no capsules",
			register: func(d *CapsuleDiscovery) {
				d.Register("go", port.ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
			},
			root:  "/root",
			wantN: 0,
		},
		{
			name: "root has a manifest",
			tree: `
├─ go.mod
`,
			files: map[string]string{
				"/root/go.mod": "module example.com/root",
			},
			register: func(d *CapsuleDiscovery) {
				d.Register("go", port.ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
			},
			root:  "/root",
			wantN: 1,
			wantCaps: []struct {
				ID       string
				Dir      string
				LangName string
			}{
				{ID: "example.com/root", Dir: "/root", LangName: "go"},
			},
		},
		{
			name: "walk discovers a capsule in a subdirectory",
			tree: `
└─ sub/
   └─ go.mod
`,
			files: map[string]string{
				"/root/sub/go.mod": "module example.com/sub",
			},
			register: func(d *CapsuleDiscovery) {
				d.Register("go", port.ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
			},
			root:  "/root",
			wantN: 1,
			wantCaps: []struct {
				ID       string
				Dir      string
				LangName string
			}{
				{ID: "example.com/sub", Dir: "/root/sub", LangName: "go"},
			},
		},
		{
			name: "upward walk finds parent capsule",
			tree: `
├─ go.mod
└─ subdir/
   └─ .gitkeep
`,
			files: map[string]string{
				"/root/go.mod":          "module example.com/root",
				"/root/subdir/.gitkeep": "",
			},
			register: func(d *CapsuleDiscovery) {
				d.Register("go", port.ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
			},
			root:  "/root/subdir",
			wantN: 1,
			wantCaps: []struct {
				ID       string
				Dir      string
				LangName string
			}{
				{ID: "example.com/root", Dir: "/root", LangName: "go"},
			},
		},
		{
			name: "multiple capsules are returned sorted by directory",
			tree: `
├─ a/
│  └─ go.mod
├─ b/
│  └─ go.mod
└─ c/
   └─ go.mod
`,
			files: map[string]string{
				"/root/a/go.mod": "module example.com/a",
				"/root/b/go.mod": "module example.com/b",
				"/root/c/go.mod": "module example.com/c",
			},
			register: func(d *CapsuleDiscovery) {
				d.Register("go", port.ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
			},
			root:  "/root",
			wantN: 3,
			wantCaps: []struct {
				ID       string
				Dir      string
				LangName string
			}{
				{ID: "example.com/a", Dir: "/root/a", LangName: "go"},
				{ID: "example.com/b", Dir: "/root/b", LangName: "go"},
				{ID: "example.com/c", Dir: "/root/c", LangName: "go"},
			},
		},
		{
			name: "capsules from upward and downward phases are both reported",
			tree: `
├─ go.mod
└─ sub/
   └─ go.mod
`,
			files: map[string]string{
				"/root/go.mod":     "module example.com/root",
				"/root/sub/go.mod": "module example.com/sub",
			},
			register: func(d *CapsuleDiscovery) {
				d.Register("go", port.ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
			},
			root:  "/root",
			wantN: 2,
		},
		{
			name: "multiple registered languages select the matching one",
			tree: `
└─ go.mod
`,
			files: map[string]string{
				"/root/go.mod": "module example.com/root",
			},
			register: func(d *CapsuleDiscovery) {
				d.Register("go", port.ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
				d.Register("dart", port.ManifestInfo{Names: []string{"pubspec.yaml"}, ParseFunc: parsePubspec})
			},
			root:  "/root",
			wantN: 1,
			wantCaps: []struct {
				ID       string
				Dir      string
				LangName string
			}{
				{ID: "example.com/root", Dir: "/root", LangName: "go"},
			},
		},
		{
			name: "empty capsule ID from parser is skipped",
			tree: `
└─ go.mod
`,
			files: map[string]string{
				"/root/go.mod": "module ",
			},
			register: func(d *CapsuleDiscovery) {
				d.Register("go", port.ManifestInfo{Names: []string{"go.mod"}, ParseFunc: emptyParser()})
			},
			root:  "/root",
			wantN: 0,
		},
		{
			name: "parser error is ignored when capsule ID is produced",
			tree: `
└─ go.mod
`,
			files: map[string]string{
				"/root/go.mod": "module ",
			},
			register: func(d *CapsuleDiscovery) {
				d.Register("go", port.ManifestInfo{Names: []string{"go.mod"}, ParseFunc: errorParser("example.com/capsule")})
			},
			root:  "/root",
			wantN: 1,
			wantCaps: []struct {
				ID       string
				Dir      string
				LangName string
			}{
				{ID: "example.com/capsule", Dir: "/root", LangName: "go"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := make(map[string]string)
			if tt.files != nil {
				for k, v := range tt.files {
					files[k] = v
				}
			}
			fsys := buildFS(tt.root, tt.tree, files)
			d := NewCapsuleDiscovery()
			tt.register(d)

			entries, err := d.Discover(context.Background(), fsys, tt.root)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entries) != tt.wantN {
				t.Errorf("expected %d capsules, got %d", tt.wantN, len(entries))
			}
			for i, want := range tt.wantCaps {
				if i >= len(entries) {
					break
				}
				got := entries[i]
				if got.Capsule.CapsuleID != want.ID {
					t.Errorf("capsule %d: expected ID %q, got %q", i+1, want.ID, got.Capsule.CapsuleID)
				}
				if got.Capsule.Dir != want.Dir {
					t.Errorf("capsule %d: expected dir %q, got %q", i+1, want.Dir, got.Capsule.Dir)
				}
				if got.LangName != want.LangName {
					t.Errorf("capsule %d: expected language %q, got %q", i+1, want.LangName, got.LangName)
				}
			}
		})
	}
}

// TestCapsuleDiscovery_GlobPattern verifies that manifest names can be glob
// patterns (e.g., *.csproj for C#) and that the first matching file is used.
// Regression: checkManifest only did a literal Stat, so *.csproj never matched.
func TestCapsuleDiscovery_GlobPattern(t *testing.T) {
	fsys := memfs.New()
	files := map[string]string{
		"/root/Api.csproj": `<Project>
  <PropertyGroup>
    <RootNamespace>MyApp.Api</RootNamespace>
  </PropertyGroup>
</Project>`,
		"/root/Domain.csproj": `<Project>
  <PropertyGroup>
    <RootNamespace>MyApp.Domain</RootNamespace>
  </PropertyGroup>
</Project>`,
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	d := NewCapsuleDiscovery()
	d.Register("csharp", port.ManifestInfo{
		Names:     []string{"*.csproj"},
		ParseFunc: csharp.ReadCsprojName,
	})

	entries, err := d.Discover(context.Background(), fsys, "/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 capsule (first matching csproj), got %d", len(entries))
	}

	got := entries[0]
	if got.Capsule.CapsuleID != "MyApp.Api" {
		t.Errorf("expected capsule ID %q (Api.csproj sorted first), got %q", "MyApp.Api", got.Capsule.CapsuleID)
	}
	if got.Capsule.Dir != "/root" {
		t.Errorf("expected dir %q, got %q", "/root", got.Capsule.Dir)
	}
	if got.LangName != "csharp" {
		t.Errorf("expected language %q, got %q", "csharp", got.LangName)
	}
}

// TestCapsuleDiscovery_GlobNoMatch verifies that a glob pattern that doesn't
// match any file in the directory is skipped gracefully.
func TestCapsuleDiscovery_GlobNoMatch(t *testing.T) {
	fsys := memfs.New()
	files := map[string]string{
		"/root/go.mod": "module example.com/root",
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	d := NewCapsuleDiscovery()
	d.Register("go", port.ManifestInfo{
		Names:     []string{"go.mod"},
		ParseFunc: goModParser(),
	})
	d.Register("csharp", port.ManifestInfo{
		Names:     []string{"*.csproj"},
		ParseFunc: csharp.ReadCsprojName,
	})

	entries, err := d.Discover(context.Background(), fsys, "/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 capsule (go), got %d", len(entries))
	}
	if entries[0].LangName != "go" {
		t.Errorf("expected language %q, got %q", "go", entries[0].LangName)
	}
}

// TestCapsuleDiscovery_GlobMultipleMatches returns the first matching file
// in sorted order.
func TestCapsuleDiscovery_GlobMultipleMatchesSortedFirst(t *testing.T) {
	fsys := memfs.New()
	files := map[string]string{
		"/root/Z.csproj": `<Project><PropertyGroup><RootNamespace>Z</RootNamespace></PropertyGroup></Project>`,
		"/root/A.csproj": `<Project><PropertyGroup><RootNamespace>A</RootNamespace></PropertyGroup></Project>`,
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	d := NewCapsuleDiscovery()
	d.Register("csharp", port.ManifestInfo{
		Names:     []string{"*.csproj"},
		ParseFunc: csharp.ReadCsprojName,
	})

	entries, err := d.Discover(context.Background(), fsys, "/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 capsule (first sorted match), got %d", len(entries))
	}

	// A.csproj comes first alphabetically
	if entries[0].Capsule.CapsuleID != "A" {
		t.Errorf("expected capsule ID %q (A.csproj sorted first), got %q", "A", entries[0].Capsule.CapsuleID)
	}
}

// Regression: when a glob matches multiple candidates and the first fails
// to parse (returns empty capsule ID), discovery falls through to the next.
func TestCapsuleDiscovery_GlobParseFailureFallback(t *testing.T) {
	fsys := memfs.New()
	files := map[string]string{
		// A.csproj sorts first but has no valid namespace — parser falls back to "A"
		"/root/A.csproj": `<Project><PropertyGroup><OutputType>Exe</OutputType></PropertyGroup></Project>`,
		// B.csproj has a proper namespace
		"/root/B.csproj": `<Project><PropertyGroup><RootNamespace>MyApp</RootNamespace></PropertyGroup></Project>`,
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	d := NewCapsuleDiscovery()
	d.Register("csharp", port.ManifestInfo{
		Names:     []string{"*.csproj"},
		ParseFunc: csharp.ReadCsprojName,
	})

	entries, err := d.Discover(context.Background(), fsys, "/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A.csproj sorts first and returns "A" (filename fallback), so it's picked.
	if len(entries) != 1 {
		t.Fatalf("expected 1 capsule, got %d", len(entries))
	}
	if entries[0].Capsule.CapsuleID != "A" {
		t.Errorf("expected capsule ID %q (A.csproj fallback), got %q", "A", entries[0].Capsule.CapsuleID)
	}
}

// Regression: verify that a parse function returning an error (not empty string)
// causes the candidate to be skipped, allowing the next candidate to be tried.
func TestCapsuleDiscovery_GlobParseErrorSkipsCandidate(t *testing.T) {
	fsys := memfs.New()
	files := map[string]string{
		"/root/A.config": "invalid",
		"/root/B.config": "valid",
	}
	for path, content := range files {
		if err := fsys.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	failThenPass := func(fsys port.FileSystem, path string) (string, error) {
		if filepath.Base(path) == "A.config" {
			return "", fmt.Errorf("parse error")
		}
		return "valid-id", nil
	}

	d := NewCapsuleDiscovery()
	d.Register("testlang", port.ManifestInfo{
		Names:     []string{"*.config"},
		ParseFunc: failThenPass,
	})

	entries, err := d.Discover(context.Background(), fsys, "/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 capsule (B.config, since A.config errors), got %d", len(entries))
	}
	if entries[0].Capsule.CapsuleID != "valid-id" {
		t.Errorf("expected capsule ID %q, got %q", "valid-id", entries[0].Capsule.CapsuleID)
	}
}

func TestCapsuleDiscoveryBaseIgnoreEntries(t *testing.T) {
	d := NewCapsuleDiscovery()

	if len(d.BaseIgnoreEntries()) != 0 {
		t.Errorf("expected empty base ignore entries initially, got %d", len(d.BaseIgnoreEntries()))
	}

	d.Register("go", port.ManifestInfo{BaseIgnoreEntries: []string{"vendor"}})
	d.Register("typescript", port.ManifestInfo{BaseIgnoreEntries: []string{"node_modules"}})
	d.Register("rust", port.ManifestInfo{BaseIgnoreEntries: []string{"target"}})

	entries := d.BaseIgnoreEntries()
	if len(entries) != 3 {
		t.Errorf("expected 3 base ignore entries, got %d", len(entries))
	}
	for _, dir := range []string{"vendor", "node_modules", "target"} {
		if !entries[dir] {
			t.Errorf("expected %q to be in base ignore entries", dir)
		}
	}

	d.Register("go", port.ManifestInfo{BaseIgnoreEntries: []string{"vendor"}})
	if len(d.BaseIgnoreEntries()) != 3 {
		t.Errorf("expected 3 base ignore entries after duplicate registration, got %d", len(d.BaseIgnoreEntries()))
	}

	d.Register("dart", port.ManifestInfo{BaseIgnoreEntries: []string{".dart_tool", ".pub"}})
	if len(d.BaseIgnoreEntries()) != 5 {
		t.Errorf("expected 5 base ignore entries after adding dart dirs, got %d", len(d.BaseIgnoreEntries()))
	}
	for _, dir := range []string{".dart_tool", ".pub"} {
		if !d.BaseIgnoreEntries()[dir] {
			t.Errorf("expected %q to be in base ignore entries", dir)
		}
	}
}
