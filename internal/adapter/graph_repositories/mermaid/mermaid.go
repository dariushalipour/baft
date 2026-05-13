package mermaid

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dariushalipour/baft/internal/domain/graph"
)

var (
	// Pre-compiled regex for node matching - compiled once at package init.
	nodeRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\[(?:"([^"]*)"|([^\]]*))\](?::::([A-Za-z_][A-Za-z0-9_,]*))?$`)

	// Pre-built replacers for node ID encoding/decoding to avoid per-call allocation.
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

	nodeIdDecodeReplacer = strings.NewReplacer(
		"_slash_", "/",
		"_dot_", ".",
		"_dash_", "-",
		"_asterisk_", "*",
		"_atsign_", "@",
		"_lsqb_", "[",
		"_rsqb_", "]",
		"_lbrace_", "{",
		"_rbrace_", "}",
		"_plus_", "+",
		"_qmark_", "?",
		"_comma_", ",",
		"_space_", " ",
		"_tab_", "\t",
		"_newline_", "\n",
		"_carriage_return_", "\r",
		"_vertical_tab_", "\x0b",
		"_form_feed_", "\x0c",
	)

	globDecodeReplacer = strings.NewReplacer(
		"&ast;", "*",
		"&#42;", "*",
	)
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

// parseErrors holds multiple ParseError instances and implements error.
type parseErrors []ParseError

func (pe parseErrors) Error() string {
	msgs := make([]string, len(pe))
	for i, e := range pe {
		msgs[i] = e.Msg
	}
	return strings.Join(msgs, "; ")
}

// ParseErrorWithNext allows chaining via Unwrap for errors.As compatibility.
type ParseErrorWithNext struct {
	Msg  string
	Line int
	Raw  string
	Next error
}

func (e *ParseErrorWithNext) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("line %d: unrecognized line in mermaid block: %q", e.Line, e.Raw)
}

func (e *ParseErrorWithNext) Unwrap() error { return e.Next }

// toChain converts parseErrors into a linked chain of ParseErrorWithNext
// so that errors.As can walk through every element.
func (pe parseErrors) toChain() *ParseErrorWithNext {
	if len(pe) == 0 {
		return nil
	}
	// Sort for deterministic output: by line, then message.
	sort.Slice(pe, func(i, j int) bool {
		if pe[i].Line != pe[j].Line {
			return pe[i].Line < pe[j].Line
		}
		return pe[i].Msg < pe[j].Msg
	})
	// Build chain from last to first so Unwrap points to the next error.
	var head *ParseErrorWithNext
	for i := len(pe) - 1; i >= 0; i-- {
		e := &pe[i]
		head = &ParseErrorWithNext{
			Msg:  e.Msg,
			Line: e.Line,
			Raw:  e.Raw,
			Next: head,
		}
	}
	return head
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
		if preserved, ok := g.NodeDisplays[id]; ok && preserved == glob {
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
			sb.WriteString("  ")
			sb.WriteString(encodeNodeId(src))
			sb.WriteString(" --> ")
			sb.WriteString(encodeNodeId(dst))
			sb.WriteByte('\n')
		}
	}

	sb.WriteString("```\n")
	return sb.String()
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

// Load extracts the mermaid block from markdown and builds a Graph.
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
		if line[0] == '%' && len(line) >= 2 && line[1] == '%' {
			continue
		}
		if line == "flowchart TD" || line == "flowchart LR" || line == "flowchart RL" || line == "flowchart BT" ||
			strings.HasPrefix(line, "flowchart ") || line == "graph TD" || line == "graph LR" || line == "graph RL" || line == "graph BT" ||
			strings.HasPrefix(line, "graph ") || strings.HasPrefix(line, "classDef ") {
			continue
		}
		if idx := strings.Index(line, "%%"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
			if line == "" {
				continue
			}
		}
		if strings.Contains(line, "-->") {
			if err := parseEdgeLine(line, g, absLine); err != nil {
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
	var allErrs parseErrors
	if err := checkEmptyGlobs(g); err != nil {
		if pe, ok := err.(parseErrors); ok {
			allErrs = append(allErrs, pe...)
		} else {
			allErrs = append(allErrs, *err.(*ParseError))
		}
	}
	if err := checkUndefinedEdgeNodes(g); err != nil {
		if pe, ok := err.(parseErrors); ok {
			allErrs = append(allErrs, pe...)
		} else {
			allErrs = append(allErrs, *err.(*ParseError))
		}
	}
	if err := checkCycles(g); err != nil {
		if pe, ok := err.(parseErrors); ok {
			allErrs = append(allErrs, pe...)
		} else {
			allErrs = append(allErrs, *err.(*ParseError))
		}
	}
	if len(allErrs) > 0 {
		return nil, allErrs.toChain()
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
	if _, ok := g.NodeDisplays[id]; !ok {
		g.NodeDisplays[id] = glob
	}
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

func parseEdgeLine(line string, g *graph.Graph, lineNum int) error {
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
	var errs parseErrors
	for id, glob := range g.Nodes {
		if glob == "" {
			errs = append(errs, ParseError{
				Line: g.NodeLines[id],
				Msg:  fmt.Sprintf("node %q has empty glob", id),
			})
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// encodeNodeGlob escapes special characters in glob/display text for mermaid output.
func encodeNodeGlob(s string) string {
	return strings.ReplaceAll(s, "*", "&ast;")
}

// decodeNodeGlob reverses encodeNodeGlob.
func decodeNodeGlob(s string) string {
	return globDecodeReplacer.Replace(s)
}

// encodeNodeId produces a valid mermaid identifier from a raw node ID.
func encodeNodeId(s string) string {
	if s == "" || s == "." {
		return "root"
	}
	result := nodeIdReplacer.Replace(s)
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
	return nodeIdDecodeReplacer.Replace(s)
}

// quotedEncode wraps the encoded glob in Go-style double quotes for mermaid output.
func quotedEncode(s string) string {
	return fmt.Sprintf("%q", encodeNodeGlob(s))
}

func checkUndefinedEdgeNodes(g *graph.Graph) error {
	var errs parseErrors
	for src, dsts := range g.Edges {
		for dst := range dsts {
			if _, ok := g.Nodes[src]; !ok {
				line := g.EdgeLines[src+"\t"+dst]
				errs = append(errs, ParseError{Line: line, Msg: fmt.Sprintf("edge references undefined node %q", src)})
			}
			if _, ok := g.Nodes[dst]; !ok {
				line := g.EdgeLines[src+"\t"+dst]
				errs = append(errs, ParseError{Line: line, Msg: fmt.Sprintf("edge references undefined node %q", dst)})
			}
		}
	}
	if len(errs) > 0 {
		return errs
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
	color := make(map[string]state, len(g.Nodes))
	for id := range g.Nodes {
		color[id] = white
	}
	// Pre-allocate path with capacity for all nodes.
	path := make([]string, 0, len(g.Nodes))
	var errs parseErrors
	seenCycles := make(map[string]struct{})
	var dfs func(node string)
	dfs = func(node string) {
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
					cycleStr := ""
					for i := cycleStart; i < len(path); i++ {
						if i > cycleStart {
							cycleStr += " \u2192 "
						}
						cycleStr += path[i]
					}
					cycleStr += " \u2192 " + dst
					// Deduplicate cycles by their canonical string representation.
					if _, ok := seenCycles[cycleStr]; !ok {
						seenCycles[cycleStr] = struct{}{}
						line := g.EdgeLines[path[len(path)-1]+"\t"+dst]
						errs = append(errs, ParseError{Line: line, Msg: "cycle detected: " + cycleStr})
					}
				}
			} else if c == white {
				dfs(dst)
			}
		}
		path = path[:len(path)-1]
		color[node] = black
	}
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if color[id] == white {
			dfs(id)
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func looksLikeFilePath(p string) bool {
	if p == "." || p == "" {
		return false
	}
	// Find last '/' using byte scan.
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
	// Check for dot in last segment.
	for i := 0; i < len(last); i++ {
		if last[i] == '.' {
			return true
		}
	}
	return false
}
