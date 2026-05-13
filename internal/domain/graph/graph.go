package graph

import (
	"path/filepath"
	"sort"
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

	// edgeCount caches the total edge count to avoid O(n) iteration.
	edgeCount int

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
	// dirNodes and fileNodes hold pre-partitioned node IDs for fast iteration.
	dirNodes  []string
	fileNodes []string
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
	// Pre-allocate partition slices to avoid repeated resizing.
	g.dirNodes = make([]string, 0, len(g.Nodes))
	g.fileNodes = make([]string, 0, len(g.Nodes))
	// Pre-compute edge count.
	g.edgeCount = 0
	for _, targets := range g.Edges {
		g.edgeCount += len(targets)
	}
	for id, pattern := range g.Nodes {
		ni := &nodeInfo{
			pattern:    pattern,
			segments:   splitPath(pattern),
			isFileGlob: isFileGlobFast(pattern),
		}
		ni.hasWildcard = hasWildcardInSegments(ni.segments)
		ni.isDirGlob = !ni.isFileGlob
		ni.hasDirGlob = !ni.isFileGlob
		ni.specificity = globSpecificityFast(ni.segments)
		g.nodeInfos[id] = ni
		if ni.isFileGlob {
			g.fileNodes = append(g.fileNodes, id)
		} else {
			g.dirNodes = append(g.dirNodes, id)
		}
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
	for _, id := range g.dirNodes {
		ni := g.nodeInfos[id]
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
	for _, id := range g.fileNodes {
		ni := g.nodeInfos[id]
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
	return g.edgeCount
}

func (g *Graph) FileGlobNodes() []string {
	g.ensureNodeInfos()
	ids := make([]string, len(g.fileNodes))
	copy(ids, g.fileNodes)
	sort.Strings(ids)
	return ids
}

// MatchDirGlob reports whether dirPath matches the directory glob pattern.
func MatchDirGlob(pattern, dirPath string) bool {
	if pattern == "." {
		return dirPath == "."
	}
	seg := splitPath(pattern)
	hasW := hasWildcardInSegments(seg)
	dirSegs := splitPath(dirPath)
	return matchDirGlobSegments(seg, hasW, dirSegs)
}

// matchDirGlobSegments matches pre-split segments against a dir path.
func matchDirGlobSegments(patternSegs []string, patternHasWildcard bool, dirSegs []string) bool {
	if len(patternSegs) == 0 {
		return len(dirSegs) == 0
	}
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
			if stringsContainsByte(prefix[len(prefix)-1], '*') {
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
// Uses byte-level operations to avoid string allocations.
func matchSegmentFast(segPattern, segment string) bool {
	if !stringsContainsByte(segPattern, '*') {
		return segPattern == segment
	}
	p := segPattern
	s := segment

	// Find first and last '*' positions.
	firstStar := -1
	lastStar := -1
	for i := 0; i < len(p); i++ {
		if p[i] == '*' {
			if firstStar == -1 {
				firstStar = i
			}
			lastStar = i
		}
	}

	// Single wildcard: simple prefix + suffix check.
	if firstStar == lastStar && firstStar != -1 {
		if firstStar > 0 && !strHasPrefix(s, p[:firstStar]) {
			return false
		}
		suffixLen := len(p) - lastStar - 1
		if suffixLen > 0 {
			if len(s) < firstStar+suffixLen {
				return false
			}
			return strHasSuffix(s, p[len(p)-suffixLen:])
		}
		return true
	}

	// Multiple wildcards: check prefix, middle parts, and suffix.
	if firstStar > 0 && !strHasPrefix(s, p[:firstStar]) {
		return false
	}
	remaining := s[firstStar+1:]

	// Extract middle parts (between wildcards) and check each.
	for i := firstStar + 1; i <= lastStar; {
		// Skip '*'
		if p[i] == '*' {
			i++
			continue
		}
		// Find end of this middle part.
		end := i
		for end <= lastStar && p[end] != '*' {
			end++
		}
		if end > i {
			middle := p[i:end]
			idx := strIndex(remaining, middle)
			if idx < 0 {
				return false
			}
			remaining = remaining[idx+len(middle):]
		}
		i = end + 1
	}

	// Check suffix after last wildcard.
	suffixLen := len(p) - lastStar - 1
	if suffixLen > 0 {
		return strHasSuffix(remaining, p[len(p)-suffixLen:])
	}
	return true
}

// strHasPrefix checks if s starts with prefix using byte-level comparison.
func strHasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if s[i] != prefix[i] {
			return false
		}
	}
	return true
}

// strHasSuffix checks if s ends with suffix using byte-level comparison.
func strHasSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	sStart := len(s) - len(suffix)
	for i := 0; i < len(suffix); i++ {
		if s[sStart+i] != suffix[i] {
			return false
		}
	}
	return true
}

// strIndex finds the first occurrence of sep in s, returns -1 if not found.
func strIndex(s, sep string) int {
	if len(sep) == 0 {
		return 0
	}
	if len(sep) > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i] == sep[0] {
			match := true
			for j := 1; j < len(sep); j++ {
				if s[i+j] != sep[j] {
					match = false
					break
				}
			}
			if match {
				return i
			}
		}
	}
	return -1
}

// stringsContainsByte checks if s contains the given byte.
func stringsContainsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

// GlobSpecificity returns a score where higher means more specific.
func GlobSpecificity(pattern string) int {
	if pattern == "." {
		return 10
	}
	return globSpecificityFast(splitPath(pattern))
}

// globSpecificityFast computes specificity from pre-split segments.
func globSpecificityFast(segments []string) int {
	score := 0
	for _, segment := range segments {
		switch {
		case segment == "**":
			score += 1
		case stringsContainsByte(segment, '*'):
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
	patternSegs := splitPath(pattern)
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
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

// NodeKeyForDir returns the directory-level key for a path.
// Files are stripped to their parent directory.
func NodeKeyForDir(path string) string {
	hasDot := false
	hasSlash := false
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			hasDot = true
		}
		if path[i] == '/' {
			hasSlash = true
		}
	}
	if hasDot && hasSlash {
		dir := filepath.Dir(filepath.FromSlash(path))
		return filepath.ToSlash(dir)
	}
	if hasDot {
		return "."
	}
	return path
}

// NodeKeyForFile returns the full path as the node key.
func NodeKeyForFile(path string) string {
	return path
}

func (g *Graph) Validate() []string {
	errs := make([]string, 0, len(g.Nodes)*2)
	for id, pattern := range g.Nodes {
		for _, msg := range ValidateNodeGlob(pattern) {
			errs = append(errs, "node "+id+": "+msg)
		}
	}
	sort.Strings(errs)
	return errs
}

func ValidateNodeGlob(pattern string) []string {
	var msgs []string
	segs := splitPath(pattern)
	for _, seg := range segs {
		if len(seg) >= 2 && seg[0] == '.' && seg[1] == '.' {
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
	aWild := stringsContainsByte(a, '*')
	bWild := stringsContainsByte(b, '*')
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

	// Find first and last '*' positions.
	firstStar := -1
	lastStar := -1
	for i := 0; i < len(wild); i++ {
		if wild[i] == '*' {
			if firstStar == -1 {
				firstStar = i
			}
			lastStar = i
		}
	}

	// Check prefix.
	if firstStar > 0 && !strHasPrefix(literal, wild[:firstStar]) {
		return false
	}
	// Check suffix.
	suffixLen := len(wild) - lastStar - 1
	if suffixLen > 0 {
		if len(literal) < firstStar+suffixLen {
			return false
		}
		if !strHasSuffix(literal, wild[len(wild)-suffixLen:]) {
			return false
		}
	}

	// Check middle parts.
	if lastStar-firstStar > 1 {
		remaining := literal[firstStar+1 : len(literal)-suffixLen]
		for i := firstStar + 1; i <= lastStar; {
			if wild[i] == '*' {
				i++
				continue
			}
			end := i
			for end <= lastStar && wild[end] != '*' {
				end++
			}
			if end > i {
				middle := wild[i:end]
				idx := strIndex(remaining, middle)
				if idx < 0 {
					return false
				}
				remaining = remaining[idx+len(middle):]
			}
			i = end + 1
		}
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
		if stringsContainsByte(s, '*') {
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
