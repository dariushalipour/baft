package typescript

import (
	"sort"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

func TestIsScannableFile(t *testing.T) {
	l := Language{}
	cases := map[string]bool{
		// .ts and .tsx are scannable
		"src/app.ts":                true,
		"src/components/Button.tsx": true,
		"src/deep/nested/ok.tsx":    true,
		"src/sub/module/index.ts":   true,
		"src/app.test.ts":           true,
		"src/app.spec.ts":           true,
		"src/app.test.tsx":          true,
		"src/app.spec.tsx":          true,

		// .d.ts / .d.tsx declaration files are also scannable
		"src/models/user.d.ts":  true,
		"src/models/user.d.tsx": true,

		// .js, .jsx, .json and everything else are not scannable
		"src/app.js":   false,
		"src/app.jsx":  false,
		"src/app.json": false,
		"src/app.md":   false,
	}
	for rel, want := range cases {
		if got := l.IsScannableFile(rel); got != want {
			t.Errorf("IsScannableFile(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestParseImports(t *testing.T) {
	src := `// header
import { useEffect } from 'react';
import { useState } from "react";
import { formatDate } from '../lib/utils/format';
import { User } from '@baft/models/user';
export { helper } from './helper';
export * from './utils';
import axios from 'axios';

// import commented from 'commented-out';
`
	fs := memfs.New()
	fs.WriteFile("/sample.ts", []byte(src), 0o644)
	got, err := (&Language{}).ParseImports(fs, "/sample.ts")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"react",
		"../lib/utils/format",
		"@baft/models/user",
		"./helper",
		"./utils",
		"axios",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i].Path != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i].Path, want[i])
		}
	}
}

func TestParseImportsAllPatterns(t *testing.T) {
	src := `
// Static imports
import defaultExport from './module';
import defaultExport from './module.js';
import defaultExport from './module.ts';
import defaultExport from './module.tsx';
import defaultExport from './module.jsx';
import defaultExport from './module.mjs';
import defaultExport from './module.mts';
import defaultExport from './module.cjs';
import defaultExport from './module.cts';
import defaultExport from './module.json';
import defaultExport from './module.node';
import { named } from './named';
import { named as alias } from './aliased';
import defaultExport, { named } from './mixed';
import * as ns from './namespace';
import './side-effect';
import type { MyType } from './type-import';
import type defaultType from './type-default';
import { type MyType } from './inline-type';

// Re-exports
export { x } from './reexport';
export { x as alias } from './reexport-alias';
export * from './reexport-star';
export * as ns from './reexport-ns';
export { default } from './reexport-default';
export { default as name } from './reexport-default-alias';
export type { MyType } from './export-type';

// Dynamic imports
const a = await import('./dynamic');
const b = import('./dynamic-promise').then(m => m);

// CommonJS
const c = require('./commonjs');

// TypeScript import = require
import Foo = require('./import-require');
`
	fs := memfs.New()
	fs.WriteFile("/sample.ts", []byte(src), 0o644)
	got, err := (&Language{}).ParseImports(fs, "/sample.ts")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"./module",
		"./module.js",
		"./module.ts",
		"./module.tsx",
		"./module.jsx",
		"./module.mjs",
		"./module.mts",
		"./module.cjs",
		"./module.cts",
		"./module.json",
		"./module.node",
		"./named",
		"./aliased",
		"./mixed",
		"./namespace",
		"./side-effect",
		"./type-import",
		"./type-default",
		"./inline-type",
		"./reexport",
		"./reexport-alias",
		"./reexport-star",
		"./reexport-ns",
		"./reexport-default",
		"./reexport-default-alias",
		"./export-type",
		"./import-require",
		"./dynamic",
		"./dynamic-promise",
		"./commonjs",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d, want %d\n\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	// Sort both slices for comparison since the combined regex returns in source order.
	gotPaths := make([]string, len(got))
	for i, imp := range got {
		gotPaths[i] = imp.Path
	}
	sort.Strings(gotPaths)
	sort.Strings(want)
	for i := range want {
		if gotPaths[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, gotPaths[i], want[i])
		}
	}
}

func TestResolveInternalTarget(t *testing.T) {
	l := &Language{}

	t.Run("relative imports", func(t *testing.T) {
		capsule := port.Capsule{CapsuleID: "@baft/app"}
		cases := []struct {
			spec     string
			fileRel  string
			wantPath string
			wantIntl bool
		}{
			{"../lib/utils/format", "src/components/Button.tsx", "src/lib/utils/format", true},
			{"./helper", "src/components/Button.tsx", "src/components/helper", true},
			{"../../outside", "src/app.ts", "", false},
			{"./sub/module", "src/app.ts", "src/sub/module", true},
		}
		for _, c := range cases {
			gotPath, gotIntl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: c.spec}, capsule, c.fileRel)
			if gotPath != c.wantPath || gotIntl != c.wantIntl {
				t.Errorf("ResolveInternalTarget(%q, file=%q) = (%q, %v), want (%q, %v)",
					c.spec, c.fileRel, gotPath, gotIntl, c.wantPath, c.wantIntl)
			}
		}
	})

	t.Run("package name imports", func(t *testing.T) {
		capsule := port.Capsule{CapsuleID: "@baft/app"}
		cases := []struct {
			spec     string
			fileRel  string
			wantPath string
			wantIntl bool
		}{
			{"react", "src/app.ts", "", false},
			{"axios", "src/app.ts", "", false},
			{"@baft/app/lib/utils.ts", "src/app.ts", "src/lib/utils.ts", true},
			{"@baft/app/components/Button", "src/app.ts", "src/components/Button", true},
			{"@other/pkg", "src/app.ts", "", false},
		}
		for _, c := range cases {
			gotPath, gotIntl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: c.spec}, capsule, c.fileRel)
			if gotPath != c.wantPath || gotIntl != c.wantIntl {
				t.Errorf("ResolveInternalTarget(%q, file=%q) = (%q, %v), want (%q, %v)",
					c.spec, c.fileRel, gotPath, gotIntl, c.wantPath, c.wantIntl)
			}
		}
	})
}

func TestResolveInternalTargetWithTsconfig(t *testing.T) {
	t.Run("paths with baseUrl", func(t *testing.T) {
		l := &Language{}
		tsconfig := `{
			"compilerOptions": {
				"baseUrl": "src",
				"paths": {
					"@lib/*": ["lib/*"],
					"@components/*": ["components/*"],
					"@utils": ["lib/utils"]
				}
			}
		}`
		fs := memfs.New()
		fs.WriteFile("/tsconfig.json", []byte(tsconfig), 0o644)
		capsule := port.Capsule{CapsuleID: "my-app", Dir: "/"}

		cases := []struct {
			spec     string
			fileRel  string
			wantPath string
			wantIntl bool
		}{
			{"@lib/utils", "src/app.ts", "src/lib/utils", true},
			{"@lib/helpers/format", "src/app.ts", "src/lib/helpers/format", true},
			{"@components/Button", "src/app.ts", "src/components/Button", true},
			{"@utils", "src/app.ts", "src/lib/utils", true},
			{"react", "src/app.ts", "", false},
		}
		for _, c := range cases {
			gotPath, gotIntl := l.ResolveInternalTarget(fs, port.ImportSpec{Path: c.spec}, capsule, c.fileRel)
			if gotPath != c.wantPath || gotIntl != c.wantIntl {
				t.Errorf("ResolveInternalTarget(%q, file=%q) = (%q, %v), want (%q, %v)",
					c.spec, c.fileRel, gotPath, gotIntl, c.wantPath, c.wantIntl)
			}
		}
	})

	t.Run("paths without baseUrl", func(t *testing.T) {
		l := &Language{}
		tsconfig := `{
			"compilerOptions": {
				"paths": {
					"@app/*": ["src/*"]
				}
			}
		}`
		fs := memfs.New()
		fs.WriteFile("/tsconfig.json", []byte(tsconfig), 0o644)
		capsule := port.Capsule{CapsuleID: "my-app", Dir: "/"}

		gotPath, gotIntl := l.ResolveInternalTarget(fs, port.ImportSpec{Path: "@app/lib/utils"}, capsule, "src/app.ts")
		if gotPath != "src/lib/utils" || !gotIntl {
			t.Errorf("got (%q, %v), want (src/lib/utils, true)", gotPath, gotIntl)
		}
	})

	t.Run("extends parent tsconfig", func(t *testing.T) {
		l := &Language{}
		parentTsconfig := `{
			"compilerOptions": {
				"baseUrl": ".",
				"paths": {
					"@shared/*": ["shared/*"]
				}
			}
		}`
		childTsconfig := `{
			"extends": "/tsconfig.base.json",
			"compilerOptions": {
				"paths": {
					"@src/*": ["src/*"]
				}
			}
		}`
		fs := memfs.New()
		fs.WriteFile("/tsconfig.base.json", []byte(parentTsconfig), 0o644)
		fs.WriteFile("/tsconfig.json", []byte(childTsconfig), 0o644)

		capsule := port.Capsule{CapsuleID: "my-app", Dir: "/"}

		gotPath, gotIntl := l.ResolveInternalTarget(fs, port.ImportSpec{Path: "@src/lib/utils"}, capsule, "src/app.ts")
		if gotPath != "src/lib/utils" || !gotIntl {
			t.Errorf("child path: got (%q, %v), want (src/lib/utils, true)", gotPath, gotIntl)
		}

		gotPath, gotIntl = l.ResolveInternalTarget(fs, port.ImportSpec{Path: "@shared/types"}, capsule, "src/app.ts")
		if gotPath != "shared/types" || !gotIntl {
			t.Errorf("parent path: got (%q, %v), want (shared/types, true)", gotPath, gotIntl)
		}
	})

	t.Run("no tsconfig falls back to package name", func(t *testing.T) {
		l := &Language{}
		capsule := port.Capsule{CapsuleID: "@my/app", Dir: "/"}

		gotPath, gotIntl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: "@my/app/lib/utils"}, capsule, "src/app.ts")
		if gotPath != "src/lib/utils" || !gotIntl {
			t.Errorf("got (%q, %v), want (src/lib/utils, true)", gotPath, gotIntl)
		}
	})
}

func TestReadPackageName(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/package.json", []byte(`{"name": "@baft/app", "version": "1.0.0"}`), 0o644)
	got, err := readCapsuleName(fs, "/package.json")
	if err != nil {
		t.Fatal(err)
	}
	if got != "@baft/app" {
		t.Fatalf("got %q", got)
	}
}

func TestReadPackageNameNoName(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/package.json", []byte(`{"type": "module"}`), 0o644)
	got, err := readCapsuleName(fs, "/package.json")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestReadPackageNameEmptyName(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/package.json", []byte(`{"name": ""}`), 0o644)
	got, err := readCapsuleName(fs, "/package.json")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestDiscoverSkipsNamelessPackage(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/features/package.json", []byte(`{"type": "module"}`), 0o644)
	fs.WriteFile("/features/BAFT.md", []byte("# Features"), 0o644)
	fs.WriteFile("/core/package.json", []byte(`{"name": "@baft/core"}`), 0o644)
	fs.WriteFile("/core/BAFT.md", []byte("# Core"), 0o644)

	disco := service.NewCapsuleDiscovery()
	(&Language{}).Register(disco)
	got, err := disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 package, got %d", len(got))
	}

	if port.Label(got[0].Capsule) != got[0].Capsule.Dir {
		t.Errorf("expected label %q, got %q", got[0].Capsule.Dir, port.Label(got[0].Capsule))
	}

	if got[0].Capsule.CapsuleID != "@baft/core" {
		t.Errorf("expected capsuleID \"@baft/core\", got %q", got[0].Capsule.CapsuleID)
	}
}

func TestDiscoverDraftSkipsNamelessPackage(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/features/package.json", []byte(`{"type": "module"}`), 0o644)
	fs.WriteFile("/core/package.json", []byte(`{"name": "@baft/core"}`), 0o644)

	disco := service.NewCapsuleDiscovery()
	(&Language{}).Register(disco)
	got, err := disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 package, got %d", len(got))
	}

	if port.Label(got[0].Capsule) != got[0].Capsule.Dir {
		t.Errorf("expected label %q, got %q", got[0].Capsule.Dir, port.Label(got[0].Capsule))
	}
}

func TestDiscoverAllNamelessSkipped(t *testing.T) {
	fs := memfs.New()
	for _, name := range []string{"features", "utils"} {
		fs.WriteFile("/"+name+"/package.json", []byte(`{"type": "module"}`), 0o644)
		fs.WriteFile("/"+name+"/BAFT.md", []byte("# "+name), 0o644)
	}

	disco := service.NewCapsuleDiscovery()
	(&Language{}).Register(disco)
	got, err := disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(got))
	}
}

func TestMatchPath(t *testing.T) {
	cases := []struct {
		pattern string
		spec    string
		want    bool
	}{
		{"@lib/*", "@lib/utils", true},
		{"@lib/*", "@lib/helpers/format", true},
		{"@lib/*", "@other/utils", false},
		{"@lib", "@lib", true},
		{"@lib", "@lib/utils", false},
		{"@app/*", "@app/components/Button", true},
		{"*", "anything", true},
	}
	for _, c := range cases {
		got := matchPath(c.pattern, c.spec)
		if got != c.want {
			t.Errorf("matchPath(%q, %q) = %v, want %v", c.pattern, c.spec, got, c.want)
		}
	}
}

func TestSubstitutePattern(t *testing.T) {
	cases := []struct {
		pattern     string
		replacement string
		spec        string
		want        string
	}{
		{"@lib/*", "src/lib/*", "@lib/utils", "src/lib/utils"},
		{"@lib/*", "src/lib/*", "@lib/helpers/format", "src/lib/helpers/format"},
		{"@app/*", "src/*", "@app/components/Button", "src/components/Button"},
		{"@utils", "src/utils", "@utils", "src/utils"},
	}
	for _, c := range cases {
		got := substitutePattern(c.pattern, c.replacement, c.spec)
		if got != c.want {
			t.Errorf("substitutePattern(%q, %q, %q) = %q, want %q",
				c.pattern, c.replacement, c.spec, got, c.want)
		}
	}
}

func TestResolveExtension(t *testing.T) {
	t.Run(".js resolves to .ts when .ts exists", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/Runtime.ts", []byte{}, 0o644)
		got := resolveExtension(fs, "src/Runtime.js", "/")
		if got != "src/Runtime.ts" {
			t.Errorf("got %q, want src/Runtime.ts", got)
		}
	})

	t.Run(".jsx resolves to .tsx when .tsx exists", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/Button.tsx", []byte{}, 0o644)
		got := resolveExtension(fs, "src/Button.jsx", "/")
		if got != "src/Button.tsx" {
			t.Errorf("got %q, want src/Button.tsx", got)
		}
	})

	t.Run(".mjs resolves to .mts", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/worker.mts", []byte{}, 0o644)
		got := resolveExtension(fs, "src/worker.mjs", "/")
		if got != "src/worker.mts" {
			t.Errorf("got %q, want src/worker.mts", got)
		}
	})

	t.Run(".cjs resolves to .cts", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/config.cts", []byte{}, 0o644)
		got := resolveExtension(fs, "src/config.cjs", "/")
		if got != "src/config.cts" {
			t.Errorf("got %q, want src/config.cts", got)
		}
	})

	t.Run(".ts left as-is", func(t *testing.T) {
		got := resolveExtension(memfs.New(), "src/app.ts", "/")
		if got != "src/app.ts" {
			t.Errorf("got %q, want src/app.ts", got)
		}
	})

	t.Run(".tsx left as-is", func(t *testing.T) {
		got := resolveExtension(memfs.New(), "src/app.tsx", "/")
		if got != "src/app.tsx" {
			t.Errorf("got %q, want src/app.tsx", got)
		}
	})

	t.Run("bare import resolves to .ts", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/utils.ts", []byte{}, 0o644)
		got := resolveExtension(fs, "src/utils", "/")
		if got != "src/utils.ts" {
			t.Errorf("got %q, want src/utils.ts", got)
		}
	})

	t.Run("bare import resolves to .tsx", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/Button.tsx", []byte{}, 0o644)
		got := resolveExtension(fs, "src/Button", "/")
		if got != "src/Button.tsx" {
			t.Errorf("got %q, want src/Button.tsx", got)
		}
	})

	t.Run("index file resolution", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/lib/index.ts", []byte{}, 0o644)
		got := resolveExtension(fs, "src/lib", "/")
		if got != "src/lib/index.ts" {
			t.Errorf("got %q, want src/lib/index.ts", got)
		}
	})

	t.Run(".js file actually exists—no rewrite", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/legacy.js", []byte{}, 0o644)
		got := resolveExtension(fs, "src/legacy.js", "/")
		if got != "src/legacy.js" {
			t.Errorf("got %q, want src/legacy.js", got)
		}
	})

	t.Run("non-existent .js with no .ts fallback returns original", func(t *testing.T) {
		got := resolveExtension(memfs.New(), "src/missing.js", "/")
		if got != "src/missing.js" {
			t.Errorf("got %q, want src/missing.js", got)
		}
	})

	t.Run(".json file left as-is", func(t *testing.T) {
		got := resolveExtension(memfs.New(), "src/data.json", "/")
		if got != "src/data.json" {
			t.Errorf("got %q, want src/data.json", got)
		}
	})
}

func TestResolveInternalTarget_JSExtensions(t *testing.T) {
	fs := memfs.New()
	files := []string{
		"src/Runtime.ts",
		"src/DirectClientRuntimeLink.ts",
		"src/BffFetcher.ts",
		"src/Bff.ts",
		"src/CojaResponse.ts",
		"src/CojaRequest.ts",
		"src/ClientRuntimeLink.ts",
		"src/Client.ts",
		"src/HttpClientRuntimeLink.ts",
		"src/RequestContext.ts",
	}
	for _, f := range files {
		fs.WriteFile("/"+f, []byte{}, 0o644)
	}

	capsule := port.Capsule{CapsuleID: "cojajs-coja", Dir: "/"}
	l := &Language{}

	cases := []struct {
		spec     string
		fileRel  string
		wantPath string
		wantIntl bool
	}{
		{"./Runtime.js", "src/index.ts", "src/Runtime.ts", true},
		{"./DirectClientRuntimeLink.js", "src/index.ts", "src/DirectClientRuntimeLink.ts", true},
		{"./BffFetcher.js", "src/index.ts", "src/BffFetcher.ts", true},
		{"./Bff.js", "src/index.ts", "src/Bff.ts", true},
		{"./CojaResponse.js", "src/index.ts", "src/CojaResponse.ts", true},
		{"./CojaRequest.js", "src/index.ts", "src/CojaRequest.ts", true},
		{"./Bff", "src/Client.ts", "src/Bff.ts", true},
		{"./ClientRuntimeLink", "src/Client.ts", "src/ClientRuntimeLink.ts", true},
		{"../lib/utils.js", "src/components/Button.tsx", "src/lib/utils.ts", true},
	}

	fs.WriteFile("/src/lib/utils.ts", []byte{}, 0o644)
	fs.WriteFile("/src/components/Button.tsx", []byte{}, 0o644)

	for _, c := range cases {
		gotPath, gotIntl := l.ResolveInternalTarget(fs, port.ImportSpec{Path: c.spec}, capsule, c.fileRel)
		if gotPath != c.wantPath || gotIntl != c.wantIntl {
			t.Errorf("ResolveInternalTarget(%q, file=%q) = (%q, %v), want (%q, %v)",
				c.spec, c.fileRel, gotPath, gotIntl, c.wantPath, c.wantIntl)
		}
	}
}
