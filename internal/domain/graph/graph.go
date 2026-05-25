package graph

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// nodeInfo holds pre-computed data for a single graph node to avoid
// repeated string splitting during hot-path lookups.
type nodeInfo struct {
	pattern     string
	segments    []string
	hasWildcard bool
	isFileGlob  bool
	isDirGlob   bool
	specificity int
	hasDirGlob  bool
}

// Graph is the parsed contract from a contract file mermaid block.
// It is a pure domain model with no mutable cache state or concurrency primitives.
type Graph struct {
	Nodes        map[string]string
	NodeDisplays map[string]string
	Edges        map[string]map[string]bool
	Classes      map[string]map[string]bool
	NodeLines    map[string]int
	EdgeLines    map[string]int

	// NodeOrder is the declaration order of nodes as defined by the user
	// in the contract file. It serves as the tiebreaker when multiple
	// nodes have equal specificity for a given path. Serializers should
	// preserve this order, and new graph constructors must populate it.
	NodeOrder []string

	// EdgeOrder is the declaration order of edges as defined by the user
	// in the contract file. Each entry is "src\tdst". Serializers should
	// preserve this order, and new graph constructors must populate it.
	EdgeOrder []string

	// GlobSeparator, when non-empty, indicates the character used as path separator
	// in node glob patterns (e.g. "." for Kotlin/Java style). Patterns are normalized
	// to slash-based internally, but the original form is preserved in NodeDisplays.
	GlobSeparator string
}

// GraphIndex provides fast cached lookups over a Graph. It holds all mutable
// cache state and concurrency primitives, keeping the Graph domain entity pristine.
type GraphIndex struct {
	graph     *Graph
	edgeCount int
	nodeInfos map[string]*nodeInfo
	dirNodes  []string
	fileNodes []string
	dirCache  map[string]string
	fileCache map[string]string
	cacheMu   sync.RWMutex

	// onCompute is nil in production. Tests set it to verify the double-check
	// pattern prevents redundant computation under concurrent cache misses.
	// It is called only when a goroutine wins the double-check race and is
	// about to write the cached result — never for cache hits or losers.
	onCompute func()
}

func (gi *GraphIndex) Graph() *Graph {
	return gi.graph
}

// NewGraphIndex builds a GraphIndex for the given graph, pre-computing all
// node info and partitioning nodes into directory and file categories.
func NewGraphIndex(g *Graph) *GraphIndex {
	gi := &GraphIndex{
		graph:     g,
		nodeInfos: make(map[string]*nodeInfo, len(g.Nodes)),
	}

	gi.dirNodes = make([]string, 0, len(g.Nodes))
	gi.fileNodes = make([]string, 0, len(g.Nodes))
	gi.edgeCount = 0
	for _, targets := range g.Edges {
		gi.edgeCount += len(targets)
	}
	for id, pattern := range g.Nodes {
		normalized := pattern
		if g.GlobSeparator != "" {
			normalized = normalizeGlobSeparator(pattern, g.GlobSeparator)
		}
		ni := &nodeInfo{
			pattern:    normalized,
			segments:   splitPath(normalized),
			isFileGlob: isFileGlobFast(normalized),
		}
		ni.hasWildcard = hasWildcardInSegments(ni.segments)
		ni.isDirGlob = !ni.isFileGlob
		ni.hasDirGlob = !ni.isFileGlob
		ni.specificity = globSpecificityFast(ni.segments)
		gi.nodeInfos[id] = ni
		if ni.isFileGlob {
			gi.fileNodes = append(gi.fileNodes, id)
		} else {
			gi.dirNodes = append(gi.dirNodes, id)
		}
	}

	sort.Slice(gi.dirNodes, func(i, j int) bool {
		return nodeOrderIdx(g, gi.dirNodes[i]) < nodeOrderIdx(g, gi.dirNodes[j])
	})
	sort.Slice(gi.fileNodes, func(i, j int) bool {
		return nodeOrderIdx(g, gi.fileNodes[i]) < nodeOrderIdx(g, gi.fileNodes[j])
	})

	return gi
}

func (g *Graph) IsEndophobic(nodeID string) bool {
	return g.Classes[nodeID]["endophobic"]
}

// normalizeGlobSeparator replaces occurrences of the custom separator string with slashes.
// The separator is replaced only when preceded by a segment character and followed by a
// segment character or '*'. Standalone "." and ".." segments are preserved.
func normalizeGlobSeparator(pattern, sep string) string {
	if sep == "" {
		return pattern
	}
	var result strings.Builder
	result.Grow(len(pattern))
	i := 0
	for i < len(pattern) {
		if strings.HasPrefix(pattern[i:], sep) {
			beforeOk := i > 0 && isSegmentChar(pattern[i-1])
			afterIdx := i + len(sep)
			afterOk := afterIdx < len(pattern) && (isSegmentChar(pattern[afterIdx]) || pattern[afterIdx] == '*')
			if beforeOk && afterOk {
				result.WriteByte('/')
			} else {
				result.WriteString(sep)
			}
			i += len(sep)
		} else {
			result.WriteByte(pattern[i])
			i++
		}
	}
	return result.String()
}

func isSegmentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '-'
}

func (g *Graph) Allows(sourceID, targetID string) bool {
	if sourceID == targetID {
		return true
	}
	return g.Edges[sourceID][targetID]
}

// NodeForDir returns the node ID that best matches the given directory path.
func (gi *GraphIndex) NodeForDir(dirPath string) string {
	if dirPath == "" {
		dirPath = "."
	}
	gi.cacheMu.RLock()
	if gi.dirCache != nil {
		if cached, ok := gi.dirCache[dirPath]; ok {
			gi.cacheMu.RUnlock()
			return cached
		}
	}
	gi.cacheMu.RUnlock()
	result := gi.findMostSpecificDir(dirPath)
	gi.cacheMu.Lock()
	if gi.dirCache == nil {
		gi.dirCache = make(map[string]string, len(gi.graph.Nodes))
	}
	if cached, ok := gi.dirCache[dirPath]; ok {
		gi.cacheMu.Unlock()
		return cached
	}
	if gi.onCompute != nil {
		gi.onCompute()
	}
	gi.dirCache[dirPath] = result
	gi.cacheMu.Unlock()
	return result
}

// NodeForPath returns the node ID that best matches the given file path.
func (gi *GraphIndex) NodeForPath(filePath string) string {
	if filePath == "" {
		filePath = "."
	}
	gi.cacheMu.RLock()
	if gi.fileCache != nil {
		if cached, ok := gi.fileCache[filePath]; ok {
			gi.cacheMu.RUnlock()
			return cached
		}
	}
	gi.cacheMu.RUnlock()
	best := gi.findMostSpecificFile(filePath)
	if best != "" {
		gi.cacheMu.Lock()
		if gi.fileCache == nil {
			gi.fileCache = make(map[string]string, len(gi.graph.Nodes))
		}
		if cached, ok := gi.fileCache[filePath]; ok {
			gi.cacheMu.Unlock()
			return cached
		}
		if gi.onCompute != nil {
			gi.onCompute()
		}
		gi.fileCache[filePath] = best
		gi.cacheMu.Unlock()
		return best
	}
	if isFileGlobFast(filePath) {
		filePath = DirOf(filePath)
	}
	result := gi.NodeForDir(filePath)
	gi.cacheMu.Lock()
	if gi.fileCache == nil {
		gi.fileCache = make(map[string]string, len(gi.graph.Nodes))
	}
	if cached, ok := gi.fileCache[filePath]; ok {
		gi.cacheMu.Unlock()
		return cached
	}
	gi.fileCache[filePath] = result
	gi.cacheMu.Unlock()
	return result
}

func (gi *GraphIndex) EdgeCount() int {
	return gi.edgeCount
}

func (gi *GraphIndex) FileGlobNodes() []string {
	ids := make([]string, len(gi.fileNodes))
	copy(ids, gi.fileNodes)
	sort.Strings(ids)
	return ids
}

func (gi *GraphIndex) findMostSpecificDir(dirPath string) string {
	bestID := ""
	bestScore := -1
	dirSegs := splitPath(dirPath)
	for _, id := range gi.dirNodes {
		ni := gi.nodeInfos[id]
		if matchDirGlobSegments(ni.segments, ni.hasWildcard, dirSegs) {
			if ni.specificity > bestScore {
				bestID = id
				bestScore = ni.specificity
			}
		}
	}
	return bestID
}

func (gi *GraphIndex) findMostSpecificFile(filePath string) string {
	bestID := ""
	bestScore := -1
	pathSegs := splitPath(filePath)
	for _, id := range gi.fileNodes {
		ni := gi.nodeInfos[id]
		if matchFileGlobSegments(ni.segments, pathSegs) {
			if ni.specificity > bestScore {
				bestID = id
				bestScore = ni.specificity
			}
		}
	}
	return bestID
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

func matchSegmentFast(segPattern, segment string) bool {
	if !stringsContainsByte(segPattern, '*') {
		return segPattern == segment
	}
	p := segPattern
	s := segment

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
		// Find end of this middle part between wildcards.
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

	// Check suffix after last wildcard against remaining string.
	suffixLen := len(p) - lastStar - 1
	if suffixLen > 0 {
		return strHasSuffix(remaining, p[len(p)-suffixLen:])
	}
	return true
}

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

func isFileGlobFast(pattern string) bool {
	if pattern == "" || pattern == "." {
		return false
	}
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

func matchFileGlobSegments(patternSegs, pathSegs []string) bool {
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

// NormalizeGlobs converts custom glob separators (e.g., "." for Kotlin/Java)
// to slash-based patterns. It should be called once after parsing, so that
// all consumers of Graph.Nodes see canonical slash-based globs.
func (g *Graph) NormalizeGlobs() {
	if g.GlobSeparator == "" {
		return
	}
	for id, pattern := range g.Nodes {
		g.Nodes[id] = normalizeGlobSeparator(pattern, g.GlobSeparator)
	}
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

func splitPath(p string) []string {
	if p == "" || p == "." {
		return nil
	}
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

func hasWildcardInSegments(segments []string) bool {
	for _, s := range segments {
		if stringsContainsByte(s, '*') {
			return true
		}
	}
	return false
}

func nodeOrderIdx(g *Graph, id string) int {
	if len(g.NodeOrder) == 0 {
		return g.NodeLines[id]
	}
	for i, nid := range g.NodeOrder {
		if nid == id {
			return i
		}
	}
	return len(g.NodeOrder)
}

func NewGraph(nodes map[string]string, edges map[string]map[string]bool, nodeOrder, edgeOrder []string) *Graph {
	g := &Graph{
		Nodes:        make(map[string]string, len(nodes)),
		NodeDisplays: map[string]string{},
		Edges:        make(map[string]map[string]bool, len(edges)),
		Classes:      map[string]map[string]bool{},
		NodeLines:    map[string]int{},
		EdgeLines:    map[string]int{},
		NodeOrder:    make([]string, 0, len(nodes)),
		EdgeOrder:    make([]string, 0, len(edges)),
	}

	for id, glob := range nodes {
		g.Nodes[id] = glob
	}

	if len(nodeOrder) == len(nodes) {
		g.NodeOrder = make([]string, len(nodeOrder))
		copy(g.NodeOrder, nodeOrder)
	} else {
		for id := range nodes {
			g.NodeOrder = append(g.NodeOrder, id)
		}
		sort.Strings(g.NodeOrder)
	}

	for src, dsts := range edges {
		g.Edges[src] = make(map[string]bool, len(dsts))
		for dst := range dsts {
			g.Edges[src][dst] = true
		}
	}

	if len(edgeOrder) == lenEdgeCount(edges) {
		g.EdgeOrder = make([]string, len(edgeOrder))
		copy(g.EdgeOrder, edgeOrder)
	} else {
		for src, dsts := range edges {
			for dst := range dsts {
				g.EdgeOrder = append(g.EdgeOrder, src+"\t"+dst)
			}
		}
		sort.Strings(g.EdgeOrder)
	}

	return g
}

func lenEdgeCount(edges map[string]map[string]bool) int {
	n := 0
	for _, dsts := range edges {
		n += len(dsts)
	}
	return n
}
