package gitignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const (
	gitignoreFile = ".gitignore"
)

// ReadPatterns reads .gitignore patterns recursively from the given root
// directory. Patterns are returned in ascending priority order (later = higher).
// Only .gitignore files are read (not .git/info/exclude or core.excludesfile).
func ReadPatterns(root string) ([]Pattern, error) {
	return readPatterns(root, nil)
}

func readPatterns(root string, domain []string) ([]Pattern, error) {
	var patterns []Pattern

	// Read .gitignore in the current directory
	gitignorePath := filepath.Join(root, gitignoreFile)
	ps, err := readIgnoreFile(gitignorePath, domain)
	if err != nil {
		return nil, err
	}
	patterns = append(patterns, ps...)

	// Recursively read subdirectories, skipping ignored dirs
	entries, err := os.ReadDir(root)
	if err != nil {
		return patterns, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		m := NewMatcher(patterns)
		if m.Match(append(domain, entry.Name()), true) {
			continue
		}

		subPath := filepath.Join(root, entry.Name())
		subps, err := readPatterns(subPath, append(domain, entry.Name()))
		if err != nil {
			return patterns, err
		}
		if len(subps) > 0 {
			patterns = append(patterns, subps...)
		}
	}

	return patterns, nil
}

func readIgnoreFile(path string, domain []string) ([]Pattern, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var patterns []Pattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		patterns = append(patterns, ParsePattern(line, domain))
	}

	return patterns, scanner.Err()
}
