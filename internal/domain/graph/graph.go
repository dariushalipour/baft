package graph

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Graph is the parsed contract from a STRATA.md mermaid block.
type Graph struct {
	Nodes     map[string]string
	Edges     map[string]map[string]bool
	Classes   map[string]map[string]bool
	NodeLines map[string]int
	EdgeLines map[string]int
}

func (g *Graph) IsEndophobic(nodeID string) bool {
	return g.Classes[nodeID]["endophobic"]
}

func (g *Graph) NodeForDir(dirPath string) string {
	if dirPath == "" {
		dirPath = "."
	}
	return g.findMostSpecific(func(pattern string) bool {
		if IsFileGlob(pattern) {
			return false
		}
		return MatchDirGlob(pattern, dirPath)
	})
}

func (g *Graph) NodeForPath(filePath string) string {
	if filePath == "" {
		filePath = "."
	}
	best := g.findMostSpecific(func(pattern string) bool {
		if !IsFileGlob(pattern) {
			return false
		}
		return MatchFileGlob(pattern, filePath)
	})
	if best != "" {
		return best
	}
	if IsFileGlob(filePath) {
		filePath = DirOf(filePath)
	}
	return g.NodeForDir(filePath)
}

func (g *Graph) findMostSpecific(match func(pattern string) bool) string {
	bestID := ""
	bestScore := -1
	for id, pattern := range g.Nodes {
		if !match(pattern) {
			continue
		}
		score := GlobSpecificity(pattern)
		if score > bestScore {
			bestID = id
			bestScore = score
		}
	}
	return bestID
}

func (g *Graph) Allows(sourceID, targetID string) bool {
	if sourceID == targetID {
		return true
	}
	return g.Edges[sourceID][targetID]
}

func (g *Graph) EdgeCount() int {
	count := 0
	for _, targets := range g.Edges {
		count += len(targets)
	}
	return count
}

func (g *Graph) FileGlobNodes() []string {
	var ids []string
	for id, pattern := range g.Nodes {
		if IsFileGlob(pattern) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// MatchDirGlob reports whether dirPath matches the directory glob pattern.
func MatchDirGlob(pattern, dirPath string) bool {
	if pattern == "." {
		return dirPath == "."
	}

	patternSegs := strings.Split(pattern, "/")
	lastPattern := patternSegs[len(patternSegs)-1]

	if lastPattern == "**" {
		prefix := patternSegs[:len(patternSegs)-1]
		if len(prefix) == 0 {
			return true
		}
		if dirPath == "." {
			return false
		}
		dirSegs := strings.Split(dirPath, "/")

		minLen := len(prefix)
		if strings.Contains(prefix[len(prefix)-1], "*") {
			minLen++
		}
		if len(dirSegs) < minLen {
			return false
		}

		for i, segPattern := range prefix {
			if !MatchSegment(segPattern, dirSegs[i]) {
				return false
			}
		}
		return true
	}

	if dirPath == "." {
		return false
	}

	dirSegs := strings.Split(dirPath, "/")
	if len(patternSegs) != len(dirSegs) {
		return false
	}
	for i, segPattern := range patternSegs {
		if !MatchSegment(segPattern, dirSegs[i]) {
			return false
		}
	}
	return true
}

// MatchSegment matches a single path segment against a pattern that may contain wildcards.
func MatchSegment(segPattern, segment string) bool {
	if !strings.Contains(segPattern, "*") {
		return segPattern == segment
	}
	parts := strings.Split(segPattern, "*")
	if parts[0] != "" && !strings.HasPrefix(segment, parts[0]) {
		return false
	}
	remaining := segment[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(remaining, parts[i])
		if idx < 0 {
			return false
		}
		remaining = remaining[idx+len(parts[i]):]
	}
	return strings.HasSuffix(remaining, parts[len(parts)-1])
}

// GlobSpecificity returns a score where higher means more specific.
func GlobSpecificity(pattern string) int {
	score := 0
	for _, segment := range strings.Split(pattern, "/") {
		switch {
		case segment == "**":
			score += 1
		case strings.Contains(segment, "*"):
			score += 3
		default:
			score += 10
		}
	}
	return score
}

// IsFileGlob reports whether the pattern refers to files (last segment contains a dot).
func IsFileGlob(pattern string) bool {
	if pattern == "" || pattern == "." {
		return false
	}
	parts := strings.Split(pattern, "/")
	last := parts[len(parts)-1]
	return last != "." && last != ".." && strings.Contains(last, ".")
}

// MatchFileGlob reports whether filePath matches the file glob pattern.
func MatchFileGlob(pattern, filePath string) bool {
	if filePath == "" || filePath == "." {
		return false
	}
	patternSegs := strings.Split(pattern, "/")
	pathSegs := strings.Split(filePath, "/")
	if len(patternSegs) != len(pathSegs) {
		return false
	}
	for i, segPattern := range patternSegs {
		if !MatchSegment(segPattern, pathSegs[i]) {
			return false
		}
	}
	return true
}

// DirOf returns the directory portion of a path, or "." if it has no directory.
func DirOf(path string) string {
	if !strings.Contains(path, "/") {
		return "."
	}
	return path[:strings.LastIndex(path, "/")]
}

// NodeKeyForDir returns the directory-level key for a path.
// Files are stripped to their parent directory.
func NodeKeyForDir(path string) string {
	if strings.Contains(path, ".") && strings.Contains(path, "/") {
		dir := filepath.Dir(filepath.FromSlash(path))
		return filepath.ToSlash(dir)
	}
	if strings.Contains(path, ".") {
		return "."
	}
	return path
}

// NodeKeyForFile returns the full path as the node key.
func NodeKeyForFile(path string) string {
	return path
}

func (g *Graph) Validate() []string {
	var errs []string
	for id, pattern := range g.Nodes {
		for _, msg := range ValidateNodeGlob(pattern) {
			errs = append(errs, fmt.Sprintf("node %q: %s", id, msg))
		}
	}
	sort.Strings(errs)
	return errs
}

func NewGraph(nodes map[string]string, edges map[string]map[string]bool) *Graph {
	g := &Graph{
		Nodes:     make(map[string]string, len(nodes)),
		Edges:     make(map[string]map[string]bool, len(edges)),
		Classes:   map[string]map[string]bool{},
		NodeLines: map[string]int{},
		EdgeLines: map[string]int{},
	}

	for id, glob := range nodes {
		g.Nodes[id] = glob
	}

	for src, dsts := range edges {
		g.Edges[src] = make(map[string]bool, len(dsts))
		for dst := range dsts {
			g.Edges[src][dst] = true
		}
	}

	return g
}

func ValidateNodeGlob(pattern string) []string {
	var msgs []string
	for _, seg := range strings.Split(pattern, "/") {
		if seg == ".." {
			msgs = append(msgs, `".." not allowed in node globs`)
		}
	}
	return msgs
}

// GlobsOverlap reports whether two directory globs can match any common path.
func GlobsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	if IsFileGlob(a) || IsFileGlob(b) {
		return false
	}

	aSegs := strings.Split(a, "/")
	bSegs := strings.Split(b, "/")

	aRecursive := len(aSegs) > 0 && aSegs[len(aSegs)-1] == "**"
	bRecursive := len(bSegs) > 0 && bSegs[len(bSegs)-1] == "**"

	if aRecursive && bRecursive {
		aPrefix := aSegs[:len(aSegs)-1]
		bPrefix := bSegs[:len(bSegs)-1]
		if len(aPrefix) == len(bPrefix) {
			return segmentsOverlap(aPrefix, bPrefix)
		}
		if len(aPrefix) < len(bPrefix) {
			return prefixMatchesPath(aPrefix, bPrefix)
		}
		return prefixMatchesPath(bPrefix, aPrefix)
	}

	if aRecursive {
		return prefixMatchesPath(aSegs[:len(aSegs)-1], bSegs)
	}
	if bRecursive {
		return prefixMatchesPath(bSegs[:len(bSegs)-1], aSegs)
	}

	if len(aSegs) != len(bSegs) {
		return false
	}
	return segmentsOverlap(aSegs, bSegs)
}

func segmentsOverlap(a, b []string) bool {
	for i := range a {
		if !pairCanOverlap(a[i], b[i]) {
			return false
		}
	}
	return true
}

func prefixMatchesPath(prefix, pathSegs []string) bool {
	if len(pathSegs) < len(prefix) {
		return false
	}
	for i := range prefix {
		if !pairCanOverlap(prefix[i], pathSegs[i]) {
			return false
		}
	}
	return true
}

func pairCanOverlap(a, b string) bool {
	if a == b {
		return true
	}
	aWild := strings.Contains(a, "*")
	bWild := strings.Contains(b, "*")
	if !aWild && !bWild {
		return false
	}
	if a == "*" || b == "*" || aWild && bWild {
		return true
	}

	wild, literal := a, b
	if bWild {
		wild, literal = b, a
	}

	parts := strings.Split(wild, "*")
	if parts[0] != "" && !strings.HasPrefix(literal, parts[0]) {
		return false
	}
	if parts[len(parts)-1] != "" && !strings.HasSuffix(literal, parts[len(parts)-1]) {
		return false
	}
	middle := literal[len(parts[0]) : len(literal)-len(parts[len(parts)-1])]
	for i := 1; i < len(parts)-1; i++ {
		if !strings.Contains(middle, parts[i]) {
			return false
		}
		middle = middle[strings.Index(middle, parts[i])+len(parts[i]):]
	}
	return true
}
