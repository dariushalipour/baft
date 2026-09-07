package python

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/adapter/fs/realfs"
	"github.com/dariushalipour/baft/internal/port"
)

func TestName(t *testing.T) {
	if got := (Language{}).Name(); got != "python" {
		t.Errorf("Name() = %q, want %q", got, "python")
	}
}

func TestSupportsFileGlobs(t *testing.T) {
	if (Language{}).SupportsFileGlobs() {
		t.Error("expected false")
	}
}

func TestIsScannableFile(t *testing.T) {
	l := Language{}
	tests := []struct {
		rel  string
		want bool
	}{
		{"foo/bar.py", true},
		{"foo/bar/__init__.py", true},
		{"foo/bar.pyi", true},
		{"foo/bar/__init__.pyi", true},
		{"foo/bar.txt", false},
		{"foo/bar.pyc", false},
		{"foo/bar.pyx", false},
	}
	for _, tt := range tests {
		got := l.IsScannableFile(tt.rel)
		if got != tt.want {
			t.Errorf("IsScannableFile(%q) = %v, want %v", tt.rel, got, tt.want)
		}
	}
}

func TestParseImports(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os\n"+
			"from collections import OrderedDict\n"+
			"import json\n"+
			"from typing import List, Dict\n"+
			"from mypackage.utils import helper\n"+
			"import mypackage.services.auth\n"+
			"from . import sibling\n"+
			"from .local import thing\n"+
			"from ..parent import value\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	want := []struct {
		path string
		line int
		col  int
	}{
		{"os", 1, 8},
		{"collections", 2, 6},
		{"json", 3, 8},
		{"typing", 4, 6},
		{"mypackage.utils", 5, 6},
		{"mypackage.services.auth", 6, 8},
		{".sibling", 7, 14},
		{".local", 8, 6},
		{"..parent", 9, 6},
	}

	if len(specs) != len(want) {
		t.Fatalf("got %d specs, want %d", len(specs), len(want))
	}

	for i, w := range want {
		got := specs[i]
		if got.Path != w.path {
			t.Errorf("[%d] Path = %q, want %q", i, got.Path, w.path)
		}
		if got.Line != w.line {
			t.Errorf("[%d] Line = %d, want %d", i, got.Line, w.line)
		}
		if got.Col != w.col {
			t.Errorf("[%d] Col = %d, want %d", i, got.Col, w.col)
		}
		if got.ColEnd != w.col+len(w.path) {
			t.Errorf("[%d] ColEnd = %d, want %d", i, got.ColEnd, w.col+len(w.path))
		}
	}
}

func TestParseImports_CommaSeparated(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os, sys, json\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	for _, want := range []string{"os", "sys", "json"} {
		if !paths[want] {
			t.Errorf("missing import %q (got %v)", want, specs)
		}
	}
}

func TestParseImports_Parenthesized(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import (\n"+
			"    os,\n"+
			"    sys,\n"+
			")\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	for _, want := range []string{"os", "sys"} {
		if !paths[want] {
			t.Errorf("missing import %q (got %v)", want, specs)
		}
	}
}

func TestParseImports_FromParenthesized(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"from mypackage.utils import (\n"+
			"    helper,\n"+
			"    formatter,\n"+
			")\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if !paths["mypackage.utils"] {
		t.Errorf("missing import %q (got %v)", "mypackage.utils", specs)
	}
}

func TestParseImports_SkipsComments(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os\n"+
			"# import fake_module\n"+
			"import sys\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if paths["fake_module"] {
		t.Error("should not parse import inside comment")
	}
	if !paths["os"] || !paths["sys"] {
		t.Errorf("missing expected imports (got %v)", specs)
	}
}

func TestParseImports_SkipsStringLiterals(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os\n"+
			"doc = \"\"\"import fake_module\"\"\"\n"+
			"import sys\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if paths["fake_module"] {
		t.Error("should not parse import inside string literal")
	}
	if !paths["os"] || !paths["sys"] {
		t.Errorf("missing expected imports (got %v)", specs)
	}
}

func TestParseImports_SkipsMultilineStrings(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os\n"+
			"doc = \"\"\"\n"+
			"import fake_module\n"+
			"\"\"\"\n"+
			"import sys\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if paths["fake_module"] {
		t.Error("should not parse import inside multiline string")
	}
	if !paths["os"] || !paths["sys"] {
		t.Errorf("missing expected imports (got %v)", specs)
	}
}

func TestParseImports_SkipsFStringLiteral(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os\n"+
			"msg = f\"\"\"import fake_module\"\"\"\n"+
			"import sys\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if paths["fake_module"] {
		t.Error("should not parse import inside f-string literal")
	}
	if !paths["os"] || !paths["sys"] {
		t.Errorf("missing expected imports (got %v)", specs)
	}
}

func TestParseImports_SkipsMultilineFString(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os\n"+
			"msg = f\"\"\"\n"+
			"import fake_module\n"+
			"\"\"\"\n"+
			"import sys\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if paths["fake_module"] {
		t.Error("should not parse import inside multiline f-string")
	}
	if !paths["os"] || !paths["sys"] {
		t.Errorf("missing expected imports (got %v)", specs)
	}
}

func TestParseImports_Aliased(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import numpy as np\n"+
			"import pandas as pd\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	for _, want := range []string{"numpy", "pandas"} {
		if !paths[want] {
			t.Errorf("missing import %q (got %v)", want, specs)
		}
	}
}

func TestResolveInternalTarget(t *testing.T) {
	l := Language{}
	capsule := port.Capsule{CapsuleID: "mypackage"}

	tests := []struct {
		spec    port.ImportSpec
		fileRel string
		want    string
		ok      bool
	}{
		{port.ImportSpec{Path: "mypackage.services"}, "src/mypackage/user.py", "src/mypackage/services", true},
		{port.ImportSpec{Path: "mypackage"}, "src/mypackage/user.py", "src/mypackage", true},
		{port.ImportSpec{Path: "mypackage.api.routes"}, "src/mypackage/user.py", "src/mypackage/api/routes", true},
		{port.ImportSpec{Path: "os.path"}, "src/mypackage/user.py", "", false},
		{port.ImportSpec{Path: "mypackage"}, "mypackage/user.py", "mypackage", true},
		{port.ImportSpec{Path: "mypackage.deep.module.sub"}, "src/mypackage/user.py", "src/mypackage/deep/module/sub", true},
		// Relative imports
		{port.ImportSpec{Path: "."}, "src/mypackage/sub/user.py", "src/mypackage/sub", true},
		{port.ImportSpec{Path: ".."}, "src/mypackage/sub/user.py", "src/mypackage", true},
		{port.ImportSpec{Path: ".local"}, "src/mypackage/sub/user.py", "src/mypackage/sub/local", true},
		{port.ImportSpec{Path: "..other"}, "src/mypackage/sub/user.py", "src/mypackage/other", true},
		{port.ImportSpec{Path: "..other.module"}, "src/mypackage/sub/user.py", "src/mypackage/other/module", true},
		{port.ImportSpec{Path: ".sibling"}, "mypackage/sub/user.py", "mypackage/sub/sibling", true},
		{port.ImportSpec{Path: "..top"}, "mypackage/sub/user.py", "mypackage/top", true},
		{port.ImportSpec{Path: "..."}, "mypackage/sub/deep/user.py", "mypackage", true},
		{port.ImportSpec{Path: "...."}, "mypackage/sub/deep/user.py", "", false},
	}

	for _, tt := range tests {
		got, ok := l.ResolveInternalTarget(nil, tt.spec, capsule, tt.fileRel)
		if ok != tt.ok || got != tt.want {
			t.Errorf("ResolveInternalTarget(%q, %q) = (%q, %v), want (%q, %v)", tt.spec.Path, tt.fileRel, got, ok, tt.want, tt.ok)
		}
	}
}

func TestIsInternalCapsule(t *testing.T) {
	tests := []struct {
		spec string
		pkg  string
		want bool
	}{
		{"mypackage", "mypackage", true},
		{"mypackage.services", "mypackage", true},
		{"mypackage.api.routes.v2", "mypackage", true},
		{"mypackage_extra", "mypackage", false},
		{"mypackages.auth", "mypackage", false},
		{"other.services", "mypackage", false},
		{"mypackageservices.foo", "mypackage", false},
		{"mypackage.", "mypackage", false},
		{"mypackage.x.", "mypackage", false},
	}
	for _, tt := range tests {
		got := isInternalCapsule(tt.spec, tt.pkg)
		if got != tt.want {
			t.Errorf("isInternalCapsule(%q, %q) = %v, want %v", tt.spec, tt.pkg, got, tt.want)
		}
	}
}

func TestResolveSourcePrefix(t *testing.T) {
	tests := []struct {
		fileRel string
		want    string
	}{
		{"src/mypackage/user.py", "src"},
		{"src/mypackage/deep/module/user.py", "src"},
		{"python/mypackage/user.py", "python"},
		{"lib/mypackage/user.py", "lib"},
		{"mypackage/user.py", ""},
		{"mypackage/deep/user.py", ""},
		{"mypackage/lib/user.py", ""},
		{"src/mypackage/lib/user.py", "src"},
	}
	for _, tt := range tests {
		got := resolveSourcePrefix(tt.fileRel)
		if got != tt.want {
			t.Errorf("resolveSourcePrefix(%q) = %q, want %q", tt.fileRel, got, tt.want)
		}
	}
}

func TestDiscover_ManifestTypes(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantCaps string
	}{
		{
			name: "empty dir no capsules",
			files: map[string]string{
				"/project/README.md": "hello",
			},
		},
		{
			name: "pyproject.toml",
			files: map[string]string{
				"/project/pyproject.toml":          "[project]\nname = \"myapp\"\n",
				"/project/src/myapp/__init__.py":   "",
				"/project/src/myapp/core.py":       "x = 1",
				"/project/src/myapp/api/routes.py": "from flask import Flask\n",
			},
			wantCaps: "myapp",
		},
		{
			name: "setup.py",
			files: map[string]string{
				"/project/setup.py":              "from setuptools import setup\nsetup(name=\"myapp\")\n",
				"/project/src/myapp/__init__.py": "",
				"/project/src/myapp/core.py":     "x = 1",
			},
			wantCaps: "myapp",
		},
		{
			name: "flat layout with pyproject.toml",
			files: map[string]string{
				"/project/pyproject.toml":    "[project]\nname = \"myapp\"\n",
				"/project/myapp/__init__.py": "",
				"/project/myapp/core.py":     "x = 1",
			},
			wantCaps: "myapp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := memfs.New()
			for path, content := range tt.files {
				fs.WriteFile(path, []byte(content), 0o644)
			}
			var found bool
			d := &mockDiscovery{
				onRegister: func(name string, info port.ManifestInfo) {
					if name != "python" {
						return
					}
					found = true
					for _, manifestName := range info.Names {
						manifestPath := "/project/" + manifestName
						if _, err := fs.Stat(manifestPath); err == nil {
							capsuleID, err := info.ParseFunc(fs, manifestPath)
							if err != nil {
								t.Errorf("parse %s: %v", manifestName, err)
							}
							if capsuleID != tt.wantCaps {
								t.Errorf("capsule from %s = %q, want %q", manifestName, capsuleID, tt.wantCaps)
							}
							return
						}
					}
				},
			}
			Language{}.Register(d)
			if !found {
				t.Error("python not registered")
			}
		})
	}
}

func TestFindBaseCapsule(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantCaps  string
		wantError bool
	}{
		{
			name: "src layout",
			files: map[string]string{
				"/project/src/foo/__init__.py":   "",
				"/project/src/foo/bar.py":        "x = 1",
				"/project/src/foo/api/routes.py": "y = 2",
			},
			wantCaps: "foo",
		},
		{
			name: "flat layout",
			files: map[string]string{
				"/project/foo/__init__.py": "",
				"/project/foo/bar.py":      "x = 1",
			},
			wantCaps: "foo",
		},
		{
			name: "nested package",
			files: map[string]string{
				"/project/src/foo/__init__.py":      "",
				"/project/src/foo/bar/__init__.py":  "",
				"/project/src/foo/bar/baz.py":       "x = 1",
				"/project/src/foo/bar/qux/route.py": "y = 2",
			},
			wantCaps: "foo",
		},
		{
			name: "no python source",
			files: map[string]string{
				"/project/README.md": "hello",
			},
			wantCaps: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := memfs.New()
			for path, content := range tt.files {
				fs.WriteFile(path, []byte(content), 0o644)
			}
			got, err := findBaseCapsule(fs, "project")
			if (err != nil) != tt.wantError {
				t.Errorf("err = %v, wantError = %v", err, tt.wantError)
			}
			if got != tt.wantCaps {
				t.Errorf("got %q, want %q", got, tt.wantCaps)
			}
		})
	}
}

func TestDiscover(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/capsule/pyproject.toml", []byte("[project]\nname = \"mymodule\"\n"), 0o644)
	fs.WriteFile("/capsule/src/mymodule/__init__.py", []byte(""), 0o644)
	fs.WriteFile("/capsule/src/mymodule/handler.py", []byte("import os\nfrom mymodule.utils import parse\n"), 0o644)

	fs.WriteFile("/other/pyproject.toml", []byte("[project]\nname = \"other\"\n"), 0o644)
	fs.WriteFile("/other/src/other/__init__.py", []byte(""), 0o644)
	fs.WriteFile("/other/src/other/main.py", []byte("from other.cli import run\n"), 0o644)

	var capsules []port.Capsule
	d := &mockDiscovery{
		onRegister: func(name string, info port.ManifestInfo) {
			if name != "python" {
				return
			}
			manifests := []string{"/capsule/pyproject.toml", "/other/pyproject.toml"}
			for _, mp := range manifests {
				if _, err := fs.Stat(mp); err == nil {
					capsuleID, err := info.ParseFunc(fs, mp)
					if err != nil {
						t.Errorf("parse %s: %v", mp, err)
					}
					dir := filepath.Dir(mp)
					capsules = append(capsules, port.Capsule{Dir: dir, CapsuleID: capsuleID})
				}
			}
		},
	}
	Language{}.Register(d)

	if len(capsules) != 2 {
		t.Fatalf("got %d capsules, want 2", len(capsules))
	}

	capsuleMap := make(map[string]string)
	for _, c := range capsules {
		capsuleMap[c.Dir] = c.CapsuleID
	}

	if capsuleMap["/capsule"] != "mymodule" {
		t.Errorf("capsule dir = %q, want mymodule", capsuleMap["/capsule"])
	}
	if capsuleMap["/other"] != "other" {
		t.Errorf("other dir = %q, want other", capsuleMap["/other"])
	}
}

func TestDiscover_SkipsBuildDirs(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/pyproject.toml", []byte("[project]\nname = \"pkg\"\n"), 0o644)
	fs.WriteFile("/project/src/pkg/__init__.py", []byte(""), 0o644)
	fs.WriteFile("/project/src/pkg/main.py", []byte("x = 1"), 0o644)
	fs.WriteFile("/project/src/pkg/__pycache__/main.cpython-311.pyc", []byte("cached"), 0o644)

	capsuleID, err := findBaseCapsule(fs, "project")
	if err != nil {
		t.Fatal(err)
	}
	if capsuleID != "pkg" {
		t.Errorf("got %q, want pkg", capsuleID)
	}
}

func TestParseImports_FromModuleParenthesized(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"from mypackage import (\n"+
			"    module_a,\n"+
			"    module_b,\n"+
			")\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if !paths["mypackage"] {
		t.Errorf("missing import %q (got %v)", "mypackage", specs)
	}
}

func TestParseImports_Relative(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"from . import sibling\n"+
			"from .local import thing\n"+
			"from ..parent import value\n"+
			"from ..other.module import func\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	for _, want := range []string{".sibling", ".local", "..parent", "..other.module"} {
		if !paths[want] {
			t.Errorf("missing relative import %q (got %v)", want, specs)
		}
	}
}

func TestFindBaseCapsule_StubPackage(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/src/mypkg/__init__.pyi", []byte("__all__ = ['foo']"), 0o644)
	fs.WriteFile("/project/src/mypkg/core.pyi", []byte("def foo() -> int: ..."), 0o644)
	fs.WriteFile("/project/src/mypkg/utils/helpers.pyi", []byte("def bar() -> str: ..."), 0o644)

	got, err := findBaseCapsule(fs, "project")
	if err != nil {
		t.Fatal(err)
	}
	if got != "mypkg" {
		t.Errorf("got %q, want mypkg", got)
	}
}

type mockDiscovery struct {
	onRegister func(string, port.ManifestInfo)
}

func (m *mockDiscovery) Register(name string, info port.ManifestInfo) {
	m.onRegister(name, info)
}

func TestParseImports_SkipsConsecutiveTripleQuotes(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os\n"+
			"x = \"\"\"hello\"\"\" + \"\"\"world\"\"\" + \"\"\"foo\"\"\"\n"+
			"import sys\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if !paths["os"] || !paths["sys"] {
		t.Errorf("missing expected imports (got %v)", specs)
	}
	if len(specs) != 2 {
		t.Errorf("got %d specs, want 2", len(specs))
	}
}

func TestParseImports_FunctionCallNotImport(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os\n"+
			"import_something(\n"+
			"    \"arg1\",\n"+
			"    \"arg2\",\n"+
			")\n"+
			"import sys\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if !paths["os"] || !paths["sys"] {
		t.Errorf("missing expected imports (got %v)", specs)
	}
	if len(specs) != 2 {
		t.Errorf("got %d specs, want 2 (function call should not be parsed as import)", len(specs))
	}
}

func TestParseImports_MixedTripleQuoteTypes(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os\n"+
			"\"\"\"a\"\"\" + '''b'''\n"+
			"import sys\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if !paths["os"] || !paths["sys"] {
		t.Errorf("missing expected imports (got %v)", specs)
	}
}

func TestParseImports_CommaSeparatedWithAlias(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"import os, sys as s, json\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	for _, want := range []string{"os", "sys", "json"} {
		if !paths[want] {
			t.Errorf("missing import %q (got %v)", want, specs)
		}
	}
	if paths["as"] {
		t.Error("'as' keyword should not be parsed as an import")
	}
	if len(specs) != 3 {
		t.Errorf("got %d specs, want 3 (got %v)", len(specs), specs)
	}
}

func TestParseImports_ParenImportWithMultilineString(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"from foo import (\n"+
			"    os,\n"+
			"    sys,\n"+
			")\n"+
			"doc = \"\"\"\n"+
			")\n"+
			"\"\"\"\n"+
			"import json\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	if !paths["foo"] {
		t.Errorf("missing import %q (got %v)", "foo", specs)
	}
	if !paths["json"] {
		t.Errorf("missing import %q (got %v)", "json", specs)
	}
}

func TestParseImports_BareRelativeImport(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"from . import sibling, other\n"+
			"from .. import parent_mod\n",
	), 0o644)

	l := Language{}
	specs, err := l.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	paths := make(map[string]bool)
	for _, s := range specs {
		paths[s.Path] = true
	}
	for _, want := range []string{".sibling", ".other", "..parent_mod"} {
		if !paths[want] {
			t.Errorf("missing bare relative import %q (got %v)", want, specs)
		}
	}
	if paths["."] || paths[".."] {
		t.Error("standalone dot-only paths should not appear in results")
	}
}

func TestParseImports_ParenthesizedClosingOnSameLine(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/project/foo.py", []byte(
		"from mypackage.api import (Handler,\n"+
			"    Router)\n"+
			"x = 1\n"+
			"from mypackage.domain import Model\n"+
			"import json\n",
	), 0o644)

	specs, err := Language{}.ParseImports(fs, "/project/foo.py")
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	lines := make(map[string]int)
	for _, s := range specs {
		lines[s.Path] = s.Line
	}
	for _, want := range []struct {
		path string
		line int
	}{{"mypackage.api", 1}, {"mypackage.domain", 4}, {"json", 5}} {
		if got := lines[want.path]; got != want.line {
			t.Errorf("import %q on line %d, want line %d (got %v)", want.path, got, want.line, specs)
		}
	}
}

// TestFindBaseCapsule_RealFS pins capsule discovery against the real file
// system: an absolute project root must not be turned into a relative walk root.
func TestFindBaseCapsule_RealFS(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("pyproject.toml", "[project]\nname = \"app\"\n")
	write("src/app/__init__.py", "")
	write("src/app/api/h.py", "from app.domain import Model\n")
	write("src/app/domain/model.py", "Model = object\n")

	got, err := findBaseCapsule(realfs.New(), dir)
	if err != nil {
		t.Fatalf("findBaseCapsule: %v", err)
	}
	if got != "app" {
		t.Errorf("got %q, want app", got)
	}
}
