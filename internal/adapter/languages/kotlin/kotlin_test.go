package kotlin

import (
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

func TestName(t *testing.T) {
	if got := (Language{}).Name(); got != "kotlin" {
		t.Errorf("Name() = %q, want %q", got, "kotlin")
	}
}

func TestSupportsFileGlobs(t *testing.T) {
	if (Language{}).SupportsFileGlobs() {
		t.Error("SupportsFileGlobs() = true, want false")
	}
}

func TestIsGovernedFile(t *testing.T) {
	l := Language{}
	cases := map[string]bool{
		// Main source — kotlin
		"src/main/kotlin/com/example/domain/Model.kt":   true,
		"src/main/kotlin/com/example/api/Controller.kt": true,
		"src/main/kotlin/com/example/infra/Repo.kt":     true,
		"src/main/kotlin/com/example/Main.kt":           true,
		"src/main/kotlin/com/example/deep/nested/Ok.kt": true,

		// Main source — java (mixed projects can have .kt in java tree)
		"src/main/java/com/example/domain/Model.kt": true,

		// Multi-platform source sets
		"src/jvmMain/kotlin/com/example/domain/Model.kt":     true,
		"src/jvmMain/java/com/example/domain/Model.kt":       true,
		"src/commonMain/kotlin/com/example/domain/Model.kt":  true,
		"src/commonTest/kotlin/com/example/domain/Model.kt":  true,
		"src/androidMain/kotlin/com/example/domain/Model.kt": true,
		"src/iosMain/kotlin/com/example/domain/Model.kt":     true,
		"src/macosMain/kotlin/com/example/domain/Model.kt":   true,
		"src/linuxMain/kotlin/com/example/domain/Model.kt":   true,
		"src/darwinMain/kotlin/com/example/domain/Model.kt":  true,
		"src/nativeMain/kotlin/com/example/domain/Model.kt":  true,
		"src/jsMain/kotlin/com/example/domain/Model.kt":      true,

		// Mingw (Windows) source sets
		"src/mingwMain/kotlin/com/example/domain/Model.kt": true,
		"src/mingwTest/kotlin/com/example/domain/Model.kt": true,

		// Binary-specific KMP source sets
		"src/iosArm64Main/kotlin/com/example/domain/Model.kt":          true,
		"src/iosSimulatorArm64Main/kotlin/com/example/domain/Model.kt": true,
		"src/macosX64Main/kotlin/com/example/domain/Model.kt":          true,
		"src/macosArm64Main/kotlin/com/example/domain/Model.kt":        true,
		"src/linuxX64Main/kotlin/com/example/domain/Model.kt":          true,
		"src/mingwX64Main/kotlin/com/example/domain/Model.kt":          true,
		"src/iosArm64Test/kotlin/com/example/domain/Model.kt":          true,
		"src/iosSimulatorArm64Test/kotlin/com/example/domain/Model.kt": true,
		"src/macosX64Test/kotlin/com/example/domain/Model.kt":          true,
		"src/macosArm64Test/kotlin/com/example/domain/Model.kt":        true,
		"src/linuxX64Test/kotlin/com/example/domain/Model.kt":          true,
		"src/mingwX64Test/kotlin/com/example/domain/Model.kt":          true,

		// Android instrumented test
		"src/androidInstrumentedTest/kotlin/com/example/domain/Model.kt": true,

		// Test files excluded
		"src/main/kotlin/com/example/ModelTest.kt":  false,
		"src/main/kotlin/com/example/Model_test.kt": false,
		"src/test/kotlin/com/example/ModelTest.kt":  false,

		// Non-main source sets excluded
		"src/jvmTest/kotlin/com/example/Model.kt": true,

		// Non-.kt files
		"build.gradle.kts":                      false,
		"src/main/resources/application.yaml":   false,
		"README.md":                             false,
		"src/main/kotlin/com/example/Model.kts": false,

		// Outside src/main
		"scripts/run.kt":                   false,
		"gradle/wrapper/gradle-wrapper.kt": false,
	}
	for rel, want := range cases {
		if got := l.IsGovernedFile(rel); got != want {
			t.Errorf("IsGovernedFile(%q) = %v, want %v", rel, got, want)
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
// import com.example.disabled
/* import com.example.disabled2 */

@RestController
class MyController {
}`
	fs := memfs.New()
	fs.WriteFile("/MyController.kt", []byte(src), 0o644)
	got, err := Language{}.ParseImports(fs, "/MyController.kt")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"com.example.domain.Model",
		"com.example.infra.Repository",
		"com.example.api.request.CreateRequest",
		"org.springframework.web.bind.annotation.RestController",
		"java.util.UUID",
		"com.example.utils",
		"com.example.constant.CONSTANT",
		"com.example.File",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d imports, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Path != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i].Path, want[i])
		}
	}
}

func TestResolveInternalTarget(t *testing.T) {
	l := Language{}
	capsule := port.Capsule{CapsuleID: "com.example"}

	type tc struct {
		spec     string
		fileRel  string
		wantPath string
		wantIntl bool
	}
	cases := []tc{
		// External packages
		{"org.springframework.web.bind.annotation.RestController", "src/main/kotlin/com/example/api/Controller.kt", "", false},
		{"java.util.UUID", "src/main/kotlin/com/example/api/Controller.kt", "", false},
		{"kotlinx.coroutines.launch", "src/main/kotlin/com/example/api/Controller.kt", "", false},

		// Internal — exact base package match
		{"com.example", "src/main/kotlin/com/example/api/Controller.kt", "src/main/kotlin/com/example", true},

		// Internal — sub-packages
		{"com.example.domain.Model", "src/main/kotlin/com/example/api/Controller.kt", "src/main/kotlin/com/example/domain/Model", true},
		{"com.example.infra.Repository", "src/main/kotlin/com/example/api/Controller.kt", "src/main/kotlin/com/example/infra/Repository", true},
		{"com.example.api.request.CreateRequest", "src/main/kotlin/com/example/api/Controller.kt", "src/main/kotlin/com/example/api/request/CreateRequest", true},

		// Word boundary: com.example2 must NOT match com.example
		{"com.example2.domain.Model", "src/main/kotlin/com/example/api/Controller.kt", "", false},
		{"com.exampleapi.Controller", "src/main/kotlin/com/example/api/Controller.kt", "", false},

		// Empty module ID
	}
	for _, c := range cases {
		gotPath, gotIntl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: c.spec}, capsule, c.fileRel)
		if gotPath != c.wantPath || gotIntl != c.wantIntl {
			t.Errorf("ResolveInternalTarget(%q, file=%q) = (%q, %v), want (%q, %v)",
				c.spec, c.fileRel, gotPath, gotIntl, c.wantPath, c.wantIntl)
		}
	}

	// Empty module ID
	capsuleEmpty := port.Capsule{CapsuleID: ""}
	_, intl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: "com.example.Foo"}, capsuleEmpty, "src/main/kotlin/com/example/Foo.kt")
	if intl {
		t.Error("ResolveInternalTarget with empty CapsuleID should return false")
	}
}

func TestIsInternalCapsule(t *testing.T) {
	cases := []struct {
		spec string
		base string
		want bool
	}{
		{"com.example", "com.example", true},
		{"com.example.domain", "com.example", true},
		{"com.example.domain.Model", "com.example", true},
		{"com.example2.domain", "com.example", false},
		{"com.exampleapi.Controller", "com.example", false},
		{"com.other.domain", "com.example", false},
		{"com.ex", "com.example", false},
		{"com.example.", "com.example", false},
		{"", "com.example", false},
	}
	for _, c := range cases {
		if got := isInternalCapsule(c.spec, c.base); got != c.want {
			t.Errorf("isInternalCapsule(%q, %q) = %v, want %v", c.spec, c.base, got, c.want)
		}
	}
}

func TestHasBuildScript(t *testing.T) {
	fs := memfs.New()

	// Empty dir should not have a package
	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	entries, err := disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Error("empty dir should not have build script")
	}

	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/src/main/kotlin/com/example/Main.kt", nil, 0o644)
	entries, err = disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Error("dir with build.gradle.kts should be discovered")
	}

	fs.WriteFile("/build.gradle", nil, 0o644)
	entries, err = disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Error("dir with build.gradle should be discovered")
	}
}

func TestFindBaseCapsule(t *testing.T) {
	t.Run("standard layout", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/main/kotlin/com/example/domain/Model.kt", nil, 0o644)
		fs.WriteFile("/src/main/kotlin/com/example/api/Controller.kt", nil, 0o644)
		got, err := findBaseCapsule(fs, "/")
		if err != nil {
			t.Fatal(err)
		}
		if got != "com.example" {
			t.Errorf("got %q, want %q", got, "com.example")
		}
	})

	t.Run("nested base package", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/main/kotlin/org/acme/myapp/domain/Model.kt", nil, 0o644)
		fs.WriteFile("/src/main/kotlin/org/acme/myapp/api/Controller.kt", nil, 0o644)
		got, err := findBaseCapsule(fs, "/")
		if err != nil {
			t.Fatal(err)
		}
		if got != "org.acme.myapp" {
			t.Errorf("got %q, want %q", got, "org.acme.myapp")
		}
	})

	t.Run("fallback to src/main/java", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/main/java/com/example/domain/Model.kt", nil, 0o644)
		fs.WriteFile("/src/main/java/com/example/api/Controller.kt", nil, 0o644)
		got, err := findBaseCapsule(fs, "/")
		if err != nil {
			t.Fatal(err)
		}
		if got != "com.example" {
			t.Errorf("got %q, want %q", got, "com.example")
		}
	})

	t.Run("no kotlin source dir returns empty", func(t *testing.T) {
		fs := memfs.New()
		got, err := findBaseCapsule(fs, "/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("no .kt files", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/main/kotlin/com/example/Model.java", nil, 0o644)
		_, err := findBaseCapsule(fs, "/")
		if err == nil {
			t.Error("expected error for no .kt files")
		}
	})

	t.Run("divergent packages", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/main/kotlin/com/example/domain/Model.kt", nil, 0o644)
		fs.WriteFile("/src/main/kotlin/org/other/domain/Other.kt", nil, 0o644)
		_, err := findBaseCapsule(fs, "/")
		if err == nil {
			t.Error("expected error for divergent packages")
		}
	})
}

func TestDiscover(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/src/main/kotlin/com/example/domain/Model.kt", []byte("package com.example.domain\nclass Model"), 0o644)
	fs.WriteFile("/src/main/kotlin/com/example/api/Controller.kt", []byte("package com.example.api\nclass Controller"), 0o644)
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/BAFT.md", []byte("```mermaid\nflowchart TD\n    A[\"src/main/kotlin/com/example/domain\"]\n```\n"), 0o644)

	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	entries, err := disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages, want 1", len(entries))
	}
	if entries[0].Capsule.CapsuleID != "com.example" {
		t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example")
	}

	// No BAFT.md — should still be discovered
	disco2 := service.NewCapsuleDiscovery()
	Language{}.Register(disco2)
	fs2 := memfs.New()
	fs2.WriteFile("/src/main/kotlin/com/example/domain/Model.kt", []byte("package com.example.domain\nclass Model"), 0o644)
	fs2.WriteFile("/src/main/kotlin/com/example/api/Controller.kt", []byte("package com.example.api\nclass Controller"), 0o644)
	fs2.WriteFile("/build.gradle.kts", nil, 0o644)
	entries, err = disco2.Discover(fs2, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages without BAFT.md, want 1", len(entries))
	}

	// Legacy build.gradle
	disco3 := service.NewCapsuleDiscovery()
	Language{}.Register(disco3)
	fs3 := memfs.New()
	fs3.WriteFile("/src/main/kotlin/com/example/domain/Model.kt", []byte("package com.example.domain\nclass Model"), 0o644)
	fs3.WriteFile("/src/main/kotlin/com/example/api/Controller.kt", []byte("package com.example.api\nclass Controller"), 0o644)
	fs3.WriteFile("/build.gradle", nil, 0o644)
	fs3.WriteFile("/BAFT.md", []byte("```mermaid\nflowchart TD\n    A[\"src/main/kotlin/com/example/domain\"]\n```\n"), 0o644)
	entries, err = disco3.Discover(fs3, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages with build.gradle, want 1", len(entries))
	}
}

func TestDiscover_SkipsBuildDirs(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/src/main/kotlin/com/example/Main.kt", nil, 0o644)
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/BAFT.md", []byte("```mermaid\nflowchart TD\n    A[\"src/main/kotlin/com/example\"]\n```\n"), 0o644)
	fs.WriteFile("/build/generated/kotlin/com/example/Generated.kt", nil, 0o644)
	fs.WriteFile("/build/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/build/BAFT.md", []byte("```mermaid\nflowchart TD\n    A[\"src/main/kotlin/com/example\"]\n```\n"), 0o644)

	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	entries, err := disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages, want 1 (build dir should be skipped)", len(entries))
	}
}

func TestDiscoverDraft_MultiModuleWithRoot(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/module-a/src/main/kotlin/com/example/a/Model.kt", nil, 0o644)
	fs.WriteFile("/module-a/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/module-b/src/main/kotlin/com/example/b/Service.kt", nil, 0o644)
	fs.WriteFile("/module-b/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/module-c/build.gradle.kts", nil, 0o644)

	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	entries, err := disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d packages, want 2 (root and module-c should be skipped)", len(entries))
	}

	if port.Label(entries[0].Capsule, "/") != entries[0].Capsule.Dir {
		t.Errorf("pkgs[0] label = %q, want %q", port.Label(entries[0].Capsule, "/"), entries[0].Capsule.Dir)
	}
	if entries[0].Capsule.CapsuleID != "com.example.a" {
		t.Errorf("pkgs[0].CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example.a")
	}

	if port.Label(entries[1].Capsule, "/") != entries[1].Capsule.Dir {
		t.Errorf("pkgs[1] label = %q, want %q", port.Label(entries[1].Capsule, "/"), entries[1].Capsule.Dir)
	}
	if entries[1].Capsule.CapsuleID != "com.example.b" {
		t.Errorf("pkgs[1].CapsuleID = %q, want %q", entries[1].Capsule.CapsuleID, "com.example.b")
	}
}

func TestDiscoverDraft_RootProjectNoSource(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/settings.gradle.kts", nil, 0o644)
	fs.WriteFile("/core/src/main/kotlin/com/example/core/Core.kt", nil, 0o644)
	fs.WriteFile("/core/build.gradle.kts", nil, 0o644)

	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	entries, err := disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages, want 1", len(entries))
	}
	if port.Label(entries[0].Capsule, "/") != entries[0].Capsule.Dir {
		t.Errorf("pkgs[0] label = %q, want %q", port.Label(entries[0].Capsule, "/"), entries[0].Capsule.Dir)
	}
}

func TestDiscover_SkipsKotlinCache(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/src/main/kotlin/com/example/Main.kt", nil, 0o644)
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/BAFT.md", []byte("```mermaid\nflowchart TD\n    A[\"src/main/kotlin/com/example\"]\n```\n"), 0o644)
	fs.WriteFile("/.kotlin/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/.kotlin/BAFT.md", []byte("```mermaid\nflowchart TD\n    A[\"src/main/kotlin/com/example\"]\n```\n"), 0o644)

	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	entries, err := disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages, want 1 (.kotlin dir should be skipped)", len(entries))
	}
}

func TestSkipDirs(t *testing.T) {
	l := Language{}
	skip := l.SkipDirs()
	if len(skip) != 2 {
		t.Errorf("expected 2 skip dirs, got %d", len(skip))
	}
	for _, dir := range []string{"build", ".kotlin"} {
		found := false
		for _, s := range skip {
			if s == dir {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in skip dirs", dir)
		}
	}
}

func TestIsInternalCapsule_EdgeCases(t *testing.T) {
	cases := []struct {
		spec string
		base string
		want bool
	}{
		{"com.example2", "com.example", false},
		{"com.example2.domain", "com.example", false},
		{"com.example.domain2", "com.example", true},
		{"com.example.", "com.example", false},
		{"com.ex", "com.example", false},
		{"com", "com.example", false},
		{"", "com.example", false},
		{"com.example.a.b.c.d.Deep", "com.example", true},
		{"com.example.v2.Api", "com.example", true},
		{"com.example2.v2.Api", "com.example", false},
		{"com.example_internal", "com.example", false},
		{"com.example_internal.Foo", "com.example", false},
	}
	for _, c := range cases {
		if got := isInternalCapsule(c.spec, c.base); got != c.want {
			t.Errorf("isInternalCapsule(%q, %q) = %v, want %v", c.spec, c.base, got, c.want)
		}
	}
}
