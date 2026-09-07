package jvm

import (
	"context"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

func TestName(t *testing.T) {
	if got := (&Language{}).Name(); got != "jvm" {
		t.Errorf("Name() = %q, want %q", got, "jvm")
	}
}

func TestSupportsFileGlobs(t *testing.T) {
	if (&Language{}).SupportsFileGlobs() {
		t.Error("SupportsFileGlobs() = true, want false")
	}
}

func TestIsScannableFile(t *testing.T) {
	l := &Language{}
	cases := map[string]bool{
		"src/main/java/com/example/domain/Model.java":    true,
		"src/main/kotlin/com/example/domain/Model.kt":    true,
		"src/jvmMain/java/com/example/domain/Model.java": true,
		"src/jvmMain/kotlin/com/example/domain/Model.kt": true,
		"src/main/kotlin/com/example/Service.java":       true,
		"scripts/run.java": true,
		"ModelTest.kt":     true,
		"build.gradle.kts": false,
		"build.gradle":     false,
		"pom.xml":          false,
		"application.yaml": false,
		"README.md":        false,
		"Model.kts":        false,
		"Model.graalvm":    false,
	}
	for rel, want := range cases {
		if got := l.IsScannableFile(rel); got != want {
			t.Errorf("IsScannableFile(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestParseImports(t *testing.T) {
	src := `package com.example.api

import com.example.domain.Model
import com.example.infra.Repository
import com.example.api.request.CreateRequest
import org.springframework.web.bind.annotation.RestController
import java.util.UUID
import com.example.utils.*
import static com.example.constant.CONSTANT
import com.example.File { foo, bar }

@RestController
class MyController {
}`
	fs := memfs.New()
	fs.WriteFile("/MyController.kt", []byte(src), 0o644)
	got, err := (&Language{}).ParseImports(fs, "/MyController.kt")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		path string
		line int
		col  int
	}{
		{"com.example.domain.Model", 3, 8},
		{"com.example.infra.Repository", 4, 8},
		{"com.example.api.request.CreateRequest", 5, 8},
		{"org.springframework.web.bind.annotation.RestController", 6, 8},
		{"java.util.UUID", 7, 8},
		{"com.example.utils", 8, 8},
		{"com.example.constant.CONSTANT", 9, 15},
		{"com.example.File", 10, 8},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d imports, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Path != want[i].path {
			t.Errorf("[%d] Path = %q, want %q", i, got[i].Path, want[i].path)
		}
		if got[i].Line != want[i].line {
			t.Errorf("[%d] Line = %d, want %d", i, got[i].Line, want[i].line)
		}
		if got[i].Col != want[i].col {
			t.Errorf("[%d] Col = %d, want %d", i, got[i].Col, want[i].col)
		}
		if wantColEnd := want[i].col + len(want[i].path); got[i].ColEnd != wantColEnd {
			t.Errorf("[%d] ColEnd = %d, want %d", i, got[i].ColEnd, wantColEnd)
		}
	}
}

func TestGetFileNamespace(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/Model.java", []byte("package com.example.domain;\n\nclass Model {}"), 0o644)
	fs.WriteFile("/Empty.kt", []byte("class Empty"), 0o644)
	got, err := (&Language{}).GetFileNamespace(fs, "/Model.java")
	if err != nil {
		t.Fatal(err)
	}
	if got != "com.example.domain" {
		t.Errorf("GetFileNamespace = %q, want %q", got, "com.example.domain")
	}
	if got, _ := (&Language{}).GetFileNamespace(fs, "/Empty.kt"); got != "" {
		t.Errorf("GetFileNamespace without package = %q, want empty", got)
	}
}

func TestResolveInternalTarget(t *testing.T) {
	l := &Language{}
	capsule := port.Capsule{CapsuleID: "com.example"}

	cases := []struct {
		spec     string
		fileRel  string
		wantPath string
		wantIntl bool
	}{
		// External packages
		{"org.springframework.web.bind.annotation.RestController", "src/main/java/com/example/api/Controller.java", "", false},
		{"java.util.UUID", "src/main/java/com/example/api/Controller.java", "", false},
		{"kotlinx.coroutines.launch", "src/main/kotlin/com/example/api/Controller.kt", "", false},

		// Internal — exact base package match
		{"com.example", "src/main/java/com/example/api/Controller.java", "src/main/java/com/example", true},

		// Internal — sub-packages
		{"com.example.domain.Model", "src/main/java/com/example/api/Controller.java", "src/main/java/com/example/domain/Model", true},
		{"com.example.api.request.CreateRequest", "src/main/kotlin/com/example/api/Controller.kt", "src/main/kotlin/com/example/api/request/CreateRequest", true},

		// Source prefix comes from the importing file
		{"com.example.domain.Model", "src/jvmMain/java/com/example/api/Controller.java", "src/jvmMain/java/com/example/domain/Model", true},
		{"com.example.domain.Model", "Controller.java", "src/main/java/com/example/domain/Model", true},
		{"com.example.domain.Model", "Controller.kt", "src/main/kotlin/com/example/domain/Model", true},

		// Word boundary: com.example2 must NOT match com.example
		{"com.example2.domain.Model", "src/main/java/com/example/api/Controller.java", "", false},
		{"com.exampleapi.Controller", "src/main/kotlin/com/example/api/Controller.kt", "", false},
	}
	for _, c := range cases {
		gotPath, gotIntl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: c.spec}, capsule, c.fileRel)
		if gotPath != c.wantPath || gotIntl != c.wantIntl {
			t.Errorf("ResolveInternalTarget(%q, file=%q) = (%q, %v), want (%q, %v)",
				c.spec, c.fileRel, gotPath, gotIntl, c.wantPath, c.wantIntl)
		}
	}

	// Empty capsule ID
	if _, intl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: "com.example.Foo"}, port.Capsule{}, "src/main/java/com/example/Foo.java"); intl {
		t.Error("ResolveInternalTarget with empty CapsuleID should return false")
	}
}

func TestResolveInternalTargetAcrossSourceSets(t *testing.T) {
	fs := memfs.New()
	for _, f := range []string{
		"/app/src/main/java/com/example/api/Controller.java",
		"/app/src/main/java/com/example/domain/Order.java",
		"/app/src/main/kotlin/com/example/infra/Repo.kt",
		"/app/src/main/kotlin/com/example/ui/Screen.kt",
	} {
		fs.WriteFile(f, nil, 0o644)
	}
	c := port.Capsule{Dir: "/app", CapsuleID: "com.example"}

	cases := []struct {
		spec, fileRel, want string
	}{
		{"com.example.infra.Repo", "src/main/java/com/example/api/Controller.java", "src/main/kotlin/com/example/infra/Repo"},
		{"com.example.domain.Order", "src/main/kotlin/com/example/ui/Screen.kt", "src/main/java/com/example/domain/Order"},
		// A package import resolves to the root holding the package.
		{"com.example.infra", "src/main/java/com/example/api/Controller.java", "src/main/kotlin/com/example/infra"},
		// A class declared in a differently named file resolves via its package.
		{"com.example.infra.Helper", "src/main/java/com/example/api/Controller.java", "src/main/kotlin/com/example/infra/Helper"},
		// Same-root imports keep the importing file's root.
		{"com.example.domain.Order", "src/main/java/com/example/api/Controller.java", "src/main/java/com/example/domain/Order"},
		// Nothing holds the target: the importing file's root is the fallback.
		{"com.example.missing.Gone", "src/main/kotlin/com/example/ui/Screen.kt", "src/main/kotlin/com/example/missing/Gone"},
	}
	for _, tc := range cases {
		got, intl := (&Language{}).ResolveInternalTarget(fs, port.ImportSpec{Path: tc.spec}, c, tc.fileRel)
		if !intl || got != tc.want {
			t.Errorf("ResolveInternalTarget(%q, file=%q) = (%q, %v), want (%q, true)", tc.spec, tc.fileRel, got, intl, tc.want)
		}
	}
}

func TestDiscover_ManifestTypes(t *testing.T) {
	for _, manifest := range []string{"build.gradle.kts", "build.gradle", "pom.xml"} {
		t.Run(manifest, func(t *testing.T) {
			fs := memfs.New()
			fs.WriteFile("/"+manifest, nil, 0o644)
			fs.WriteFile("/src/main/java/com/example/Main.java", nil, 0o644)
			entries := discover(t, fs)
			if len(entries) != 1 {
				t.Fatalf("got %d, want 1", len(entries))
			}
			if entries[0].Capsule.CapsuleID != "com.example" {
				t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example")
			}
			if entries[0].LangName != "jvm" {
				t.Errorf("LangName = %q, want %q", entries[0].LangName, "jvm")
			}
		})
	}

	t.Run("empty dir no capsules", func(t *testing.T) {
		if entries := discover(t, memfs.New()); len(entries) != 0 {
			t.Error("empty dir should not discover capsules")
		}
	})
}

func TestFindBaseCapsule(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{"standard java layout", []string{"/src/main/java/com/example/domain/Model.java", "/src/main/java/com/example/api/Controller.java"}, "com.example"},
		{"standard kotlin layout", []string{"/src/main/kotlin/com/example/domain/Model.kt", "/src/main/kotlin/com/example/api/Controller.kt"}, "com.example"},
		{"nested base package", []string{"/src/main/kotlin/org/acme/myapp/domain/Model.kt", "/src/main/kotlin/org/acme/myapp/api/Controller.kt"}, "org.acme.myapp"},
		{"kotlin sources under src/main/java", []string{"/src/main/java/com/example/domain/Model.kt", "/src/main/java/com/example/api/Controller.kt"}, "com.example"},
		{"java and kotlin source sets combined", []string{"/src/main/java/com/example/domain/Model.java", "/src/main/kotlin/com/example/api/Controller.kt"}, "com.example"},
		{"multiplatform source sets", []string{"/src/commonMain/kotlin/com/example/core/Model.kt", "/src/jvmMain/kotlin/com/example/core/jvm/Impl.kt"}, "com.example.core"},
		{"test source set ignored", []string{"/src/main/kotlin/com/example/api/Controller.kt", "/src/test/kotlin/org/other/ControllerTest.kt"}, "com.example.api"},
		{"test source sets ignored", []string{"/src/main/kotlin/com/example/api/Controller.kt", "/src/androidUnitTest/kotlin/org/other/Case.kt", "/src/testFixtures/kotlin/org/other/Fixture.kt"}, "com.example.api"},
		{"production source set whose name merely contains test", []string{"/src/attestation/java/com/example/a/Model.java", "/src/main/kotlin/com/example/b/Service.kt"}, "com.example"},
		{"sibling package is not a prefix", []string{"/src/main/java/com/example/app/Main.java", "/src/main/java/com/example/application/Service.java"}, "com.example"},
		{"no source dir returns empty", nil, ""},
		{"no source files", []string{"/src/main/java/com/example/Model.kts"}, ""},
		// A source set with a package of its own must not sink the capsule.
		{"divergent source sets fall back to main", []string{"/src/main/java/com/example/app/Main.java", "/src/jsMain/kotlin/org/other/Client.kt"}, "com.example.app"},
		{"divergent source sets without a main one", []string{"/src/jvmMain/kotlin/com/example/Main.kt", "/src/jsMain/kotlin/org/other/Client.kt"}, ""},
		{"divergent packages inside main", []string{"/src/main/java/com/example/domain/Model.java", "/src/main/kotlin/org/other/domain/Other.kt"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := memfs.New()
			for _, f := range c.files {
				fs.WriteFile(f, nil, 0o644)
			}
			got, err := findBaseCapsule(fs, "/")
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestScansJavaAndKotlinInOneCapsule(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/src/main/java/com/example/domain/Foo.java", []byte("package com.example.domain;"), 0o644)
	fs.WriteFile("/src/main/java/com/example/domain/Bar.kt", []byte("package com.example.domain"), 0o644)

	entries := discover(t, fs)
	if len(entries) != 1 {
		t.Fatalf("got %d capsules, want 1", len(entries))
	}
	if entries[0].Capsule.CapsuleID != "com.example.domain" {
		t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example.domain")
	}
	l := &Language{}
	for _, rel := range []string{"src/main/java/com/example/domain/Foo.java", "src/main/java/com/example/domain/Bar.kt"} {
		if !l.IsScannableFile(rel) {
			t.Errorf("IsScannableFile(%q) = false, want true", rel)
		}
	}
}

func TestDiscover_SkipsGeneratedDirs(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/src/main/kotlin/com/example/Main.kt", nil, 0o644)
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/build/generated/java/com/example/Generated.java", nil, 0o644)
	fs.WriteFile("/build/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/.kotlin/build.gradle.kts", nil, 0o644)

	entries := discover(t, fs)
	if len(entries) != 1 {
		t.Fatalf("got %d capsules, want 1 (build and .kotlin dirs have no sources)", len(entries))
	}
	if entries[0].Capsule.CapsuleID != "com.example" {
		t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example")
	}
}

func TestDiscover_MultiModuleWithSourcelessRoot(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/settings.gradle.kts", nil, 0o644)
	fs.WriteFile("/module-a/src/main/java/com/example/a/Model.java", nil, 0o644)
	fs.WriteFile("/module-a/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/module-b/src/main/kotlin/com/example/b/Service.kt", nil, 0o644)
	fs.WriteFile("/module-b/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/module-c/build.gradle.kts", nil, 0o644)

	entries := discover(t, fs)
	if len(entries) != 2 {
		t.Fatalf("got %d capsules, want 2 (root and module-c should be skipped)", len(entries))
	}
	for i, want := range []string{"com.example.a", "com.example.b"} {
		if port.Label(entries[i].Capsule) != entries[i].Capsule.Dir {
			t.Errorf("capsule %d label = %q, want %q", i, port.Label(entries[i].Capsule), entries[i].Capsule.Dir)
		}
		if entries[i].Capsule.CapsuleID != want {
			t.Errorf("capsule %d CapsuleID = %q, want %q", i, entries[i].Capsule.CapsuleID, want)
		}
		if entries[i].LangName != "jvm" {
			t.Errorf("capsule %d LangName = %q, want %q", i, entries[i].LangName, "jvm")
		}
	}
}

// A multiplatform project whose targets share no package prefix still has a
// JVM capsule; before the fallback to src/main it was dropped outright.
func TestDiscover_DivergentSourceSets(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/src/main/java/com/example/app/Main.java", nil, 0o644)
	fs.WriteFile("/src/jsMain/kotlin/org/other/Client.kt", nil, 0o644)

	entries := discover(t, fs)
	if len(entries) != 1 || entries[0].Capsule.CapsuleID != "com.example.app" {
		t.Fatalf("got %+v, want one capsule with CapsuleID com.example.app", entries)
	}
}

func discover(t *testing.T, fs port.FileSystem) []service.CapsuleEntry {
	t.Helper()
	disco := service.NewCapsuleDiscovery()
	(&Language{}).Register(disco)
	entries, err := disco.Discover(context.Background(), fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	return entries
}
