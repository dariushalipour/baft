// Package treeview parses ASCII tree diagrams from doc strings
// and extracts relative file paths.
package treeview

import (
	"strings"
)

// Entry represents a single file path extracted from a tree doc string.
type Entry struct {
	// BaseDir is the root directory from the first line of the tree.
	BaseDir string
	// RelPath is the file path relative to BaseDir.
	RelPath string
}

// ParseTree parses a tree doc string (without a root dir line) and returns
// all file entries with the given rootDir as the base directory.
//
// The doc uses tree characters (├, │, └, ─, spaces) to show directory
// structure. Only leaf entries that do not end with "/" are returned as file
// paths.
//
// The parser uses fixed 3-character slots:
//
//	"│  " = continuation from parent level
//	"   " = empty slot
//	"├  " = branch (current level)
//	"└  " = last branch (current level)
//
// Example doc:
//
//	├─ billing/
//	│  ├─ go.mod
//	│  └─ domain/
//	│     └─ order.go
//
// With rootDir="/Users/jane/baft", returns:
//
//	Entries{{BaseDir: "/Users/jane/baft", RelPath: "billing/go.mod"},
//	        {BaseDir: "/Users/jane/baft", RelPath: "billing/domain/order.go"}}
func ParseTree(rootDir, doc string) []Entry {
	lines := strings.Split(doc, "\n")

	var entries []Entry
	var stack []string // 0-indexed stack of path components

	for _, line := range lines {
		depth, name := decodeTreeLine(line)
		if name == "" {
			continue
		}

		cleanName := strings.TrimSuffix(name, "/")
		isDir := strings.HasSuffix(name, "/")

		idx := depth
		if idx < len(stack) {
			stack = stack[:idx]
		}
		stack = append(stack, cleanName)

		if isDir {
			continue
		}

		relPath := strings.Join(stack, "/")
		entries = append(entries, Entry{BaseDir: rootDir, RelPath: relPath})
	}

	return entries
}

// Parse parses a tree doc string and returns all file entries.
//
// The first line must be an absolute path (the base directory).
// Subsequent lines use tree characters (├, │, └, ─, spaces) to show
// directory structure. Only leaf entries that do not end with "/" are
// returned as file paths.
//
// The parser uses fixed 3-character slots:
//
//	"│  " = continuation from parent level
//	"   " = empty slot
//	"├  " = branch (current level)
//	"└  " = last branch (current level)
//
// Example input:
//
//	/Users/jane/baft
//	├─ billing/
//	│  ├─ go.mod
//	│  └─ domain/
//	│     └─ order.go
//
// Returns:
//
//	Entries{{BaseDir: "/Users/jane/baft", RelPath: "billing/go.mod"},
//	        {BaseDir: "/Users/jane/baft", RelPath: "billing/domain/order.go"}}
func Parse(doc string) []Entry {
	lines := strings.Split(doc, "\n")
	if len(lines) == 0 {
		return nil
	}

	baseDir := extractBaseDir(lines[0])
	if baseDir == "" {
		return nil
	}

	return ParseTree(baseDir, strings.Join(lines[1:], "\n"))
}

// extractBaseDir extracts the absolute path from the first line of a tree.
func extractBaseDir(line string) string {
	line = strings.TrimSpace(line)
	parts := strings.Fields(line)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// decodeTreeLine decodes a tree line into its depth level and name.
//
// It parses the line as a sequence of 3-character slots:
//
//	"│  " = continuation from parent level
//	"   " = empty slot
//	"├  " = branch (current level)
//	"└  " = last branch (current level)
//
// The depth equals the number of ancestor slots (continuation or empty)
// before the branch slot. The name is the text after the branch connector.
func decodeTreeLine(line string) (depth int, name string) {
	runes := []rune(line)
	n := len(runes)

	i := 0
	for i < n {
		ch := runes[i]

		switch ch {
		case '├', '└':
			depth = i / 3
			j := i + 1
			if j < n && (runes[j] == '─' || runes[j] == '-' || runes[j] == ' ') {
				j++
			}
			for j < n && runes[j] == ' ' {
				j++
			}
			name = string(runes[j:])
			if name == "" || strings.TrimSpace(name) == "" {
				return 0, ""
			}
			return depth, strings.TrimSpace(name)

		case '│':
			if i+1 < n && (runes[i+1] == '─' || runes[i+1] == '-') {
				depth = i / 3
				j := i + 2
				name = string(runes[j:])
				if name == "" || strings.TrimSpace(name) == "" {
					return 0, ""
				}
				return depth, strings.TrimSpace(name)
			}
			i += 3

		case ' ', '─', '-':
			if i == 0 && (ch == '─' || ch == '-') {
				name = string(runes[1:])
				if name == "" || strings.TrimSpace(name) == "" {
					return 0, ""
				}
				return 0, strings.TrimSpace(name)
			}
			i += 3

		default:
			name = string(runes[i:])
			return 0, strings.TrimSpace(name)
		}
	}
	return 0, ""
}
