package namespaces

import (
	"testing"

	"github.com/dariushalipour/baft/internal/adapter/fs/memfs"
)

func TestIsInternal(t *testing.T) {
	cases := []struct {
		spec string
		base string
		want bool
	}{
		{"com.example", "com.example", true},
		{"com.example.domain", "com.example", true},
		{"com.example.domain.Model", "com.example", true},
		{"com.example.domain2", "com.example", true},
		{"com.example.a.b.c.d.Deep", "com.example", true},
		{"com.example.v2.Api", "com.example", true},
		{"com.example2", "com.example", false},
		{"com.example2.domain", "com.example", false},
		{"com.example2.v2.Api", "com.example", false},
		{"com.exampleapi.Controller", "com.example", false},
		{"com.example_internal", "com.example", false},
		{"com.example_internal.Foo", "com.example", false},
		{"com.other.domain", "com.example", false},
		{"com.ex", "com.example", false},
		{"com", "com.example", false},
		{"com.example.", "com.example", false},
		{"com.example.domain.", "com.example", false},
		{"", "com.example", false},
	}
	for _, c := range cases {
		if got := IsInternal(c.spec, c.base); got != c.want {
			t.Errorf("IsInternal(%q, %q) = %v, want %v", c.spec, c.base, got, c.want)
		}
	}
}

func TestCommonPrefix(t *testing.T) {
	cases := []struct {
		name    string
		files   []string
		srcDirs []string
		want    string
		wantErr bool
	}{
		{"single dir", []string{"/src/com/example/api/A.java"}, []string{"/src"}, "com.example.api", false},
		{"shared prefix", []string{"/src/com/example/api/A.java", "/src/com/example/domain/B.kt"}, []string{"/src"}, "com.example", false},
		{"prefix is not a path segment", []string{"/src/com/example/app/A.java", "/src/com/example/application/B.java"}, []string{"/src"}, "com.example", false},
		{"across source dirs", []string{"/a/com/example/api/A.java", "/b/com/example/domain/B.kt"}, []string{"/a", "/b"}, "com.example", false},
		{"no matching files", []string{"/src/com/example/A.txt"}, []string{"/src"}, "", true},
		{"files in root only", []string{"/src/A.java"}, []string{"/src"}, "", true},
		{"nothing in common", []string{"/src/com/example/A.java", "/src/org/other/B.java"}, []string{"/src"}, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := memfs.New()
			for _, f := range c.files {
				fs.WriteFile(f, nil, 0o644)
			}
			got, err := CommonPrefix(fs, c.srcDirs, []string{".java", ".kt", ".py"})
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
