package dart

import (
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
	"github.com/dariushalipour/baft/internal/port"
)

func TestIsGovernedFile(t *testing.T) {
	l := Language{}
	cases := map[string]bool{
		"lib/app.dart":                    true,
		"lib/src/ports/foo.dart":          true,
		"lib/src/models/foo.g.dart":       false,
		"lib/src/models/foo.freezed.dart": false,
		"lib/src/models/foo_test.dart":    false,
		"test/some_test.dart":             false,
		"test/helper.dart":                false,
		"bin/tool.dart":                   false,
		"lib/app.md":                      false,
		"lib/src/deep/nested/ok.dart":     true,
	}
	for rel, want := range cases {
		if got := l.IsGovernedFile(rel); got != want {
			t.Errorf("IsGovernedFile(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestParseImports(t *testing.T) {
	src := `// header
library foo;

import 'dart:async';
import 'package:my_app/src/ports/clock.dart';
import  "package:other_pkg/x.dart"  as o;
export 'src/models/foo.dart' show Foo, Bar;
part 'src/models/foo_impl.dart';
part of 'foo.dart';

// import 'commented_out.dart'; -- must NOT match (leading //)
`
	fs := memfs.New()
	fs.WriteFile("/sample.dart", []byte(src), 0o644)
	got, err := Language{}.ParseImports(fs, "/sample.dart")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"dart:async",
		"package:my_app/src/ports/clock.dart",
		"package:other_pkg/x.dart",
		"src/models/foo.dart",
		"src/models/foo_impl.dart",
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

func TestResolveInternalTarget(t *testing.T) {
	l := Language{}
	capsule := port.Capsule{CapsuleID: "my_app"}

	type tc struct {
		spec     string
		fileRel  string
		wantPath string
		wantIntl bool
	}
	cases := []tc{
		{"dart:async", "lib/app.dart", "", false},
		{"package:my_app/src/ports/clock.dart", "lib/app.dart", "lib/src/ports/clock.dart", true},
		{"package:my_app/app.dart", "lib/src/use_cases/x.dart", "lib/app.dart", true},
		{"package:http/http.dart", "lib/app.dart", "", false},
		{"foo.dart", "lib/src/ports/a.dart", "lib/src/ports/foo.dart", true},
		{"../models/foo.dart", "lib/src/ports/a.dart", "lib/src/models/foo.dart", true},
		{"./sub/bar.dart", "lib/src/ports/a.dart", "lib/src/ports/sub/bar.dart", true},
		{"../../outside.dart", "lib/app.dart", "", false},
	}
	for _, c := range cases {
		gotPath, gotIntl := l.ResolveInternalTarget(nil, port.ImportSpec{Path: c.spec}, capsule, c.fileRel)
		if gotPath != c.wantPath || gotIntl != c.wantIntl {
			t.Errorf("ResolveInternalTarget(%q, file=%q) = (%q, %v), want (%q, %v)",
				c.spec, c.fileRel, gotPath, gotIntl, c.wantPath, c.wantIntl)
		}
	}
}

func TestReadPubspecName(t *testing.T) {
	fs := memfs.New()
	content := "# comment\nname: my_app\nversion: 0.0.1\n"
	fs.WriteFile("/pubspec.yaml", []byte(content), 0o644)
	got, err := readPubspecName(fs, "/pubspec.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got != "my_app" {
		t.Fatalf("got %q", got)
	}
}

func TestSkipDirs(t *testing.T) {
	l := Language{}
	skip := l.SkipDirs()
	if len(skip) != 2 {
		t.Errorf("expected 2 skip dirs, got %d", len(skip))
	}
	for _, dir := range []string{".dart_tool", ".pub"} {
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
