package csharp

import (
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/port"
)

func TestName(t *testing.T) {
	if got := (Language{}).Name(); got != "csharp" {
		t.Errorf("Name() = %q, want %q", got, "csharp")
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
		"src/Api/Controller.cs":        true,
		"Program.cs":                   true,
		"Domain/Model.cs":              true,
		"Tests/TestController.cs":      true,
		"src/Api/Controller.csx":       false,
		"build.gradle":                 false,
		"README.md":                    false,
		"appsettings.json":             false,
		"Views/MyView.Designer.cs":     false,
		"Generated/Model.generated.cs": false,
	}
	for rel, want := range cases {
		if got := l.IsScannableFile(rel); got != want {
			t.Errorf("IsScannableFile(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestParseImports(t *testing.T) {
	src := `using System;
using System.Collections.Generic;
using MyApp.Domain.Entities;
using MyApp.Infra.Repository;
using Db = MyApp.Infra.Database;
using static System.Math;
using MyApp.Api.Extensions;

namespace MyApp.Api
{
    public class Controller {}
}`
	fs := memfs.New()
	fs.WriteFile("/Controller.cs", []byte(src), 0o644)
	got, err := Language{}.ParseImports(fs, "/Controller.cs")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		path      string
		namespace string
		line      int
		col       int
	}{
		{"System", "System", 1, 7},
		{"System.Collections.Generic", "System.Collections.Generic", 2, 7},
		{"MyApp.Domain.Entities", "MyApp.Domain.Entities", 3, 7},
		{"MyApp.Infra.Repository", "MyApp.Infra.Repository", 4, 7},
		{"MyApp.Infra.Database", "MyApp.Infra.Database", 5, 7},
		// Static import "using static System.Math" is skipped — not a dependency
		{"MyApp.Api.Extensions", "MyApp.Api.Extensions", 7, 7},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d imports, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Path != want[i].path {
			t.Errorf("[%d] Path = %q, want %q", i, got[i].Path, want[i].path)
		}
		if got[i].Namespace != want[i].namespace {
			t.Errorf("[%d] Namespace = %q, want %q", i, got[i].Namespace, want[i].namespace)
		}
		if got[i].Line != want[i].line {
			t.Errorf("[%d] Line = %d, want %d", i, got[i].Line, want[i].line)
		}
		if got[i].Col != want[i].col {
			t.Errorf("[%d] Col = %d, want %d", i, got[i].Col, want[i].col)
		}
	}
}

func TestParseImports_GlobalUsing(t *testing.T) {
	src := `global using System;
global using MyApp.Shared;
`
	fs := memfs.New()
	fs.WriteFile("/Usings.cs", []byte(src), 0o644)
	got, err := Language{}.ParseImports(fs, "/Usings.cs")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d imports, want 2", len(got))
	}
	if got[0].Namespace != "System" {
		t.Errorf("got namespace %q, want %q", got[0].Namespace, "System")
	}
	if got[1].Namespace != "MyApp.Shared" {
		t.Errorf("got namespace %q, want %q", got[1].Namespace, "MyApp.Shared")
	}
}

func TestGetFileNamespace(t *testing.T) {
	src := `using System;

namespace MyApp.Api.Controllers
{
    public class HomeController {}
}`
	fs := memfs.New()
	fs.WriteFile("/HomeController.cs", []byte(src), 0o644)
	got, err := Language{}.GetFileNamespace(fs, "/HomeController.cs")
	if err != nil {
		t.Fatal(err)
	}
	if got != "MyApp.Api.Controllers" {
		t.Errorf("got %q, want %q", got, "MyApp.Api.Controllers")
	}
}

func TestGetFileNamespace_NoNamespace(t *testing.T) {
	src := `using System;

public class Script {}
`
	fs := memfs.New()
	fs.WriteFile("/Script.cs", []byte(src), 0o644)
	got, err := Language{}.GetFileNamespace(fs, "/Script.cs")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveInternalTarget(t *testing.T) {
	l := Language{}
	capsule := port.Capsule{CapsuleID: "MyApp"}

	type tc struct {
		spec     string
		fileRel  string
		wantPath string
		wantIntl bool
	}
	cases := []tc{
		// External
		{"System", "src/Controller.cs", "", false},
		{"System.Collections.Generic", "src/Controller.cs", "", false},
		{"Microsoft.Extensions.DependencyInjection", "src/Controller.cs", "", false},

		// Internal — exact base package match
		{"MyApp", "src/Controller.cs", ".", true},

		// Internal — sub-namespaces
		{"MyApp.Domain.Entities", "src/Controller.cs", "Domain/Entities", true},
		{"MyApp.Infra.Repository", "src/Controller.cs", "Infra/Repository", true},
		{"MyApp.Api.Extensions", "src/Controller.cs", "Api/Extensions", true},

		// Word boundary: MyApp2 must NOT match MyApp
		{"MyApp2.Domain.Model", "src/Controller.cs", "", false},
		{"MyAppApi.Controller", "src/Controller.cs", "", false},
	}
	for _, c := range cases {
		gotPath, gotIntl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: c.spec}, capsule, c.fileRel)
		if gotPath != c.wantPath || gotIntl != c.wantIntl {
			t.Errorf("ResolveInternalTarget(%q) = (%q, %v), want (%q, %v)",
				c.spec, gotPath, gotIntl, c.wantPath, c.wantIntl)
		}
	}
}

func TestIsInternalCapsule(t *testing.T) {
	cases := []struct {
		spec string
		base string
		want bool
	}{
		{"MyApp", "MyApp", true},
		{"MyApp.Domain", "MyApp", true},
		{"MyApp.Domain.Entities", "MyApp", true},
		{"MyApp2.Domain", "MyApp", false},
		{"MyApp2", "MyApp", false},
		{"MyAppApi.Controller", "MyApp", false},
		{"Other.Domain", "MyApp", false},
		{"My", "MyApp", false},
		{"MyApp.", "MyApp", false},
		{"", "MyApp", false},
	}
	for _, c := range cases {
		if got := isInternalCapsule(c.spec, c.base); got != c.want {
			t.Errorf("isInternalCapsule(%q, %q) = %v, want %v", c.spec, c.base, got, c.want)
		}
	}
}

// TestParseImports_ColEnd_AliasedImport verifies that ColEnd covers the visible
// alias token (e.g., "Db"), not the resolved namespace ("MyApp.Infra.Database").
// Regression: ColEnd was incorrectly using the resolved namespace length.
func TestParseImports_ColEnd_AliasedImport(t *testing.T) {
	src := `using System;
using System.Collections.Generic;
using MyApp.Domain.Entities;
using Db = MyApp.Infra.Database;
`
	fs := memfs.New()
	fs.WriteFile("/Controller.cs", []byte(src), 0o644)
	got, err := Language{}.ParseImports(fs, "/Controller.cs")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d imports, want 4", len(got))
	}

	// Non-aliased: ColEnd = Col + len("System") = 7 + 6 = 13
	if got[0].Path != "System" || got[0].Col != 7 || got[0].ColEnd != 13 {
		t.Errorf("[0] Path=%q Col=%d ColEnd=%d, want Path=%q Col=%d ColEnd=%d",
			got[0].Path, got[0].Col, got[0].ColEnd, "System", 7, 13)
	}

	// Non-aliased: ColEnd = Col + len("System.Collections.Generic") = 7 + 26 = 33
	if got[1].Path != "System.Collections.Generic" || got[1].Col != 7 || got[1].ColEnd != 33 {
		t.Errorf("[1] Path=%q Col=%d ColEnd=%d, want Path=%q Col=%d ColEnd=%d",
			got[1].Path, got[1].Col, got[1].ColEnd, "System.Collections.Generic", 7, 33)
	}

	// Non-aliased: ColEnd = Col + len("MyApp.Domain.Entities") = 7 + 21 = 28
	if got[2].Path != "MyApp.Domain.Entities" || got[2].Col != 7 || got[2].ColEnd != 28 {
		t.Errorf("[2] Path=%q Col=%d ColEnd=%d, want Path=%q Col=%d ColEnd=%d",
			got[2].Path, got[2].Col, got[2].ColEnd, "MyApp.Domain.Entities", 7, 28)
	}

	// Aliased: Path is the resolved namespace, but Col/ColEnd cover "Db" (2 chars)
	// ColEnd = Col + len("Db") = 7 + 2 = 9, NOT Col + len("MyApp.Infra.Database")
	if got[3].Path != "MyApp.Infra.Database" {
		t.Errorf("[3] Path = %q, want %q", got[3].Path, "MyApp.Infra.Database")
	}
	if got[3].Col != 7 {
		t.Errorf("[3] Col = %d, want 7", got[3].Col)
	}
	if got[3].ColEnd != 9 {
		t.Errorf("[3] ColEnd = %d, want 9 (alias length, not namespace length)", got[3].ColEnd)
	}
}

func TestExtractCsprojName(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"<RootNamespace>MyApp</RootNamespace>", "MyApp"},
		{"<AssemblyName>MyApp</AssemblyName>", "MyApp"},
		{"<RootNamespace>MyApp.Domain</RootNamespace>", "MyApp.Domain"},
		{"<OutputType>Exe</OutputType>", ""},
		{"<TargetFramework>net8.0</TargetFramework>", ""},
	}
	for _, c := range cases {
		if got := extractCsprojName(c.line); got != c.want {
			t.Errorf("extractCsprojName(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestReadCsprojName(t *testing.T) {
	// Test with RootNamespace
	fs := memfs.New()
	fs.WriteFile("/MyApp.csproj", []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <RootNamespace>MyApp</RootNamespace>
  </PropertyGroup>
</Project>`), 0o644)
	got, err := ReadCsprojName(fs, "/MyApp.csproj")
	if err != nil {
		t.Fatal(err)
	}
	if got != "MyApp" {
		t.Errorf("got %q, want %q", got, "MyApp")
	}

	// Test with AssemblyName fallback
	fs2 := memfs.New()
	fs2.WriteFile("/MyApp.csproj", []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <AssemblyName>MyApp.Service</AssemblyName>
  </PropertyGroup>
</Project>`), 0o644)
	got2, err := ReadCsprojName(fs2, "/MyApp.csproj")
	if err != nil {
		t.Fatal(err)
	}
	if got2 != "MyApp.Service" {
		t.Errorf("got %q, want %q", got2, "MyApp.Service")
	}

	// Test fallback to directory name
	fs3 := memfs.New()
	fs3.WriteFile("/MyApp.csproj", []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>
</Project>`), 0o644)
	got3, err := ReadCsprojName(fs3, "/MyApp.csproj")
	if err != nil {
		t.Fatal(err)
	}
	if got3 != "MyApp" {
		t.Errorf("got %q, want %q", got3, "MyApp")
	}
}
