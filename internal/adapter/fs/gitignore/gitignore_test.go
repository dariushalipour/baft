package gitignore

import (
	"testing"
)

func TestGlobMatch_TrailingDoubleStar(t *testing.T) {
	p := ParsePattern("**", nil)
	path := []string{"a", "b", "c"}
	result := p.Match(path, true)
	if result == NoMatch {
		t.Fatal("expected '**' to match any path")
	}
}

func TestGlobMatch_DoubleStarMiddle(t *testing.T) {
	p := ParsePattern("a/**/c", nil)
	result := p.Match([]string{"a", "x", "b", "c"}, true)
	if result == NoMatch {
		t.Fatal("expected 'a/**/c' to match 'a/x/b/c'")
	}
}

func TestGlobMatch_DoubleStarEnd(t *testing.T) {
	p := ParsePattern("a/**", nil)
	result := p.Match([]string{"a", "b", "c"}, true)
	if result == NoMatch {
		t.Fatal("expected 'a/**' to match 'a/b/c'")
	}
}

func TestGlobMatch_SimpleName(t *testing.T) {
	p := ParsePattern("*.go", nil)
	result := p.Match([]string{"main.go"}, false)
	if result == NoMatch {
		t.Fatal("expected '*.go' to match 'main.go'")
	}
	result = p.Match([]string{"main.txt"}, false)
	if result != NoMatch {
		t.Fatal("expected '*.go' to not match 'main.txt'")
	}
}

func TestGlobMatch_TildeHome(t *testing.T) {
	p := ParsePattern("~/file.txt", nil)
	result := p.Match([]string{"~", "file.txt"}, false)
	if result == NoMatch {
		t.Fatal("expected '~/file.txt' to match")
	}
}

func TestGlobMatch_Negation(t *testing.T) {
	p := ParsePattern("!*.log", nil)
	result := p.Match([]string{"app.log"}, false)
	if result == Exclude {
		t.Fatal("expected '!*.log' to not match (negation)")
	}
}

func TestGlobMatch_DirOnly(t *testing.T) {
	p := ParsePattern("build/", nil)
	result := p.Match([]string{"build"}, false)
	if result != NoMatch {
		t.Fatal("expected 'build/' dir-only pattern to not match file")
	}
	result = p.Match([]string{"build"}, true)
	if result == NoMatch {
		t.Fatal("expected 'build/' dir-only pattern to match directory")
	}
}

func TestPatternMatcher_MultipleDirs(t *testing.T) {
	patterns := []Pattern{
		ParsePattern("*.log", nil),
		ParsePattern("temp*", nil),
	}
	m := NewMatcher(patterns)
	if m == nil {
		t.Fatal("expected non-nil matcher")
	}

	if !m.Match([]string{"src", "app.log"}, false) {
		t.Fatal("expected 'app.log' to match '*.log' in src/")
	}
	if !m.Match([]string{"build", "temp123"}, false) {
		t.Fatal("expected 'temp123' to match 'temp*' in build/")
	}
}
