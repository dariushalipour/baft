package java

import (
	"context"
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

func TestName(t *testing.T) {
	if got := (Language{}).Name(); got != "java" {
		t.Errorf("Name() = %q, want %q", got, "java")
	}
}

func TestSupportsFileGlobs(t *testing.T) {
	if (Language{}).SupportsFileGlobs() {
		t.Error("SupportsFileGlobs() = true, want false")
	}
}

func TestIsScannableFile(t *testing.T) {
	l := Language{}
	cases := map[string]bool{
		"src/main/java/com/example/domain/Model.java":    true,
		"src/main/java/com/example/Main.java":            true,
		"src/jvmMain/java/com/example/domain/Model.java": true,
		"src/main/kotlin/com/example/Service.java":       true,
		"scripts/run.java":                               true,
		"ModelTest.java":                                 true,
		"build.gradle.kts":                               false,
		"application.yaml":                               false,
		"README.md":                                      false,
		"build.gradle":                                   false,
		"pom.xml":                                        false,
		"Model.graalvm":                                  false,
	}
	for rel, want := range cases {
		if got := l.IsScannableFile(rel); got != want {
			t.Errorf("IsScannableFile(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestParseImports(t *testing.T) {
	src := `package com.example.api;

import com.example.domain.Model;
import com.example.infra.Repository;
import com.example.api.request.CreateRequest;
import org.springframework.web.bind.annotation.RestController;
import java.util.UUID;
import com.example.utils.*;
import static com.example.constant.CONSTANT;

@RestController
public class MyController {
}`
	fs := memfs.New()
	fs.WriteFile("/MyController.java", []byte(src), 0o644)
	got, err := Language{}.ParseImports(fs, "/MyController.java")
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
		wantColEnd := want[i].col + len(want[i].path)
		if got[i].ColEnd != wantColEnd {
			t.Errorf("[%d] ColEnd = %d, want %d", i, got[i].ColEnd, wantColEnd)
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
		{"org.springframework.web.bind.annotation.RestController", "src/main/java/com/example/api/Controller.java", "", false},
		{"java.util.UUID", "src/main/java/com/example/api/Controller.java", "", false},
		{"java.lang.String", "src/main/java/com/example/api/Controller.java", "", false},

		// Internal — exact base package match
		{"com.example", "src/main/java/com/example/api/Controller.java", "src/main/java/com/example", true},

		// Internal — sub-packages
		{"com.example.domain.Model", "src/main/java/com/example/api/Controller.java", "src/main/java/com/example/domain/Model", true},
		{"com.example.infra.Repository", "src/main/java/com/example/api/Controller.java", "src/main/java/com/example/infra/Repository", true},
		{"com.example.api.request.CreateRequest", "src/main/java/com/example/api/Controller.java", "src/main/java/com/example/api/request/CreateRequest", true},

		// Word boundary: com.example2 must NOT match com.example
		{"com.example2.domain.Model", "src/main/java/com/example/api/Controller.java", "", false},
		{"com.exampleapi.Controller", "src/main/java/com/example/api/Controller.java", "", false},

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
	_, intl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: "com.example.Foo"}, capsuleEmpty, "src/main/java/com/example/Foo.java")
	if intl {
		t.Error("ResolveInternalTarget with empty CapsuleID should return false")
	}

	// jvmMain source prefix
	gotPath, gotIntl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: "com.example.domain.Model"}, capsule, "src/jvmMain/java/com/example/api/Controller.java")
	if gotPath != "src/jvmMain/java/com/example/domain/Model" || !gotIntl {
		t.Errorf("jvmMain prefix: got (%q, %v), want (src/jvmMain/java/com/example/domain/Model, true)", gotPath, gotIntl)
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
		{"com.example2", "com.example", false},
		{"com.exampleapi.Controller", "com.example", false},
		{"com.other.domain", "com.example", false},
		{"com.ex", "com.example", false},
		{"com", "com.example", false},
		{"com.example.", "com.example", false},
		{"", "com.example", false},
		{"com.example.domain2", "com.example", true},
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

func TestDiscover_ManifestTypes(t *testing.T) {
	t.Run("empty dir no capsules", func(t *testing.T) {
		fs := memfs.New()
		disco := service.NewCapsuleDiscovery()
		Language{}.Register(disco)
		entries, err := disco.Discover(context.Background(), fs, "/")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Error("empty dir should not discover capsules")
		}
	})

	t.Run("build.gradle.kts", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/build.gradle.kts", nil, 0o644)
		fs.WriteFile("/src/main/java/com/example/Main.java", nil, 0o644)
		disco := service.NewCapsuleDiscovery()
		Language{}.Register(disco)
		entries, err := disco.Discover(context.Background(), fs, "/")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("got %d, want 1", len(entries))
		}
		if entries[0].Capsule.CapsuleID != "com.example" {
			t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example")
		}
		if entries[0].LangName != "java" {
			t.Errorf("LangName = %q, want %q", entries[0].LangName, "java")
		}
	})

	t.Run("build.gradle", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/build.gradle", nil, 0o644)
		fs.WriteFile("/src/main/java/com/example/Main.java", nil, 0o644)
		disco := service.NewCapsuleDiscovery()
		Language{}.Register(disco)
		entries, err := disco.Discover(context.Background(), fs, "/")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("got %d, want 1", len(entries))
		}
		if entries[0].Capsule.CapsuleID != "com.example" {
			t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example")
		}
		if entries[0].LangName != "java" {
			t.Errorf("LangName = %q, want %q", entries[0].LangName, "java")
		}
	})

	t.Run("pom.xml", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/pom.xml", nil, 0o644)
		fs.WriteFile("/src/main/java/com/example/Main.java", nil, 0o644)
		disco := service.NewCapsuleDiscovery()
		Language{}.Register(disco)
		entries, err := disco.Discover(context.Background(), fs, "/")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("got %d, want 1", len(entries))
		}
		if entries[0].Capsule.CapsuleID != "com.example" {
			t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example")
		}
		if entries[0].LangName != "java" {
			t.Errorf("LangName = %q, want %q", entries[0].LangName, "java")
		}
	})
}

func TestFindBaseCapsule(t *testing.T) {
	t.Run("standard layout", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/main/java/com/example/domain/Model.java", nil, 0o644)
		fs.WriteFile("/src/main/java/com/example/api/Controller.java", nil, 0o644)
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
		fs.WriteFile("/src/main/java/org/acme/myapp/domain/Model.java", nil, 0o644)
		fs.WriteFile("/src/main/java/org/acme/myapp/api/Controller.java", nil, 0o644)
		got, err := findBaseCapsule(fs, "/")
		if err != nil {
			t.Fatal(err)
		}
		if got != "org.acme.myapp" {
			t.Errorf("got %q, want %q", got, "org.acme.myapp")
		}
	})

	t.Run("no java source dir returns empty", func(t *testing.T) {
		fs := memfs.New()
		got, err := findBaseCapsule(fs, "/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("no .java files", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/main/java/com/example/Model.kts", nil, 0o644)
		_, err := findBaseCapsule(fs, "/")
		if err == nil {
			t.Error("expected error for no .java files")
		}
	})

	t.Run("divergent packages", func(t *testing.T) {
		fs := memfs.New()
		fs.WriteFile("/src/main/java/com/example/domain/Model.java", nil, 0o644)
		fs.WriteFile("/src/main/java/org/other/domain/Other.java", nil, 0o644)
		_, err := findBaseCapsule(fs, "/")
		if err == nil {
			t.Error("expected error for divergent packages")
		}
	})
}

func TestDiscover(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/src/main/java/com/example/domain/Model.java", []byte("package com.example.domain\nclass Model"), 0o644)
	fs.WriteFile("/src/main/java/com/example/api/Controller.java", []byte("package com.example.api\nclass Controller"), 0o644)
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/BAFT.md", []byte("```mermaid\nflowchart TD\n    A[\"src/main/java/com/example/domain\"]\n```\n"), 0o644)

	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	entries, err := disco.Discover(context.Background(), fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages, want 1", len(entries))
	}
	if entries[0].Capsule.CapsuleID != "com.example" {
		t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example")
	}
	if entries[0].LangName != "java" {
		t.Errorf("LangName = %q, want %q", entries[0].LangName, "java")
	}

	// No BAFT.md — should still be discovered
	disco2 := service.NewCapsuleDiscovery()
	Language{}.Register(disco2)
	fs2 := memfs.New()
	fs2.WriteFile("/src/main/java/com/example/domain/Model.java", []byte("package com.example.domain\nclass Model"), 0o644)
	fs2.WriteFile("/src/main/java/com/example/api/Controller.java", []byte("package com.example.api\nclass Controller"), 0o644)
	fs2.WriteFile("/build.gradle.kts", nil, 0o644)
	entries, err = disco2.Discover(context.Background(), fs2, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages without BAFT.md, want 1", len(entries))
	}
	if entries[0].Capsule.CapsuleID != "com.example" {
		t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example")
	}
	if entries[0].LangName != "java" {
		t.Errorf("LangName = %q, want %q", entries[0].LangName, "java")
	}

	// Legacy build.gradle
	disco3 := service.NewCapsuleDiscovery()
	Language{}.Register(disco3)
	fs3 := memfs.New()
	fs3.WriteFile("/src/main/java/com/example/domain/Model.java", []byte("package com.example.domain\nclass Model"), 0o644)
	fs3.WriteFile("/src/main/java/com/example/api/Controller.java", []byte("package com.example.api\nclass Controller"), 0o644)
	fs3.WriteFile("/build.gradle", nil, 0o644)
	fs3.WriteFile("/BAFT.md", []byte("```mermaid\nflowchart TD\n    A[\"src/main/java/com/example/domain\"]\n```\n"), 0o644)
	entries, err = disco3.Discover(context.Background(), fs3, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages with build.gradle, want 1", len(entries))
	}
	if entries[0].Capsule.CapsuleID != "com.example" {
		t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example")
	}
	if entries[0].LangName != "java" {
		t.Errorf("LangName = %q, want %q", entries[0].LangName, "java")
	}
}

func TestDiscover_SkipsBuildDirs(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/src/main/java/com/example/Main.java", nil, 0o644)
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/BAFT.md", []byte("```mermaid\nflowchart TD\n    A[\"src/main/java/com/example\"]\n```\n"), 0o644)
	fs.WriteFile("/build/generated/java/com/example/Generated.java", nil, 0o644)
	fs.WriteFile("/build/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/build/BAFT.md", []byte("```mermaid\nflowchart TD\n    A[\"src/main/java/com/example\"]\n```\n"), 0o644)

	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	entries, err := disco.Discover(context.Background(), fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages, want 1 (build dir should be skipped)", len(entries))
	}
	if entries[0].Capsule.CapsuleID != "com.example" {
		t.Errorf("CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example")
	}
	if entries[0].LangName != "java" {
		t.Errorf("LangName = %q, want %q", entries[0].LangName, "java")
	}
}

func TestDiscoverDraft_MultiModuleWithRoot(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/module-a/src/main/java/com/example/a/Model.java", nil, 0o644)
	fs.WriteFile("/module-a/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/module-b/src/main/java/com/example/b/Service.java", nil, 0o644)
	fs.WriteFile("/module-b/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/module-c/build.gradle.kts", nil, 0o644)

	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	entries, err := disco.Discover(context.Background(), fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d packages, want 2 (root and module-c should be skipped)", len(entries))
	}

	if port.Label(entries[0].Capsule) != entries[0].Capsule.Dir {
		t.Errorf("pkgs[0] label = %q, want %q", port.Label(entries[0].Capsule), entries[0].Capsule.Dir)
	}
	if entries[0].Capsule.CapsuleID != "com.example.a" {
		t.Errorf("pkgs[0].CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example.a")
	}
	if entries[0].LangName != "java" {
		t.Errorf("pkgs[0].LangName = %q, want %q", entries[0].LangName, "java")
	}

	if port.Label(entries[1].Capsule) != entries[1].Capsule.Dir {
		t.Errorf("pkgs[1] label = %q, want %q", port.Label(entries[1].Capsule), entries[1].Capsule.Dir)
	}
	if entries[1].Capsule.CapsuleID != "com.example.b" {
		t.Errorf("pkgs[1].CapsuleID = %q, want %q", entries[1].Capsule.CapsuleID, "com.example.b")
	}
	if entries[1].LangName != "java" {
		t.Errorf("pkgs[1].LangName = %q, want %q", entries[1].LangName, "java")
	}
}

func TestDiscoverDraft_RootProjectNoSource(t *testing.T) {
	fs := memfs.New()
	fs.WriteFile("/build.gradle.kts", nil, 0o644)
	fs.WriteFile("/settings.gradle.kts", nil, 0o644)
	fs.WriteFile("/core/src/main/java/com/example/core/Core.java", nil, 0o644)
	fs.WriteFile("/core/build.gradle.kts", nil, 0o644)

	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	entries, err := disco.Discover(context.Background(), fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d packages, want 1", len(entries))
	}
	if port.Label(entries[0].Capsule) != entries[0].Capsule.Dir {
		t.Errorf("pkgs[0] label = %q, want %q", port.Label(entries[0].Capsule), entries[0].Capsule.Dir)
	}
	if entries[0].Capsule.CapsuleID != "com.example.core" {
		t.Errorf("pkgs[0].CapsuleID = %q, want %q", entries[0].Capsule.CapsuleID, "com.example.core")
	}
	if entries[0].LangName != "java" {
		t.Errorf("pkgs[0].LangName = %q, want %q", entries[0].LangName, "java")
	}
}
