package port

import (
	"testing"
)

func TestShouldSkipDir(t *testing.T) {
	skip := map[string]bool{
		".git":          true,
		".hg":           true,
		".svn":          true,
		".idea":         true,
		".vscode":       true,
		".vs":           true,
		"coverage":      true,
		"coverage.lcov": true,
		".hidden":       true,
	}
	for name, want := range skip {
		if got := ShouldSkipDir(name); got != want {
			t.Errorf("ShouldSkipDir(%q) = %v, want %v", name, got, want)
		}
	}

	keep := []string{
		".",
		"src",
		"lib",
		"internal",
		"packages",
		"cmd",
		"test",
		"main",
	}
	for _, name := range keep {
		if got := ShouldSkipDir(name); got {
			t.Errorf("ShouldSkipDir(%q) = true, want false", name)
		}
	}
}
