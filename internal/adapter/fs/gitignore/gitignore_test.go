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

func TestGlobMatch_TrailingDoubleStarWithMiddleSegment(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		path      []string
		isDir     bool
		wantMatch bool
	}{
		{
			name:      "**/__pycache__/** matches path containing __pycache__",
			pattern:   "**/__pycache__/**",
			path:      []string{"src", "__pycache__", "foo.pyc"},
			isDir:     false,
			wantMatch: true,
		},
		{
			name:      "**/__pycache__/** matches __pycache__ directory",
			pattern:   "**/__pycache__/**",
			path:      []string{"src", "__pycache__"},
			isDir:     true,
			wantMatch: true,
		},
		{
			name:      "**/__pycache__/** does not match path without __pycache__",
			pattern:   "**/__pycache__/**",
			path:      []string{"src", "main", "java"},
			isDir:     true,
			wantMatch: false,
		},
		{
			name:      "**/nonexistent/** does not match arbitrary directory",
			pattern:   "**/nonexistent/**",
			path:      []string{"features", "mod", "src", "main", "java"},
			isDir:     true,
			wantMatch: false,
		},
		{
			name:      "**/nonexistent/** does not match any directory",
			pattern:   "**/nonexistent/**",
			path:      []string{"anything"},
			isDir:     true,
			wantMatch: false,
		},
		{
			name:      "**/node_modules/** matches path with node_modules",
			pattern:   "**/node_modules/**",
			path:      []string{"project", "node_modules", "package", "index.js"},
			isDir:     false,
			wantMatch: true,
		},
		{
			name:      "**/node_modules/** does not match path without node_modules",
			pattern:   "**/node_modules/**",
			path:      []string{"project", "src", "index.js"},
			isDir:     false,
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ParsePattern(tt.pattern, nil)
			result := p.Match(tt.path, tt.isDir)
			gotMatch := result != NoMatch
			if gotMatch != tt.wantMatch {
				t.Errorf("pattern %q path %v isDir=%v: got match=%v, want match=%v",
					tt.pattern, tt.path, tt.isDir, gotMatch, tt.wantMatch)
			}
		})
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
