package mermaid

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/domain/graph"
)

// MermaidRepository implements port.GraphRepository using mermaid flowchart format.
type MermaidRepository struct{}

// ParseError is returned when a mermaid block cannot be parsed.
type ParseError struct {
	Line  int
	Raw   string
	Label string
	Msg   string
}

func (e *ParseError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("line %d: unrecognized line in mermaid block: %q", e.Line, e.Raw)
}

// Save produces a mermaid flowchart from the Graph.
// Directory nodes (non-file globs) get a "/**" suffix for mermaid display.
func (r *MermaidRepository) Save(g *graph.Graph) string {
	var sb strings.Builder

	sb.WriteString("<!-- BAFT — Architecture Contract: edit this file to change allowed imports. -->\n")
	sb.WriteString("<!-- AI agents and developers working in this codebase: if BAFT is unfamiliar, run `baft manual` to study the contract format and rules. -->\n")
	sb.WriteString("<!-- Nodes claim files with globs. Arrows allow imports. `:::endophobic` forbids same-node imports. -->\n")
	sb.WriteString("<!-- Check this contract with `baft check .` -->\n")
	sb.WriteString("\n")
	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart TD\n")

	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		glob := g.Nodes[id]
		display := glob
		if glob != "." && !looksLikeFilePath(glob) && !strings.HasSuffix(glob, "/**") {
			display = glob + "/**"
		}
		sb.WriteString(fmt.Sprintf("  %s[%s]\n", encodeNodeId(id), quotedEncode(display)))
	}

	sb.WriteString("\n")

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
			sb.WriteString(fmt.Sprintf("  %s --> %s\n", encodeNodeId(src), encodeNodeId(dst)))
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}

// Load extracts the mermaid block from markdown and builds a Graph.
func (r *MermaidRepository) Load(md string) (*graph.Graph, error) {
	block, blockStartLine, err := extractMermaidBlock(md)
	if err != nil {
		return nil, err
	}

	g := &graph.Graph{
		Nodes:     map[string]string{},
		Edges:     map[string]map[string]bool{},
		Classes:   map[string]map[string]bool{},
		NodeLines: map[string]int{},
		EdgeLines: map[string]int{},
	}

	lines := strings.Split(block, "\n")
	nodeRe := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\[(?:"([^"]*)"|([^\]]*))\](?::::([A-Za-z_][A-Za-z0-9_,]*))?$`)

	lineNum := 0
	for _, raw := range lines {
		lineNum++
		absLine := blockStartLine + lineNum - 1
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "%%") || strings.HasPrefix(line, "flowchart") || strings.HasPrefix(line, "graph") {
			continue
		}
		if strings.HasPrefix(line, "classDef ") {
			continue
		}
		if idx := strings.Index(line, "%%"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
			if line == "" {
				continue
			}
		}
		if strings.Contains(line, "-->") {
			if err := parseEdgeLine(line, g, nodeRe, absLine); err != nil {
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
		trimmed := strings.TrimSpace(raw)
		label := ""
		if idx := strings.Index(trimmed, "["); idx > 0 {
			label = strings.TrimSpace(trimmed[:idx])
		}
		return nil, &ParseError{Line: absLine, Raw: raw, Label: label}
	}

	if len(g.Nodes) == 0 {
		return nil, &ParseError{Msg: "mermaid block declared no nodes"}
	}
	if err := checkEmptyGlobs(g); err != nil {
		return nil, err
	}
	if err := checkUndefinedEdgeNodes(g); err != nil {
		return nil, err
	}
	if err := checkCycles(g); err != nil {
		return nil, err
	}
	return g, nil
}

func registerNode(g *graph.Graph, m []string, lineNum int) error {
	rawID := m[1]
	id := decodeNodeId(rawID)

	rawGlob := m[2]
	if rawGlob == "" {
		rawGlob = m[3]
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
		g.NodeLines[id] = lineNum
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

func parseEdgeLine(line string, g *graph.Graph, nodeRe *regexp.Regexp, lineNum int) error {
	tokens := splitArrow(line)
	if len(tokens) < 2 {
		return &ParseError{
			Line: lineNum,
			Msg:  fmt.Sprintf("edge has fewer than two nodes: %q", line),
		}
	}
	ids := make([]string, len(tokens))
	for i, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if m := nodeRe.FindStringSubmatch(tok); m != nil {
			if err := registerNode(g, m, lineNum); err != nil {
				return err
			}
			ids[i] = decodeNodeId(m[1])
			continue
		}
		if !isIdentifier(tok) {
			return &ParseError{
				Line: lineNum,
				Msg:  fmt.Sprintf("invalid edge token %q in line %q", tok, line),
			}
		}
		ids[i] = decodeNodeId(tok)
	}
	for i := 0; i < len(ids)-1; i++ {
		src, dst := ids[i], ids[i+1]
		if src == dst {
			return &ParseError{
				Line: lineNum,
				Msg:  fmt.Sprintf("edge references same node on both sides: %s → %s", src, dst),
			}
		}
		if _, ok := g.Edges[src]; !ok {
			g.Edges[src] = map[string]bool{}
		}
		g.Edges[src][dst] = true
		g.EdgeLines[src+"\t"+dst] = lineNum
	}
	return nil
}

func splitArrow(line string) []string {
	parts := strings.Split(line, "-->")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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
			// Check for a second mermaid block after this one closes
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

func checkEmptyGlobs(g *graph.Graph) error {
	for id, glob := range g.Nodes {
		if glob == "" {
			return &ParseError{
				Line: g.NodeLines[id],
				Msg:  fmt.Sprintf("node %q has empty glob", id),
			}
		}
	}
	return nil
}

// encodeNodeGlob escapes special characters in glob/display text for mermaid output.
func encodeNodeGlob(s string) string {
	return strings.ReplaceAll(s, "*", "&ast;")
}

// decodeNodeGlob reverses encodeNodeGlob.
func decodeNodeGlob(s string) string {
	return strings.NewReplacer(
		"&ast;", "*",
		"&#42;", "*",
	).Replace(s)
}

// encodeNodeId produces a valid mermaid identifier from a raw node ID.
func encodeNodeId(s string) string {
	if s == "" || s == "." {
		return "root"
	}
	result := strings.NewReplacer(
		"/", "_slash_",
		".", "_dot_",
		"-", "_dash_",
		"*", "_asterisk_",
		"@", "_atsign_",
		"[", "_lsqb_",
		"]", "_rsqb_",
		"{", "_lbrace_",
		"}", "_rbrace_",
	).Replace(s)
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "n" + result
	}
	return result
}

// decodeNodeId reverses encodeNodeId.
func decodeNodeId(s string) string {
	if s == "root" {
		return "."
	}
	if len(s) > 1 && s[0] == 'n' && s[1] >= '0' && s[1] <= '9' {
		s = s[1:]
	}
	return strings.NewReplacer(
		"_slash_", "/",
		"_dot_", ".",
		"_dash_", "-",
		"_asterisk_", "*",
		"_atsign_", "@",
		"_lsqb_", "[",
		"_rsqb_", "]",
		"_lbrace_", "{",
		"_rbrace_", "}",
	).Replace(s)
}

// quotedEncode wraps the encoded glob in Go-style double quotes for mermaid output.
func quotedEncode(s string) string {
	return fmt.Sprintf("%q", encodeNodeGlob(s))
}

func checkUndefinedEdgeNodes(g *graph.Graph) error {
	for src, dsts := range g.Edges {
		for dst := range dsts {
			if _, ok := g.Nodes[src]; !ok {
				line := g.EdgeLines[src+"\t"+dst]
				return &ParseError{Line: line, Msg: fmt.Sprintf("edge references undefined node %q", src)}
			}
			if _, ok := g.Nodes[dst]; !ok {
				line := g.EdgeLines[src+"\t"+dst]
				return &ParseError{Line: line, Msg: fmt.Sprintf("edge references undefined node %q", dst)}
			}
		}
	}
	return nil
}

func checkCycles(g *graph.Graph) error {
	type state byte
	const (
		white state = iota
		gray
		black
	)
	color := make(map[string]state)
	for id := range g.Nodes {
		color[id] = white
	}
	var path []string
	var dfs func(node string) error
	dfs = func(node string) error {
		color[node] = gray
		path = append(path, node)
		for dst := range g.Edges[node] {
			if c, ok := color[dst]; !ok {
				continue
			} else if c == gray {
				cycleStart := -1
				for i, p := range path {
					if p == dst {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := append(path[cycleStart:], dst)
					cycleStr := ""
					for i, n := range cycle {
						if i > 0 {
							cycleStr += " → "
						}
						cycleStr += n
					}
					line := g.EdgeLines[path[len(path)-1]+"\t"+dst]
					return &ParseError{Line: line, Msg: fmt.Sprintf("cycle detected: %s", cycleStr)}
				}
			} else if c == white {
				if err := dfs(dst); err != nil {
					return err
				}
			}
		}
		path = path[:len(path)-1]
		color[node] = black
		return nil
	}
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if color[id] == white {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func looksLikeFilePath(p string) bool {
	if p == "." || p == "" {
		return false
	}
	segs := strings.Split(p, "/")
	last := segs[len(segs)-1]
	if last == "." || last == ".." {
		return false
	}
	return strings.Contains(last, ".")
}
