package mermaid

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

var (
	nodeRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\[(?:"([^"]*)"|([^\]]*))\](?::::([A-Za-z_][A-Za-z0-9_,]*))?$`)

	nodeIdReplacer = strings.NewReplacer(
		"/", "_slash_",
		".", "_dot_",
		"-", "_dash_",
		"*", "_asterisk_",
		"@", "_atsign_",
		"[", "_lsqb_",
		"]", "_rsqb_",
		"{", "_lbrace_",
		"}", "_rbrace_",
		"+", "_plus_",
		"?", "_qmark_",
		",", "_comma_",
		" ", "_space_",
		"\t", "_tab_",
		"\n", "_newline_",
		"\r", "_carriage_return_",
		"\x0b", "_vertical_tab_",
		"\x0c", "_form_feed_",
	)

	// arrowRe matches every directed link Baft accepts: solid, thick and dotted,
	// each optionally carrying an inline `-- text -->` label. Only the direction
	// and, for dotted links, the toleration it declares are meaningful; the
	// variant and the label are otherwise discarded.
	arrowRe = regexp.MustCompile(`-{2,}(?:\s[^-=>|\n]*\s-{2,})?>|={2,}(?:\s[^-=>|\n]*\s={2,})?>|-\.+(?:\s[^-=>|\n]*\s\.+)?-*>`)

	globDecodeReplacer = strings.NewReplacer(
		"&ast;", "*",
		"&#42;", "*",
	)
)

var generatedStyleComment = strings.Trim(`
  %% ------------------------------------------------------------------------------------------
  %% AUTO-GENERATED STYLING: Do not edit manually.
  %% If you add, delete, or reorder nodes, you MUST run 'baft restyle' or format via your IDE.
  %% Outdated references will either break the render entirely or silently mess up the styling.
  %% ------------------------------------------------------------------------------------------
`, "\n")

var generatedStyleCommentLines = func() map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(generatedStyleComment, "\n") {
		set[strings.TrimSpace(line)] = true
	}
	return set
}()

type MermaidRepository struct{}

// ParseError reports a contract line the parser could not make sense of. File
// is empty when the parser is handed bare content; callers that know which
// contract it came from set it so the message reads "… (file:line)".
type ParseError struct {
	File string
	Line int
	Raw  string
	Msg  string
}

func (e *ParseError) Error() string {
	msg := e.Msg
	if msg == "" {
		msg = "unrecognized mermaid line: " + strings.TrimSpace(e.Raw)
	}
	switch {
	case e.File != "" && e.Line > 0:
		return fmt.Sprintf("%s (%s:%d)", msg, e.File, e.Line)
	case e.File != "":
		return fmt.Sprintf("%s (%s)", msg, e.File)
	case e.Line > 0:
		return fmt.Sprintf("%s (line %d)", msg, e.Line)
	}
	return msg
}

// At returns err with the contract path folded into its message.
func At(file string, err error) error {
	var pe *ParseError
	if !errors.As(err, &pe) {
		return err
	}
	located := *pe
	located.File = file
	return &located
}

type graphEdge struct {
	src    string
	dst    string
	dotted bool
}

var paletteColors = map[port.GraphColorPalette][]string{
	port.ColorPaletteVibrant: {
		"#0f4cde", "#c43d18", "#007e5f", "#8f2bd1",
		"#8b5e00", "#005bb5", "#9e1f63", "#2e6f00",
		"#6f3fd6", "#8a2d00", "#008299", "#b11f2f",
		"#5e6f00", "#6a2fb3", "#006e38", "#7d3b00",
	},
	port.ColorPaletteMuted: {
		"#5a6f8f", "#9a6b5c", "#62816e", "#7a6e93",
		"#8f7a5b", "#5f7895", "#94687c", "#6e7f5f",
		"#697aa2", "#8c6651", "#5f847f", "#94645f",
		"#77745b", "#6d6790", "#5d7b69", "#8a7058",
	},
	port.ColorPaletteMono: {
		"#1f1f1f", "#2a2a2a", "#353535", "#404040",
		"#4b4b4b", "#565656", "#616161", "#6c6c6c",
		"#777777", "#828282", "#8d8d8d", "#989898",
		"#a3a3a3", "#aeaeae", "#b9b9b9", "#c4c4c4",
	},
}

func (r *MermaidRepository) Save(g *graph.Graph, opts port.GraphSaveOptions) string {
	var sb strings.Builder

	sb.WriteString("<!-- 🧶 Baft architecture contract: edit nodes and edges to change allowed imports. -->\n")
	sb.WriteString("<!-- If Baft is new to you, run `baft manual`. -->\n")
	sb.WriteString("<!-- Nodes claim file globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->\n")
	sb.WriteString("<!-- Validate with `baft check`. Refresh generated styling with `baft restyle`. -->\n")
	sb.WriteString("\n")
	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart TD\n")

	ids := orderedNodeIds(g)

	if g.GlobSeparator != "" {
		sb.WriteString("  %% config globSeparator ")
		sb.WriteString(fmt.Sprintf("%q", g.GlobSeparator))
		sb.WriteString("\n")
	}
	if g.NamespaceMode {
		sb.WriteString("  %% config namespaceMode \"true\"\n")
	}
	if g.GlobSeparator != "" || g.NamespaceMode {
		sb.WriteString("\n")
	}

	for _, id := range ids {
		glob := g.Nodes[id]
		display := glob
		if g.GlobSeparator != "" {
			display = strings.ReplaceAll(glob, "/", g.GlobSeparator)
		} else if preserved, ok := g.NodeDisplays[id]; ok {
			display = preserved
		} else if glob != "." && !looksLikeFilePath(glob) && !strings.HasSuffix(glob, "/**") {
			display = glob + "/**"
		}
		sb.WriteString("  ")
		sb.WriteString(encodeNodeId(id))
		sb.WriteString("[")
		sb.WriteString(quotedEncode(display))
		sb.WriteString("]")
		if classes := sortedNodeClasses(g.Classes[id]); len(classes) > 0 {
			sb.WriteString(":::")
			sb.WriteString(strings.Join(classes, ","))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	for _, edge := range orderedEdges(g) {
		sb.WriteString("  ")
		sb.WriteString(encodeNodeId(edge.src))
		sb.WriteString(arrowFor(edge.dotted))
		sb.WriteString(encodeNodeId(edge.dst))
		sb.WriteByte('\n')
	}

	for _, line := range styleBlock(g, opts) {
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	sb.WriteString("```\n")
	return sb.String()
}

// Restyle rewrites only the generated styling tail of the mermaid block, leaving
// every other byte of the contract — prose, direction, comments, nodes, edges — as written.
func (r *MermaidRepository) Restyle(md string, opts port.GraphSaveOptions) (string, error) {
	g, err := r.Load(md)
	if err != nil {
		return "", err
	}
	block, blockStartLine, err := extractMermaidBlock(md)
	if err != nil {
		return "", err
	}

	lines := strings.Split(md, "\n")
	first := blockStartLine - 1
	last := first + strings.Count(block, "\n")

	out := make([]string, 0, len(lines))
	out = append(out, lines[:first]...)
	out = append(out, spliceStyleTail(lines[first:last], g, opts)...)
	out = append(out, lines[last:]...)
	return strings.Join(out, "\n"), nil
}

func spliceStyleTail(blockLines []string, g *graph.Graph, opts port.GraphSaveOptions) []string {
	body := make([]string, 0, len(blockLines))
	for _, line := range blockLines {
		if !isGeneratedStyleLine(line) {
			body = append(body, line)
		}
	}
	tail := styleBlock(g, opts)
	if len(body) == len(blockLines) && len(tail) == 0 {
		return body
	}
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	return append(body, tail...)
}

func isGeneratedStyleLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return isStyleLine(trimmed) || generatedStyleCommentLines[trimmed]
}

func isStyleLine(line string) bool {
	return strings.HasPrefix(line, "classDef ") || strings.HasPrefix(line, "style ") || strings.HasPrefix(line, "linkStyle ")
}

func orderedNodeIds(g *graph.Graph) []string {
	if len(g.NodeOrder) == len(g.Nodes) {
		return g.NodeOrder
	}
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func orderedEdges(g *graph.Graph) []graphEdge {
	count := 0
	for _, targets := range g.Edges {
		count += len(targets)
	}
	edges := make([]graphEdge, 0, count)
	if len(g.EdgeOrder) == count {
		for _, key := range g.EdgeOrder {
			if parts := strings.Split(key, "\t"); len(parts) == 2 {
				edges = append(edges, graphEdge{src: parts[0], dst: parts[1], dotted: g.Tolerated[key]})
			}
		}
		return edges
	}
	sources := make([]string, 0, len(g.Edges))
	for src := range g.Edges {
		sources = append(sources, src)
	}
	sort.Strings(sources)
	for _, src := range sources {
		targets := make([]string, 0, len(g.Edges[src]))
		for dst := range g.Edges[src] {
			targets = append(targets, dst)
		}
		sort.Strings(targets)
		for _, dst := range targets {
			edges = append(edges, graphEdge{src: src, dst: dst, dotted: g.IsTolerated(src, dst)})
		}
	}
	return edges
}

// styleBlock renders the generated styling tail: a blank separator, the notice, then the style lines.
func styleBlock(g *graph.Graph, opts port.GraphSaveOptions) []string {
	styleLines := buildStyleLines(g, opts)
	if len(styleLines) == 0 {
		return nil
	}
	block := append([]string{""}, strings.Split(generatedStyleComment, "\n")...)
	for _, line := range styleLines {
		block = append(block, "  "+line)
	}
	return block
}

func buildStyleLines(g *graph.Graph, opts port.GraphSaveOptions) []string {
	ids, edges := orderedNodeIds(g), orderedEdges(g)
	palette := normalizedPalette(opts.ColorPalette)
	nodeColors := map[string]string{}
	if palette != port.ColorPaletteNone {
		colors := paletteColors[palette]
		for i, id := range ids {
			nodeColors[id] = colors[i%len(colors)]
		}
	}

	lines := make([]string, 0, len(ids)+len(edges))
	for _, id := range ids {
		attrs := nodeStyleAttributes(nodeColors[id], g.IsEndophobic(id))
		if attrs == "" {
			continue
		}
		lines = append(lines, "style "+encodeNodeId(id)+" "+attrs)
	}
	linkStyleOrder := make([]string, 0)
	linkStyleGroups := map[string][]string{}
	for i, edge := range edges {
		color := nodeColors[edge.src]
		if color == "" {
			continue
		}
		attrs := fmt.Sprintf("stroke:%s,stroke-width:2px", color)
		if edge.dotted {
			attrs += ",stroke-dasharray:5 5"
		}
		if _, ok := linkStyleGroups[attrs]; !ok {
			linkStyleOrder = append(linkStyleOrder, attrs)
		}
		linkStyleGroups[attrs] = append(linkStyleGroups[attrs], fmt.Sprintf("%d", i))
	}
	for _, attrs := range linkStyleOrder {
		lines = append(lines, "linkStyle "+strings.Join(linkStyleGroups[attrs], ",")+" "+attrs)
	}
	return lines
}

func normalizedPalette(palette port.GraphColorPalette) port.GraphColorPalette {
	switch palette {
	case "", port.ColorPaletteVibrant:
		return port.ColorPaletteVibrant
	case port.ColorPaletteMuted:
		return port.ColorPaletteMuted
	case port.ColorPaletteMono:
		return port.ColorPaletteMono
	case port.ColorPaletteNone:
		return port.ColorPaletteNone
	default:
		return port.ColorPaletteVibrant
	}
}

func nodeStyleAttributes(color string, endophobic bool) string {
	attrs := make([]string, 0, 3)
	if color != "" {
		attrs = append(attrs, "stroke:"+color, "stroke-width:2px")
	}
	if endophobic {
		if color == "" {
			attrs = append(attrs, "stroke-width:2px")
		}
		attrs = append(attrs, "stroke-dasharray:5 5")
	}
	return strings.Join(attrs, ",")
}

func sortedNodeClasses(classes map[string]bool) []string {
	if len(classes) == 0 {
		return nil
	}
	names := make([]string, 0, len(classes))
	for name, enabled := range classes {
		if enabled {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return names
}

func (r *MermaidRepository) Load(md string) (*graph.Graph, error) {
	block, blockStartLine, err := extractMermaidBlock(md)
	if err != nil {
		return nil, err
	}

	g := &graph.Graph{
		Nodes:        map[string]string{},
		NodeDisplays: map[string]string{},
		Edges:        map[string]map[string]bool{},
		Classes:      map[string]map[string]bool{},
		NodeLines:    map[string]int{},
		EdgeLines:    map[string]int{},
		Tolerated:    map[string]bool{},
		NodeOrder:    []string{},
		EdgeOrder:    []string{},
	}

	lines := strings.Split(block, "\n")

	lineNum := 0
	for _, raw := range lines {
		lineNum++
		absLine := blockStartLine + lineNum - 1
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "%%") {
			stripped := strings.TrimSpace(line[2:])
			if fields := strings.Fields(stripped); len(fields) > 0 && fields[0] == "config" {
				if err := parseConfigLine(stripped, g, absLine); err != nil {
					return nil, err
				}
			}
			continue
		}
		if hasAnyPrefix(line, "flowchart ", "graph ") || isStyleLine(line) {
			continue
		}
		if idx := inlineCommentStart(line); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		line = strings.TrimRight(line, "; \t")
		if line == "" {
			continue
		}
		if arrows := topLevelArrows(line); len(arrows) > 0 {
			if err := parseEdgeLine(line, arrows, g, absLine); err != nil {
				return nil, err
			}
			continue
		}
		if m := nodeRe.FindStringSubmatch(line); m != nil {
			if err := registerNode(g, m, absLine); err != nil {
				return nil, err
			}
			continue
		}
		return nil, &ParseError{Line: absLine, Raw: raw}
	}

	if len(g.Nodes) == 0 {
		return nil, &ParseError{Msg: "mermaid block declared no nodes"}
	}
	g.NormalizeGlobs()
	return g, nil
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// maskLine flags the bytes that sit inside a quoted string or a node label,
// where mermaid punctuation carries no structural meaning.
func maskLine(line string) []bool {
	mask := make([]bool, len(line))
	inQuotes, depth := false, 0
	for i := 0; i < len(line); i++ {
		mask[i] = inQuotes || depth > 0
		switch c := line[i]; {
		case c == '"':
			inQuotes = !inQuotes
		case c == '[' && !inQuotes:
			depth++
		case c == ']' && !inQuotes && depth > 0:
			depth--
		}
	}
	return mask
}

func inlineCommentStart(line string) int {
	mask := maskLine(line)
	for i := 0; i+1 < len(line); i++ {
		if !mask[i] && line[i] == '%' && line[i+1] == '%' {
			return i
		}
	}
	return -1
}

// topLevelArrows locates the link tokens that separate node groups, ignoring
// any that appear inside a node label.
func topLevelArrows(line string) [][]int {
	mask := maskLine(line)
	var arrows [][]int
	for _, loc := range arrowRe.FindAllStringIndex(line, -1) {
		if !mask[loc[0]] {
			arrows = append(arrows, loc)
		}
	}
	return arrows
}

// splitOutside splits on sep wherever it is not inside a quoted string or label.
func splitOutside(s string, sep byte) []string {
	mask := maskLine(s)
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep && !mask[i] {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

func registerNode(g *graph.Graph, m []string, lineNum int) error {
	id := m[1]

	rawGlob := m[2]
	if rawGlob == "" {
		rawGlob = m[3]
	}
	if strings.Contains(rawGlob, "*") {
		return &ParseError{
			Line: lineNum,
			Msg:  fmt.Sprintf("node %q uses raw \"*\" in glob %q; write &ast; instead", id, rawGlob),
		}
	}
	glob := decodeNodeGlob(rawGlob)

	if existing, ok := g.Nodes[id]; ok && existing != glob {
		return &ParseError{
			Line: lineNum,
			Msg:  fmt.Sprintf("node %q redefined with a different glob (%q vs %q)", id, existing, glob),
		}
	}
	g.Nodes[id] = glob
	if _, ok := g.NodeLines[id]; !ok {
		g.NodeOrder = append(g.NodeOrder, id)
		g.NodeLines[id] = lineNum
	}
	if _, ok := g.NodeDisplays[id]; !ok {
		g.NodeDisplays[id] = glob
	}

	if len(m) >= 5 && m[4] != "" {
		if g.Classes[id] == nil {
			g.Classes[id] = map[string]bool{}
		}
		for _, c := range strings.Split(m[4], ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				g.Classes[id][c] = true
			}
		}
	}
	return nil
}

// parseConfigLine parses a `config <key> <value>` directive. The value may be
// bare or wrapped in single or double quotes.
func parseConfigLine(line string, g *graph.Graph, lineNum int) error {
	fail := func(format string, args ...any) error {
		return &ParseError{Line: lineNum, Msg: fmt.Sprintf(format, args...)}
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "config"))
	key, value := rest, ""
	if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
		key, value = rest[:idx], strings.TrimSpace(rest[idx+1:])
	}
	switch {
	case key == "":
		return fail("config directive is missing a key: %q", line)
	case value == "":
		return fail("config key %q is missing a value", key)
	}
	if q := value[0]; q == '"' || q == '\'' {
		if len(value) < 2 || value[len(value)-1] != q {
			return fail("config key %q has an unterminated quoted value: %s", key, value)
		}
		value = value[1 : len(value)-1]
	}
	switch key {
	case "globSeparator":
		if value == "" {
			return fail("config key %q must not be empty", key)
		}
		g.GlobSeparator = value
	case "namespaceMode":
		switch strings.ToLower(value) {
		case "true":
			g.NamespaceMode = true
		case "false":
			g.NamespaceMode = false
		default:
			return fail("config key %q expects true or false, got %q", key, value)
		}
	default:
		return fail("unknown config key %q", key)
	}
	return nil
}

// parseEdgeLine reads a chain of node groups joined by arrows, where a group is
// one node or an `a & b` fan-out. Every node of a group is linked to every node
// of the next one, tolerated when the joining arrow was dotted.
func parseEdgeLine(line string, arrows [][]int, g *graph.Graph, lineNum int) error {
	groups := make([][]string, 0, len(arrows)+1)
	dotted := make([]bool, 0, len(arrows))
	pos := 0
	for _, loc := range arrows {
		if loc[0] < pos {
			continue // an arrow that lived inside a skipped edge label
		}
		ids, err := parseNodeGroup(line[pos:loc[0]], line, g, lineNum)
		if err != nil {
			return err
		}
		groups = append(groups, ids)
		dotted = append(dotted, strings.HasPrefix(line[loc[0]:loc[1]], "-."))
		pos = skipEdgeLabel(line, loc[1])
	}
	ids, err := parseNodeGroup(line[pos:], line, g, lineNum)
	if err != nil {
		return err
	}
	groups = append(groups, ids)

	for i := 0; i < len(groups)-1; i++ {
		for _, src := range groups[i] {
			for _, dst := range groups[i+1] {
				if err := addEdge(g, src, dst, dotted[i], lineNum); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func parseNodeGroup(segment, line string, g *graph.Graph, lineNum int) ([]string, error) {
	parts := splitOutside(segment, '&')
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, &ParseError{
				Line: lineNum,
				Msg:  fmt.Sprintf("edge has an empty node reference in %q", line),
			}
		}
		if m := nodeRe.FindStringSubmatch(part); m != nil {
			if err := registerNode(g, m, lineNum); err != nil {
				return nil, err
			}
			ids = append(ids, m[1])
			continue
		}
		if !isIdentifier(part) {
			return nil, &ParseError{
				Line: lineNum,
				Msg:  fmt.Sprintf("invalid node reference %q in edge %q", part, line),
			}
		}
		ids = append(ids, part)
	}
	return ids, nil
}

// skipEdgeLabel steps over a `|text|` label trailing an arrow; labels are
// decoration and carry no contract meaning.
func skipEdgeLabel(line string, pos int) int {
	i := pos
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i < len(line) && line[i] == '|' {
		if end := strings.IndexByte(line[i+1:], '|'); end >= 0 {
			return i + end + 2
		}
	}
	return pos
}

// addEdge records src → dst, tolerated only while every declaration of the pair
// is dotted: a solid one promotes an edge declared dotted elsewhere.
func addEdge(g *graph.Graph, src, dst string, dotted bool, lineNum int) error {
	if src == dst {
		return &ParseError{
			Line: lineNum,
			Msg:  fmt.Sprintf("edge references same node on both sides: %s → %s", src, dst),
		}
	}
	if _, ok := g.Edges[src]; !ok {
		g.Edges[src] = map[string]bool{}
	}
	key := graph.EdgeKey(src, dst)
	if !g.Edges[src][dst] {
		g.EdgeOrder = append(g.EdgeOrder, key)
		if dotted {
			g.Tolerated[key] = true
		}
	} else if !dotted {
		delete(g.Tolerated, key)
	}
	g.Edges[src][dst] = true
	if _, ok := g.EdgeLines[key]; !ok {
		g.EdgeLines[key] = lineNum
	}
	return nil
}

func arrowFor(dotted bool) string {
	if dotted {
		return " -.-> "
	}
	return " --> "
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func extractMermaidBlock(md string) (string, int, error) {
	lines := strings.Split(md, "\n")
	inside := false
	var buf strings.Builder
	blockStartLine := 0
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if !inside {
			if strings.HasPrefix(trim, "```mermaid") {
				inside = true
				blockStartLine = i + 2
			}
			continue
		}
		if strings.HasPrefix(trim, "```") {
			for j := i + 1; j < len(lines); j++ {
				if strings.HasPrefix(strings.TrimSpace(lines[j]), "```mermaid") {
					return "", 0, &ParseError{Line: j + 1, Msg: "multiple ```mermaid blocks found"}
				}
			}
			return buf.String(), blockStartLine, nil
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if inside {
		return "", 0, &ParseError{Msg: "unclosed ```mermaid block"}
	}
	return "", 0, &ParseError{Msg: "no ```mermaid block found"}
}

func encodeNodeGlob(s string) string {
	return strings.ReplaceAll(s, "*", "&ast;")
}

func decodeNodeGlob(s string) string {
	return globDecodeReplacer.Replace(s)
}

// encodeNodeId maps a graph node id onto a mermaid-safe identifier. Load keeps
// ids verbatim, so encoding must be idempotent for a loaded graph to
// re-serialize unchanged.
func encodeNodeId(s string) string {
	if s == "" || s == "." {
		return "root"
	}
	result := nodeIdReplacer.Replace(s)
	if result[0] >= '0' && result[0] <= '9' {
		result = "n" + result
	}
	return result
}

func quotedEncode(s string) string {
	return fmt.Sprintf("%q", encodeNodeGlob(s))
}

func looksLikeFilePath(p string) bool {
	if p == "." || p == "" {
		return false
	}
	lastSlash := -1
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			lastSlash = i
			break
		}
	}
	var last string
	if lastSlash >= 0 {
		last = p[lastSlash+1:]
	} else {
		last = p
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
