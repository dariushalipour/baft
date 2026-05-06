package service

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dariushalipour/strata/internal/adapter/fs/memfs"
	"github.com/dariushalipour/strata/internal/port"
	"github.com/dariushalipour/strata/pkg/treeview"
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
		_ = fsys.WriteFile(absPath, []byte(files[absPath]), 0644)
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
				d.Register("go", ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
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
				d.Register("go", ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
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
				d.Register("go", ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
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
				d.Register("go", ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
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
				d.Register("go", ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
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
				d.Register("go", ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
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
				d.Register("go", ManifestInfo{Names: []string{"go.mod"}, ParseFunc: goModParser()})
				d.Register("dart", ManifestInfo{Names: []string{"pubspec.yaml"}, ParseFunc: parsePubspec})
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
				d.Register("go", ManifestInfo{Names: []string{"go.mod"}, ParseFunc: emptyParser()})
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
				d.Register("go", ManifestInfo{Names: []string{"go.mod"}, ParseFunc: errorParser("example.com/capsule")})
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

			entries, err := d.Discover(fsys, tt.root)
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
