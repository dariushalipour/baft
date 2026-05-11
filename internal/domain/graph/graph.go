package graph

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// nodeInfo holds pre-computed data for a single graph node to avoid
// repeated string splitting during hot-path lookups.
type nodeInfo struct {
	pattern     string
	segments    []string // pre-split pattern segments
	hasWildcard bool     // segment contains '*'
	isFileGlob  bool
	isDirGlob   bool
	specificity int
	hasDirGlob  bool // node has at least one directory-glob pattern
}

// Graph is the parsed contract from a BAFT.md mermaid block.
type Graph struct {
	Nodes     map[string]string
	Edges     map[string]map[string]bool
	Classes   map[string]map[string]bool
	NodeLines map[string]int
	EdgeLines map[string]int

	// nodeInfos holds pre-computed info per node ID for fast matching.
	nodeInfos map[string]*nodeInfo
	// nodeInfosOnce ensures buildNodeInfos runs exactly once.
	nodeInfosOnce sync.Once
	// dirCache caches NodeForDir results for O(1) repeated lookups.
	dirCache map[string]string
	// fileCache caches NodeForPath results for file glob lookups.
	fileCache map[string]string
	// cacheMu protects dirCache and fileCache writes.
	cacheMu sync.RWMutex
}

func (g *Graph) IsEndophobic(nodeID string) bool {
	return g.Classes[nodeID]["endophobic"]
}

// ensureNodeInfos lazily builds nodeInfos if not already done.
func (g *Graph) ensureNodeInfos() {
	g.nodeInfosOnce.Do(g.buildNodeInfos)
}

// buildNodeInfos pre-comutes nodeInfo for every node in the graph.
// Must be called after Nodes and Edges are populated.
func (g *Graph) buildNodeInfos() {
	if g.nodeInfos == nil {
		g.nodeInfos = make(map[string]*nodeInfo, len(g.Nodes))
	}
	for id, pattern := range g.Nodes {
		ni := &nodeInfo{
			pattern:    pattern,
			segments:   strings.Split(pattern, "/"),
			isFileGlob: isFileGlobFast(pattern),
		}
		ni.hasWildcard = hasWildcardInSegments(ni.segments)
		ni.isDirGlob = !ni.isFileGlob
		ni.hasDirGlob = !ni.isFileGlob
		ni.specificity = globSpecificityFast(ni.segments)
		g.nodeInfos[id] = ni
	}
}

func (g *Graph) NodeForDir(dirPath string) string {
	if dirPath == "" {
		dirPath = "."
	}
	g.cacheMu.RLock()
	if g.dirCache != nil {
		if cached, ok := g.dirCache[dirPath]; ok {
			g.cacheMu.RUnlock()
			return cached
		}
	}
	g.cacheMu.RUnlock()
	g.ensureNodeInfos()
	result := g.findMostSpecificDir(dirPath)
	g.cacheMu.Lock()
	if g.dirCache == nil {
		g.dirCache = make(map[string]string, len(g.Nodes))
	}
	g.dirCache[dirPath] = result
	g.cacheMu.Unlock()
	return result
}

func (g *Graph) NodeForPath(filePath string) string {
	if filePath == "" {
		filePath = "."
	}
	g.cacheMu.RLock()
	if g.fileCache != nil {
		if cached, ok := g.fileCache[filePath]; ok {
			g.cacheMu.RUnlock()
			return cached
		}
	}
	g.cacheMu.RUnlock()
	g.ensureNodeInfos()
	best := g.findMostSpecificFile(filePath)
	if best != "" {
		g.cacheMu.Lock()
		if g.fileCache == nil {
			g.fileCache = make(map[string]string, len(g.Nodes))
		}
		g.fileCache[filePath] = best
		g.cacheMu.Unlock()
		return best
	}
	if isFileGlobFast(filePath) {
		filePath = DirOf(filePath)
	}
	result := g.NodeForDir(filePath)
	g.cacheMu.Lock()
	if g.fileCache == nil {
		g.fileCache = make(map[string]string, len(g.Nodes))
	}
	g.fileCache[filePath] = result
	g.cacheMu.Unlock()
	return result
}

func (g *Graph) findMostSpecificDir(dirPath string) string {
	bestID := ""
	bestScore := -1
	dirSegs := splitPath(dirPath)
	for id, ni := range g.nodeInfos {
		if ni.isFileGlob {
			continue
		}
		if matchDirGlobSegments(ni.segments, ni.hasWildcard, dirSegs) {
			if ni.specificity > bestScore {
				bestID = id
				bestScore = ni.specificity
			}
		}
	}
	return bestID
}

func (g *Graph) findMostSpecificFile(filePath string) string {
	bestID := ""
	bestScore := -1
	pathSegs := splitPath(filePath)
	for id, ni := range g.nodeInfos {
		if !ni.isFileGlob {
			continue
		}
		if matchFileGlobSegments(ni.segments, ni.hasWildcard, pathSegs) {
			if ni.specificity > bestScore {
				bestID = id
				bestScore = ni.specificity
			}
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
	seg := strings.Split(pattern, "/")
	hasW := hasWildcardInSegments(seg)
	dirSegs := splitPath(dirPath)
	return matchDirGlobSegments(seg, hasW, dirSegs)
}

// matchDirGlobSegments matches pre-split segments against a dir path.
func matchDirGlobSegments(patternSegs []string, patternHasWildcard bool, dirSegs []string) bool {
	if len(patternSegs) == 1 && patternSegs[0] == "." {
		return len(dirSegs) == 0
	}

	lastPattern := patternSegs[len(patternSegs)-1]
	if lastPattern == "**" {
		prefix := patternSegs[:len(patternSegs)-1]
		if len(prefix) == 0 {
			return true
		}
		if len(dirSegs) == 0 {
			return false
		}
		minLen := len(prefix)
		if patternHasWildcard && len(prefix) > 0 {
			if strings.Contains(prefix[len(prefix)-1], "*") {
				minLen++
			}
		}
		if len(dirSegs) < minLen {
			return false
		}
		for i, sp := range prefix {
			if !matchSegmentFast(sp, dirSegs[i]) {
				return false
			}
		}
		return true
	}

	if len(dirSegs) == 0 {
		return false
	}
	if len(patternSegs) != len(dirSegs) {
		return false
	}
	for i, sp := range patternSegs {
		if !matchSegmentFast(sp, dirSegs[i]) {
			return false
		}
	}
	return true
}

// MatchSegment matches a single path segment against a pattern that may contain wildcards.
func MatchSegment(segPattern, segment string) bool {
	return matchSegmentFast(segPattern, segment)
}

// matchSegmentFast is the internal fast path for segment matching.
func matchSegmentFast(segPattern, segment string) bool {
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
	seg := strings.Split(pattern, "/")
	return globSpecificityFast(seg)
}

// globSpecificityFast computes specificity from pre-split segments.
func globSpecificityFast(segments []string) int {
	score := 0
	for _, segment := range segments {
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
	return isFileGlobFast(pattern)
}

// isFileGlobFast checks if a pattern is a file glob without splitting the full path.
func isFileGlobFast(pattern string) bool {
	if pattern == "" || pattern == "." {
		return false
	}
	// Find last '/' efficiently.
	lastSlash := -1
	for i := len(pattern) - 1; i >= 0; i-- {
		if pattern[i] == '/' {
			lastSlash = i
			break
		}
	}
	var last string
	if lastSlash >= 0 {
		last = pattern[lastSlash+1:]
	} else {
		last = pattern
	}
	if last == "." || last == ".." {
		return false
	}
	// Check for dot in last segment.
	for i := 0; i < len(last); i++ {
		if last[i] == '.' {
			return true
		}
	}
	return false
}

// MatchFileGlob reports whether filePath matches the file glob pattern.
func MatchFileGlob(pattern, filePath string) bool {
	if filePath == "" || filePath == "." {
		return false
	}
	patternSegs := strings.Split(pattern, "/")
	pathSegs := splitPath(filePath)
	if len(patternSegs) != len(pathSegs) {
		return false
	}
	for i, sp := range patternSegs {
		if !matchSegmentFast(sp, pathSegs[i]) {
			return false
		}
	}
	return true
}

// matchFileGlobSegments matches pre-split segments for file globs.
func matchFileGlobSegments(patternSegs []string, patternHasWildcard bool, pathSegs []string) bool {
	if len(pathSegs) == 0 {
		return false
	}
	if len(patternSegs) != len(pathSegs) {
		return false
	}
	for i, sp := range patternSegs {
		if !matchSegmentFast(sp, pathSegs[i]) {
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

func ValidateNodeGlob(pattern string) []string {
	var msgs []string
	for _, seg := range strings.Split(pattern, "/") {
		if strings.HasPrefix(seg, "..") {
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
	if isFileGlobFast(a) || isFileGlobFast(b) {
		return false
	}

	aSegs := splitPath(a)
	bSegs := splitPath(b)

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

// splitPath splits a path into segments efficiently using byte operations.
func splitPath(p string) []string {
	if p == "" || p == "." {
		return nil
	}
	// Count segments.
	n := 1
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			n++
		}
	}
	segs := make([]string, 0, n)
	start := 0
	for i := 0; i <= len(p); i++ {
		if i == len(p) || p[i] == '/' {
			segs = append(segs, p[start:i])
			start = i + 1
		}
	}
	return segs
}

// hasWildcardInSegments checks if any segment contains '*'.
func hasWildcardInSegments(segments []string) bool {
	for _, s := range segments {
		if strings.Contains(s, "*") {
			return true
		}
	}
	return false
}

// NewGraph creates a new Graph and pre-computes node info.
func NewGraph(nodes map[string]string, edges map[string]map[string]bool) *Graph {
	g := &Graph{
		Nodes:     make(map[string]string, len(nodes)),
		Edges:     make(map[string]map[string]bool, len(edges)),
		Classes:   map[string]map[string]bool{},
		NodeLines: map[string]int{},
		EdgeLines: map[string]int{},
		dirCache:  make(map[string]string, len(nodes)),
		fileCache: make(map[string]string, len(nodes)),
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

	g.buildNodeInfos()

	return g
}
