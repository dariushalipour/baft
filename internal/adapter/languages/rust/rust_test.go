package rust

import (
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

func TestIsScannableFile(t *testing.T) {
	l := Language{}
	cases := map[string]bool{
		// Scannable: any .rs file
		"src/main.rs":               true,
		"src/lib.rs":                true,
		"src/domain/model.rs":       true,
		"src/api/handler.rs":        true,
		"src/deep/nested/module.rs": true,
		"tests/integration.rs":      true,
		"benches/benchmark.rs":      true,
		"examples/demo.rs":          true,
		"build.rs":                  true,
		"src/bin/cli.rs":            true,
		"src/examples/demo.rs":      true,

		// Not scannable: wrong extension
		"src/lib.toml":    false,
		"README.md":       false,
		"src/config.json": false,
		"Cargo.toml":      false,
	}
	for rel, want := range cases {
		t.Run(rel, func(t *testing.T) {
			if got := l.IsScannableFile(rel); got != want {
				t.Errorf("IsScannableFile(%q) = %v, want %v", rel, got, want)
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
			label: "crate imports",
			src: `use crate::domain::Model;
use crate::api::handler::Handler;
use crate::infra::config;
fn main() {}
`,
			want: []string{
				"crate::domain::Model",
				"crate::api::handler::Handler",
				"crate::infra::config",
			},
		},
		{
			label: "super and self imports",
			src: `use super::Model;
use super::utils::format;
use self::helpers;
use self::internal::private;
fn main() {}
`,
			want: []string{
				"super::Model",
				"super::utils::format",
				"self::helpers",
				"self::internal::private",
			},
		},
		{
			label: "external crate imports",
			src: `use serde::Serialize;
use tokio::runtime::Runtime;
use anyhow::Result;
fn main() {}
`,
			want: []string{
				"serde::Serialize",
				"tokio::runtime::Runtime",
				"anyhow::Result",
			},
		},
		{
			label: "mod statements",
			src: `mod domain;
mod api;
mod infra;
mod config;
fn main() {}
`,
			want: []string{
				"domain",
				"api",
				"infra",
				"config",
			},
		},
		{
			label: "pub mod statements",
			src: `pub mod domain;
pub(crate) mod api;
pub(super) mod infra;
mod config;
fn main() {}
`,
			want: []string{
				"domain",
				"api",
				"infra",
				"config",
			},
		},
		{
			label: "mixed imports",
			src: `use crate::domain::Model;
use super::utils;
mod infra;
use serde::Serialize;
fn main() {}
`,
			want: []string{
				"crate::domain::Model",
				"super::utils",
				"infra",
				"serde::Serialize",
			},
		},
		{
			label: "no imports",
			src: `fn main() {}
`,
			want: []string{},
		},
		{
			label: "nested braces",
			src: `use crate::domain::{Model, Repository};
use std::collections::{HashMap, HashSet};
fn main() {}
`,
			want: []string{
				"crate::domain::{Model, Repository}",
				"std::collections::{HashMap, HashSet}",
			},
		},
		{
			label: "wildcard glob imports",
			src: `use std::collections::*;
use crate::mymodule::*;
use super::*;
use self::*;
fn main() {}
`,
			want: []string{
				"std::collections::*",
				"crate::mymodule::*",
				"super::*",
				"self::*",
			},
		},
		{
			label: "grouped nested imports",
			src: `use std::{fmt, io};
use std::{fmt::Display, fmt::Debug};
use std::fmt::{Display, Debug};
use std::fmt::{Display, Debug, self};
use std::{fmt::{Display, Debug}, io::{Read, Write}};
use std::{self, fmt, io};
use crate::{mymodule, othermodule::MyStruct};
fn main() {}
`,
			want: []string{
				"std::{fmt, io}",
				"std::{fmt::Display, fmt::Debug}",
				"std::fmt::{Display, Debug}",
				"std::fmt::{Display, Debug, self}",
				"std::{fmt::{Display, Debug}, io::{Read, Write}}",
				"std::{self, fmt, io}",
				"crate::{mymodule, othermodule::MyStruct}",
			},
		},
		{
			label: "aliased imports",
			src: `use std::fmt::Display as Disp;
use std::io::Result as IoResult;
use crate::mymodule::MyStruct as Alias;
use std::{fmt::Display as Disp, io::Result as IoResult};
fn main() {}
`,
			want: []string{
				"std::fmt::Display as Disp",
				"std::io::Result as IoResult",
				"crate::mymodule::MyStruct as Alias",
				"std::{fmt::Display as Disp, io::Result as IoResult}",
			},
		},
		{
			label: "pub use re-exports",
			src: `pub use std::fmt::Display;
pub use crate::mymodule::MyStruct;
pub use crate::mymodule::*;
pub use std::fmt::{Display, Debug};
pub use std::fmt::Display as Disp;
fn main() {}
`,
			want: []string{
				"std::fmt::Display",
				"crate::mymodule::MyStruct",
				"crate::mymodule::*",
				"std::fmt::{Display, Debug}",
				"std::fmt::Display as Disp",
			},
		},
		{
			label: "scoped pub use re-exports",
			src: `pub(crate) use crate::mymodule::MyStruct;
pub(super) use crate::mymodule::MyEnum;
pub(self) use crate::mymodule::MyTrait;
pub(in crate::mymodule) use crate::mymodule::MyType;
fn main() {}
`,
			want: []string{
				"crate::mymodule::MyStruct",
				"crate::mymodule::MyEnum",
				"crate::mymodule::MyTrait",
				"crate::mymodule::MyType",
			},
		},
		{
			label: "extern crate declarations",
			src: `extern crate serde;
extern crate serde as s;
extern crate std;
fn main() {}
`,
			want: []string{
				"serde",
				"std",
			},
		},
		{
			label: "macro imports",
			src: `use serde::Serialize;
use serde_derive::Serialize;
use lazy_static::lazy_static;
use std::vec;
use crate::my_macro;
fn main() {}
`,
			want: []string{
				"serde::Serialize",
				"serde_derive::Serialize",
				"lazy_static::lazy_static",
				"std::vec",
				"crate::my_macro",
			},
		},
		{
			label: "standard single imports",
			src: `use std::fmt;
use std::fmt::Display;
use std::fmt::Display as Disp;
use std::collections::HashMap;
use crate::mymodule;
use crate::mymodule::MyStruct;
use super::mymodule;
use super::mymodule::MyStruct;
use self::mymodule;
use self::mymodule::MyStruct;
fn main() {}
`,
			want: []string{
				"std::fmt",
				"std::fmt::Display",
				"std::fmt::Display as Disp",
				"std::collections::HashMap",
				"crate::mymodule",
				"crate::mymodule::MyStruct",
				"super::mymodule",
				"super::mymodule::MyStruct",
				"self::mymodule",
				"self::mymodule::MyStruct",
			},
		},
		{
			label: "indented imports",
			src: `    use crate::domain::Model;
        pub use crate::api::Handler;
    pub(crate) use crate::infra::Config;
        mod utils;
fn main() {}
`,
			want: []string{
				"crate::domain::Model",
				"crate::api::Handler",
				"crate::infra::Config",
				"utils",
			},
		},
		{
			label: "duplicate imports deduped",
			src: `use crate::domain::Model;
use crate::domain::Model;
use serde::Serialize;
use serde::Serialize;
mod infra;
mod infra;
fn main() {}
`,
			want: []string{
				"crate::domain::Model",
				"serde::Serialize",
				"infra",
			},
		},
		{
			label: "deeply nested super imports",
			src: `use super::super::super::config;
use super::super::utils::helpers;
fn main() {}
`,
			want: []string{
				"super::super::super::config",
				"super::super::utils::helpers",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			fs := memfs.New()
			fs.WriteFile("/lib.rs", []byte(c.src), 0o644)
			got, err := Language{}.ParseImports(fs, "/lib.rs")
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

func TestResolveInternalTarget(t *testing.T) {
	l := Language{}
	capsule := port.Capsule{CapsuleID: "my_crate"}

	type tc struct {
		spec     string
		fileRel  string
		wantPath string
		wantIntl bool
	}
	cases := []tc{
		// crate:: imports
		{"crate::domain::Model", "src/lib.rs", "src/domain", true},
		{"crate::api::handler::Handler", "src/lib.rs", "src/api/handler", true},
		{"crate::infra::config", "src/domain/model.rs", "src/infra", true},

		// super:: imports
		{"super::Model", "src/domain/model.rs", "src/domain", true},
		{"super::utils::format", "src/domain/model.rs", "src/domain/utils", true},
		{"super::super::config", "src/domain/model.rs", "src", true},

		// self:: imports
		{"self::helpers", "src/lib.rs", "src", true},
		{"self::internal::private", "src/lib.rs", "src/internal", true},

		// External crates
		{"serde::Serialize", "src/lib.rs", "", false},
		{"tokio::runtime::Runtime", "src/lib.rs", "", false},
		{"std::collections::HashMap", "src/lib.rs", "", false},

		// Same crate by name
		{"my_crate::domain::Model", "src/lib.rs", "src/domain", true},

		// Bare module names (from mod statements)
		{"domain", "src/lib.rs", "src/domain", true},
		{"api", "src/lib.rs", "src/api", true},

		// Aliased imports - "as" clause should be stripped
		{"std::fmt::Display as Disp", "src/lib.rs", "", false},
		{"crate::domain::Model as M", "src/lib.rs", "src/domain", true},
		{"crate::api::handler::Handler as H", "src/lib.rs", "src/api/handler", true},
		{"super::Model as M", "src/domain/model.rs", "src/domain", true},
		{"self::helpers as H", "src/lib.rs", "src", true},
		{"my_crate::domain::Model as M", "src/lib.rs", "src/domain", true},

		// Wildcard imports
		{"std::collections::*", "src/lib.rs", "", false},
		{"crate::domain::*", "src/lib.rs", "src/domain", true},
		{"super::*", "src/domain/model.rs", "src/domain", true},
		{"self::*", "src/lib.rs", "src", true},

		// Grouped imports (resolved as-is, braces in path)
		{"std::{fmt, io}", "src/lib.rs", "", false},
		{"crate::{domain, api}", "src/lib.rs", "src", true},

		// Deeply nested super
		{"super::super::super::config", "src/domain/model.rs", "src", true},
		{"super::super::utils::helpers", "src/domain/model.rs", "src/utils", true},

		// Bare module names
		{"domain", "src/domain/model.rs", "src/domain/domain", true},
		{"config", "src/lib.rs", "src/config", true},

		// External crates (non-internal)
		{"tokio::runtime::Runtime", "src/lib.rs", "", false},
		{"anyhow::Result", "src/lib.rs", "", false},
		{"serde::Serialize", "src/lib.rs", "", false},
		{"serde_derive::Serialize", "src/lib.rs", "", false},
		{"lazy_static::lazy_static", "src/lib.rs", "", false},
		{"reqwest::Client", "src/lib.rs", "", false},

		// Same crate by name variations
		{"my_crate::domain::Model", "src/domain/model.rs", "src/domain", true},
		{"my_crate::api", "src/lib.rs", "src", true},
		{"my_crate", "src/lib.rs", "src/my_crate", true},
	}
	for _, c := range cases {
		t.Run(c.spec, func(t *testing.T) {
			gotPath, gotIntl := l.ResolveInternalTarget(memfs.New(), port.ImportSpec{Path: c.spec}, capsule, c.fileRel)
			if gotPath != c.wantPath || gotIntl != c.wantIntl {
				t.Errorf("ResolveInternalTarget(%q, file=%q) = (%q, %v), want (%q, %v)",
					c.spec, c.fileRel, gotPath, gotIntl, c.wantPath, c.wantIntl)
			}
		})
	}
}

func TestReadCargoToml(t *testing.T) {
	type tc struct {
		label   string
		content string
		want    string
		wantErr bool
	}
	cases := []tc{
		{
			label:   "standard",
			content: "[package]\nname = \"my_crate\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
			want:    "my_crate",
		},
		{
			label:   "with workspace",
			content: "[package]\nname = \"my_crate\"\nversion.workspace = true\nedition.workspace = true\n",
			want:    "my_crate",
		},
		{
			label:   "with leading comment",
			content: "// My crate\n\n[package]\nname = \"my_crate\"\nversion = \"0.1.0\"\n",
			want:    "my_crate",
		},
		{
			label:   "with dependencies",
			content: "[package]\nname = \"my_crate\"\nversion = \"0.1.0\"\n\n[dependencies]\nserde = \"1.0\"\n",
			want:    "my_crate",
		},
		{
			label:   "no package name",
			content: "[dependencies]\nserde = \"1.0\"\n",
			wantErr: true,
		},
		{
			label:   "empty file",
			content: "",
			wantErr: true,
		},
		{
			label:   "inline table version",
			content: "[package]\nname = \"my_crate\"\nversion = { workspace = true }\nedition = \"2021\"\n",
			want:    "my_crate",
		},
		{
			label:   "inline table with multiple fields",
			content: "[package]\nname = \"my_crate\"\nversion = \"0.1.0\"\nauthors = { workspace = true }\nedition = \"2021\"\n",
			want:    "my_crate",
		},
		{
			label:   "name as inline table",
			content: "[package]\nname = { value = \"my_crate\" }\nversion = \"0.1.0\"\nedition = \"2021\"\n",
			want:    "my_crate",
		},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			fs := memfs.New()
			fs.WriteFile("/Cargo.toml", []byte(c.content), 0o644)
			got, err := readCargoToml(fs, "/Cargo.toml")
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

func TestName(t *testing.T) {
	l := Language{}
	if got := l.Name(); got != "rust" {
		t.Errorf("Name() = %q, want %q", got, "rust")
	}
}

func TestSupportsFileGlobs(t *testing.T) {
	l := Language{}
	if got := l.SupportsFileGlobs(); got != false {
		t.Errorf("SupportsFileGlobs() = %v, want false", got)
	}
}

func TestDiscover(t *testing.T) {
	cargoToml := `[package]
name = "my_crate"
version = "0.1.0"
edition = "2021"
`
	baftMd := "```mermaid\nflowchart TD\n  a[\"src/**\"]\n  b[\"src/domain/**\"]\n  a --> b\n```\n"

	fs := memfs.New()
	fs.WriteFile("/my_crate/Cargo.toml", []byte(cargoToml), 0o644)
	fs.WriteFile("/my_crate/BAFT.md", []byte(baftMd), 0o644)
	fs.WriteFile("/other_crate/Cargo.toml", []byte(cargoToml), 0o644)
	fs.WriteFile("/no_cargo/BAFT.md", []byte(baftMd), 0o644)

	disco := service.NewCapsuleDiscovery()
	Language{}.Register(disco)
	got, err := disco.Discover(fs, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d packages, want 2 (my_crate and other_crate)", len(got))
	}
	if port.Label(got[0].Capsule) != got[0].Capsule.Dir {
		t.Errorf("got label %q, want %q", port.Label(got[0].Capsule), got[0].Capsule.Dir)
	}
	if port.Label(got[1].Capsule) != got[1].Capsule.Dir {
		t.Errorf("got label %q, want %q", port.Label(got[1].Capsule), got[1].Capsule.Dir)
	}
}
