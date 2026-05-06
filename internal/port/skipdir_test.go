package port

import (
	"testing"
)

func TestShouldSkipDir(t *testing.T) {
	skip := map[string]bool{
		".git":         true,
		".hg":          true,
		"node_modules": true,
		"vendor":       true,
		".venv":        true,
		"venv":         true,
		"build":        true,
		"dist":         true,
		"out":          true,
		"target":       true,
		".next":        true,
		".dart_tool":   true,
		".pub":         true,
		".nuxt":        true,
		".kotlin":      true,
		"__pycache__":  true,
		".idea":        true,
		".vscode":      true,
		"coverage":     true,
		".hidden":      true,
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
