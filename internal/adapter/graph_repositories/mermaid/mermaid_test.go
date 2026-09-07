package mermaid

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/domain/graph"
	"github.com/dariushalipour/baft/internal/port"
)

func TestMermaidRepository_Load(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  main["."]` + "\n" +
		`  api["internal/api/&ast;&ast;"]` + "\n" +
		`  domain["internal/domain/&ast;&ast;"]` + "\n" +
		"  main --> api --> domain\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("nodes: got %d, want 3", len(g.Nodes))
	}
	if !g.Allows("main", "api") || !g.Allows("api", "domain") {
		t.Fatalf("edges missing: %+v", g.Edges)
	}
}

func TestMermaidRepository_LoadToleratesComments(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		"  %% diagram note\n" +
		`  alpha["alpha"] %% keep this node` + "\n" +
		`  literal["literal%%label"]` + "\n" +
		`  beta["beta"]` + "\n" +
		"  alpha --> beta %% edge note\n" +
		"```\n"
	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if g.Nodes["literal"] != "literal%%label" {
		t.Fatalf("literal node glob = %q, want %q", g.Nodes["literal"], "literal%%label")
	}
	if !g.Allows("alpha", "beta") {
		t.Fatalf("expected edge alpha --> beta")
	}
}
func TestMermaidRepository_LoadEscapedGlobs(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  dom["src/domain/&ast;&ast;"]` + "\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if g.Nodes["dom"] != "src/domain/**" {
		t.Errorf("node glob = %q, want \"src/domain/**\"", g.Nodes["dom"])
	}
}

func TestMermaidRepository_LoadRejectsRawAsterisks(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  dom["src/domain/**"]` + "\n" +
		"```\n"

	_, err := (&MermaidRepository{}).Load(md)
	if err == nil {
		t.Fatal("expected error for raw * in node glob")
	}
	if got := err.Error(); !strings.Contains(got, `write &ast; instead`) {
		t.Fatalf("unexpected error %q", got)
	}
}

// TestDocumentedContractsLoad pins the documentation to the parser: every
// example a reader may copy verbatim has to load.
func TestDocumentedContractsLoad(t *testing.T) {
	blockRe := regexp.MustCompile("(?s)```mermaid\n.*?\n```")
	for _, path := range []string{"../../../../README.md", "../../../../docs/manual.md", "../../../../docs/concepts/contract.md"} {
		md, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, block := range blockRe.FindAllString(string(md), -1) {
			if _, err := (&MermaidRepository{}).Load(block); err != nil {
				t.Errorf("%s: %v in:\n%s", path, err, block)
			}
		}
	}
}

func TestMermaidRepository_LoadEndophobicClass(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  uc["internal/usecase/&ast;&ast;"]:::endophobic` + "\n" +
		`  svc["internal/service/&ast;&ast;"]` + "\n" +
		"  classDef endophobic stroke-dasharray: 5 5\n" +
		"  uc --> svc\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !g.IsEndophobic("uc") {
		t.Error("uc should be endophobic")
	}
	if g.IsEndophobic("svc") {
		t.Error("svc should not be endophobic")
	}
}

func TestMermaidRepository_LoadEmptyBlock(t *testing.T) {
	_, err := (&MermaidRepository{}).Load("```mermaid\nflowchart TD\n```\n")
	if err == nil {
		t.Fatal("expected error for empty block")
	}
}

func TestMermaidRepository_LoadNoBlock(t *testing.T) {
	_, err := (&MermaidRepository{}).Load("no mermaid here\n")
	if err == nil {
		t.Fatal("expected error for missing block")
	}
}

func TestMermaidRepository_Save(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"main":   ".",
			"api":    "internal/api/**",
			"domain": "internal/domain/**",
		},
		Edges: map[string]map[string]bool{
			"main": {"api": true},
			"api":  {"domain": true},
		},
		Classes: map[string]map[string]bool{},
	}

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{})

	if !strings.Contains(out, "```mermaid") {
		t.Error("missing mermaid fence")
	}
	if !strings.Contains(out, "flowchart TD") {
		t.Error("missing flowchart TD")
	}
	if !strings.Contains(out, "main") {
		t.Error("missing main node")
	}
	if !strings.Contains(out, "api") {
		t.Error("missing api node")
	}
	if !strings.Contains(out, "main --> api") {
		t.Error("missing main->api edge")
	}
	if !strings.Contains(out, "api --> domain") {
		t.Error("missing api->domain edge")
	}
}

func TestMermaidRepository_SavePreservesClasses(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"usecase": "internal/usecase/**",
		},
		Edges: map[string]map[string]bool{},
		Classes: map[string]map[string]bool{
			"usecase": {
				"zeta":       true,
				"endophobic": true,
				"disabled":   false,
			},
		},
	}

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{})

	if !strings.Contains(out, `usecase["internal/usecase/&ast;&ast;"]:::endophobic,zeta`) {
		t.Fatalf("missing serialized classes in:\n%s", out)
	}
}

func TestMermaidRepository_SaveDirGlobSuffix(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"api":    "internal/api",
			"domain": "internal/domain/model.ts",
		},
		Edges:   map[string]map[string]bool{},
		Classes: map[string]map[string]bool{},
	}

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{})

	if !strings.Contains(out, "internal/api/&ast;&ast;") {
		t.Errorf("expected escaped dir glob suffix in:\n%s", out)
	}
	if !strings.Contains(out, "internal/domain/model.ts") {
		t.Error("expected file path unchanged")
	}
}

func TestMermaidRepository_SaveDeterministicOrder(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"z": "z",
			"a": "a",
			"m": "m",
		},
		Edges:   map[string]map[string]bool{},
		Classes: map[string]map[string]bool{},
	}

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{})

	aIdx := strings.Index(out, "  a[")
	mIdx := strings.Index(out, "  m[")
	zIdx := strings.Index(out, "  z[")
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("nodes not sorted: a=%d m=%d z=%d", aIdx, mIdx, zIdx)
	}
}

func TestMermaidRepository_SaveGroupsRepeatedLinkStyles(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"alpha": "alpha",
			"beta":  "beta",
			"delta": "delta",
			"gamma": "gamma",
		},
		Edges: map[string]map[string]bool{
			"alpha": {
				"beta":  true,
				"delta": true,
				"gamma": true,
			},
		},
		Classes: map[string]map[string]bool{},
	}

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono})

	if !strings.Contains(out, "linkStyle 0,1,2 stroke:#1f1f1f,stroke-width:2px") {
		t.Fatalf("missing grouped linkStyle in:\n%s", out)
	}
	if strings.Contains(out, "linkStyle 0 stroke:#1f1f1f,stroke-width:2px") {
		t.Fatalf("unexpected ungrouped first linkStyle in:\n%s", out)
	}
	if strings.Contains(out, "linkStyle 1 stroke:#1f1f1f,stroke-width:2px") {
		t.Fatalf("unexpected ungrouped second linkStyle in:\n%s", out)
	}
	if strings.Contains(out, "linkStyle 2 stroke:#1f1f1f,stroke-width:2px") {
		t.Fatalf("unexpected ungrouped third linkStyle in:\n%s", out)
	}
}

func TestRoundTrip_LoadSaveLoad(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  main["."]` + "\n" +
		`  api["internal/api/&ast;&ast;"]` + "\n" +
		`  domain["internal/domain/&ast;&ast;"]` + "\n" +
		`  usecase["internal/usecase/&ast;&ast;"]:::endophobic` + "\n" +
		"  classDef endophobic stroke-dasharray: 5 5\n" +
		"  main --> api --> usecase --> domain\n" +
		"  main --> usecase\n" +
		"```\n"

	original, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}

	saved := (&MermaidRepository{}).Save(original, port.GraphSaveOptions{})
	if !strings.Contains(saved, generatedStyleComment) {
		t.Fatalf("expected generated style comment in:\n%s", saved)
	}
	roundTrip, err := (&MermaidRepository{}).Load(saved)
	if err != nil {
		t.Fatalf("round-trip load: %v\nsaved:\n%s", err, saved)
	}

	if len(original.Nodes) != len(roundTrip.Nodes) {
		t.Fatalf("node count mismatch: %d vs %d", len(original.Nodes), len(roundTrip.Nodes))
	}
	for id, glob := range original.Nodes {
		if roundTrip.Nodes[id] != glob {
			t.Errorf("node %q glob mismatch: got %q, want %q", id, roundTrip.Nodes[id], glob)
		}
	}

	origEdges := graph.NewGraphIndex(original).EdgeCount()
	rtEdges := graph.NewGraphIndex(roundTrip).EdgeCount()
	if origEdges != rtEdges {
		t.Fatalf("edge count mismatch: %d vs %d", origEdges, rtEdges)
	}
	for src, dsts := range original.Edges {
		for dst := range dsts {
			if !roundTrip.Allows(src, dst) {
				t.Errorf("missing edge %s --> %s after round-trip", src, dst)
			}
		}
	}
	if !roundTrip.IsEndophobic("usecase") {
		t.Error("usecase should remain endophobic after round-trip")
	}
}

func TestRoundTrip_LoadSaveLoadPreservesBareDirDisplay(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  dirx["dirx"]` + "\n" +
		"```\n"

	original, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}

	saved := (&MermaidRepository{}).Save(original, port.GraphSaveOptions{})
	if !strings.Contains(saved, `dirx["dirx"]`) {
		t.Fatalf("expected bare directory glob to be preserved in:\n%s", saved)
	}

	roundTrip, err := (&MermaidRepository{}).Load(saved)
	if err != nil {
		t.Fatalf("round-trip load: %v\nsaved:\n%s", err, saved)
	}
	if roundTrip.Nodes["dirx"] != "dirx" {
		t.Fatalf("node %q glob mismatch: got %q, want %q", "dirx", roundTrip.Nodes["dirx"], "dirx")
	}
}

func TestRoundTrip_RawGraph(t *testing.T) {
	nodes := map[string]string{
		"src/domain":            "src/domain",
		"src/api/router.ts":     "src/api/router.ts",
		"src/api/handler.ts":    "src/api/handler.ts",
		"src/usecase/create.ts": "src/usecase/create.ts",
	}
	edges := map[string]map[string]bool{
		"src/api/router.ts":     {"src/domain": true},
		"src/api/handler.ts":    {"src/usecase/create.ts": true},
		"src/usecase/create.ts": {"src/domain": true},
	}

	graph := rawToGraph(nodes, edges)
	saved := (&MermaidRepository{}).Save(graph, port.GraphSaveOptions{})
	roundTrip, err := (&MermaidRepository{}).Load(saved)
	if err != nil {
		t.Fatalf("load saved analysis: %v\n%s", err, saved)
	}

	expectedGlobs := map[string]string{
		"src/domain":            "src/domain/**",
		"src/api/router.ts":     "src/api/router.ts",
		"src/api/handler.ts":    "src/api/handler.ts",
		"src/usecase/create.ts": "src/usecase/create.ts",
	}
	for id, want := range expectedGlobs {
		encoded := encodeNodeId(id)
		if roundTrip.Nodes[encoded] != want {
			t.Errorf("node %q: got %q, want %q", encoded, roundTrip.Nodes[encoded], want)
		}
	}
}

func TestMermaidEscape_RoundTripAll(t *testing.T) {
	cases := []struct {
		name string
		glob string
	}{
		{"asterisk", "internal/api/**"},
		{"slash", "src/domain/**"},
		{"dot", "src/model.ts"},
		{"dash", "my-pkg/**"},
		{"at-sign", "@scope/pkg/**"},
		{"lbracket", "pkg[name]/**"},
		{"rbracket", "pkg[name]/**"},
		{"lbrace", "pkg{name}/**"},
		{"rbrace", "pkg{name}/**"},
		{"all special chars", "@scope/my-pkg[name]{ver}/src/model.ts"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &graph.Graph{
				Nodes: map[string]string{
					"node": tc.glob,
				},
				Edges:   map[string]map[string]bool{},
				Classes: map[string]map[string]bool{},
			}

			saved := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{})
			loaded, err := (&MermaidRepository{}).Load(saved)
			if err != nil {
				t.Fatalf("load after save: %v\nsaved:\n%s", err, saved)
			}
			if loaded.Nodes["node"] != tc.glob {
				t.Errorf("round-trip mismatch: got %q, want %q\nsaved:\n%s", loaded.Nodes["node"], tc.glob, saved)
			}
		})
	}
}

func TestRoundTrip_SpecialCharNodeIDs(t *testing.T) {
	cases := []struct {
		name string
		id   string
		glob string
	}{
		{"slash", "src/domain", "src/domain/**"},
		{"dot", "src/model.ts", "src/model.ts"},
		{"dash", "my-pkg", "my-pkg/**"},
		{"asterisk", "internal/api/**", "internal/api/**"},
		{"at-sign", "@scope/pkg", "@scope/pkg/**"},
		{"lbracket", "pkg[name]", "pkg[name]/**"},
		{"rbracket", "pkg[name]", "pkg[name]/**"},
		{"lbrace", "pkg{ver}", "pkg{ver}/**"},
		{"rbrace", "pkg{ver}", "pkg{ver}/**"},
		{"all special chars", "@scope/my-pkg[name]{ver}/src/model.ts", "@scope/my-pkg[name]{ver}/src/model.ts"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &graph.Graph{
				Nodes: map[string]string{
					tc.id: tc.glob,
				},
				Edges:   map[string]map[string]bool{},
				Classes: map[string]map[string]bool{},
			}

			saved := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{})
			loaded, err := (&MermaidRepository{}).Load(saved)
			if err != nil {
				t.Fatalf("load after save: %v\nsaved:\n%s", err, saved)
			}
			id := encodeNodeId(tc.id)
			if loaded.Nodes[id] != tc.glob {
				t.Errorf("round-trip mismatch: got %q, want %q\nsaved:\n%s", loaded.Nodes[id], tc.glob, saved)
			}
			if resaved := (&MermaidRepository{}).Save(loaded, port.GraphSaveOptions{}); resaved != saved {
				t.Errorf("re-save is not stable:\n%s\nvs\n%s", resaved, saved)
			}
		})
	}
}

func TestRoundTrip_SpecialCharEdges(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"src/domain":  "src/domain",
			"@scope/pkg":  "@scope/pkg",
			"my-pkg[ver]": "my-pkg[ver]",
		},
		Edges: map[string]map[string]bool{
			"@scope/pkg":  {"src/domain": true},
			"my-pkg[ver]": {"@scope/pkg": true},
		},
		Classes: map[string]map[string]bool{},
	}

	saved := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{})
	loaded, err := (&MermaidRepository{}).Load(saved)
	if err != nil {
		t.Fatalf("load after save: %v\nsaved:\n%s", err, saved)
	}

	if !loaded.Allows(encodeNodeId("@scope/pkg"), encodeNodeId("src/domain")) {
		t.Error("missing edge @scope/pkg --> src/domain")
	}
	if !loaded.Allows(encodeNodeId("my-pkg[ver]"), encodeNodeId("@scope/pkg")) {
		t.Error("missing edge my-pkg[ver] --> @scope/pkg")
	}
}

func TestSave_OutputEncoding(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"src/domain":  "src/domain",
			"@scope/pkg":  "@scope/pkg",
			"my-pkg[ver]": "my-pkg[ver]",
			"pkg{ver}":    "pkg{ver}",
		},
		Edges: map[string]map[string]bool{
			"@scope/pkg":  {"src/domain": true},
			"my-pkg[ver]": {"@scope/pkg": true},
		},
		Classes: map[string]map[string]bool{},
	}

	saved := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{})

	// Node IDs are encoded
	if !strings.Contains(saved, "src_slash_domain[") {
		t.Errorf("missing encoded node ID src_slash_domain in:\n%s", saved)
	}
	if !strings.Contains(saved, "_atsign_scope_slash_pkg[") {
		t.Errorf("missing encoded node ID _atsign_scope_slash_pkg in:\n%s", saved)
	}
	if !strings.Contains(saved, "my_dash_pkg_lsqb_ver_rsqb_[") {
		t.Errorf("missing encoded node ID my_dash_pkg_lsqb_ver_rsqb_ in:\n%s", saved)
	}
	if !strings.Contains(saved, "pkg_lbrace_ver_rbrace_[") {
		t.Errorf("missing encoded node ID pkg_lbrace_ver_rbrace_ in:\n%s", saved)
	}

	// Globs are escaped
	if !strings.Contains(saved, "src/domain/&ast;&ast;") {
		t.Errorf("missing escaped glob for src/domain in:\n%s", saved)
	}
	if !strings.Contains(saved, "src/domain/&ast;&ast;") {
		t.Errorf("missing escaped glob for src/domain in:\n%s", saved)
	}
	if !strings.Contains(saved, "@scope/pkg/&ast;&ast;") {
		t.Errorf("missing escaped glob for @scope/pkg in:\n%s", saved)
	}
	if !strings.Contains(saved, "my-pkg[ver]/&ast;&ast;") {
		t.Errorf("missing escaped glob for my-pkg[ver] in:\n%s", saved)
	}
	if !strings.Contains(saved, "pkg{ver}/&ast;&ast;") {
		t.Errorf("missing escaped glob for pkg{ver} in:\n%s", saved)
	}

	// Edges use encoded IDs
	if !strings.Contains(saved, "_atsign_scope_slash_pkg --> src_slash_domain") {
		t.Errorf("missing encoded edge in:\n%s", saved)
	}
	if !strings.Contains(saved, "my_dash_pkg_lsqb_ver_rsqb_ --> _atsign_scope_slash_pkg") {
		t.Errorf("missing encoded edge in:\n%s", saved)
	}
}

func TestMermaidRepository_LoadInlineEdge(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  app["internal/application/&ast;&ast;"] --> domain["internal/domain/&ast;&ast;"]` + "\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("nodes: got %d, want 2", len(g.Nodes))
	}
	if !g.Allows("app", "domain") {
		t.Fatalf("expected edge app --> domain")
	}
	if g.Allows("domain", "app") {
		t.Fatalf("unexpected edge domain --> app")
	}
}

func TestMermaidRepository_LoadNodesAndEdges(t *testing.T) {
	md := "prelude\n\n```mermaid\nflowchart TD\n" +
		`  main["."]` + "\n" +
		`  httpapi["internal/adapter/inbound/httpapi/&ast;&ast;"]` + "\n" +
		`  usecase["internal/usecase/&ast;&ast;"]` + "\n" +
		"  main --> httpapi --> usecase\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("nodes: got %d, want 3", len(g.Nodes))
	}
	if !g.Allows("main", "httpapi") || !g.Allows("httpapi", "usecase") {
		t.Fatalf("expected edges not present: %+v", g.Edges)
	}
	if g.Allows("usecase", "httpapi") {
		t.Fatalf("unexpected edge usecase->httpapi")
	}
	if !g.Allows("usecase", "usecase") {
		t.Fatalf("same-node should always be allowed")
	}
}

func TestMermaidRepository_DuplicateGlobLoads(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a["internal/x/&ast;&ast;"]` + "\n" +
		`  b["internal/x/&ast;&ast;"]` + "\n" +
		"```\n"
	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("nodes: got %d, want 2", len(g.Nodes))
	}
	if g.Nodes["a"] != "internal/x/**" || g.Nodes["b"] != "internal/x/**" {
		t.Fatalf("unexpected nodes: %+v", g.Nodes)
	}
}

func TestMermaidRepository_EndophobicClass(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  usecase["internal/usecase/&ast;&ast;"]:::endophobic` + "\n" +
		`  service["internal/service/&ast;&ast;"]` + "\n" +
		"  classDef endophobic stroke-dasharray: 5 5,stroke-width:2px\n" +
		"  usecase --> service\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !g.IsEndophobic("usecase") {
		t.Fatalf("usecase should be endophobic")
	}
	if g.IsEndophobic("service") {
		t.Fatalf("service should not be endophobic")
	}
}

func TestEncodeNodeId(t *testing.T) {
	cases := []struct {
		raw, encoded string
	}{
		{"src/domain", "src_slash_domain"},
		{"src/model.ts", "src_slash_model_dot_ts"},
		{"my-pkg", "my_dash_pkg"},
		{"internal/api/**", "internal_slash_api_slash__asterisk__asterisk_"},
		{"@scope/pkg", "_atsign_scope_slash_pkg"},
		{"pkg[name]", "pkg_lsqb_name_rsqb_"},
		{"pkg{ver}", "pkg_lbrace_ver_rbrace_"},
		{".", "root"},
		{"123abc", "n123abc"},
		{"Already_Lower", "Already_Lower"},
		{"my+pkg", "my_plus_pkg"},
		{"a?b", "a_qmark_b"},
		{"x,y", "x_comma_y"},
		{"hello world", "hello_space_world"},
		{"a\tb", "a_tab_b"},
		{"a\nb", "a_newline_b"},
		{"a\rb", "a_carriage_return_b"},
		{"a\x0bb", "a_vertical_tab_b"},
		{"a\x0cb", "a_form_feed_b"},
	}
	for _, tc := range cases {
		enc := encodeNodeId(tc.raw)
		if enc != tc.encoded {
			t.Errorf("encodeNodeId(%q) = %q, want %q", tc.raw, enc, tc.encoded)
		}
		if again := encodeNodeId(enc); again != enc {
			t.Errorf("encodeNodeId(%q) is not idempotent: %q", enc, again)
		}
	}
}

func TestEncodeDecodeNodeGlob(t *testing.T) {
	cases := []struct {
		raw, encoded string
	}{
		{"internal/api/**", "internal/api/&ast;&ast;"},
		{"src/model.ts", "src/model.ts"},
		{"my-pkg/**", "my-pkg/&ast;&ast;"},
		{"@scope/pkg/**", "@scope/pkg/&ast;&ast;"},
		{"pkg[name]/**", "pkg[name]/&ast;&ast;"},
		{"pkg{ver}/**", "pkg{ver}/&ast;&ast;"},
	}
	for _, tc := range cases {
		enc := encodeNodeGlob(tc.raw)
		if enc != tc.encoded {
			t.Errorf("encodeNodeGlob(%q) = %q, want %q", tc.raw, enc, tc.encoded)
		}
		dec := decodeNodeGlob(tc.encoded)
		if dec != tc.raw {
			t.Errorf("decodeNodeGlob(%q) = %q, want %q", tc.encoded, dec, tc.raw)
		}
	}
}

func TestSave_DeterministicOutput(t *testing.T) {
	nodes := map[string]string{
		"internal_slash_domain":  "internal/domain",
		"internal_slash_usecase": "internal/usecase",
		"internal_slash_api":     "internal/api",
	}
	edges := map[string]map[string]bool{
		"internal_slash_usecase": {"internal_slash_domain": true},
		"internal_slash_api":     {"internal_slash_usecase": true},
	}
	g := graph.NewGraph(nodes, edges, nil, nil)

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{})

	if !strings.Contains(out, "```mermaid") {
		t.Error("missing mermaid fence")
	}
	if !strings.Contains(out, "flowchart TD") {
		t.Error("missing flowchart TD")
	}
	if !strings.Contains(out, "internal_slash_domain") {
		t.Error("missing internal_domain node")
	}
	if !strings.Contains(out, "internal_slash_usecase") {
		t.Error("missing internal_usecase node")
	}
	if !strings.Contains(out, "internal_slash_api") {
		t.Error("missing internal_api node")
	}
	if !strings.Contains(out, "internal_slash_usecase --> internal_slash_domain") {
		t.Error("missing usecase->domain edge")
	}
	if !strings.Contains(out, "internal_slash_api --> internal_slash_usecase") {
		t.Error("missing api->usecase edge")
	}
}

func TestSave_NoEdges(t *testing.T) {
	g := graph.NewGraph(map[string]string{
		"src_slash_domain": "src/domain",
	}, nil, nil, nil)

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{})
	if !strings.Contains(out, "src_slash_domain") {
		t.Error("missing node in output")
	}
}

func TestMermaidRepository_SaveAddsDirectStyles(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"alpha": "alpha",
			"beta":  "beta",
		},
		Edges: map[string]map[string]bool{
			"alpha": {"beta": true},
		},
		Classes: map[string]map[string]bool{
			"alpha": {"endophobic": true},
		},
	}

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono})

	if !strings.Contains(out, "style alpha stroke:#1f1f1f,stroke-width:2px,stroke-dasharray:5 5") {
		t.Fatalf("missing alpha style line in:\n%s", out)
	}
	if !strings.Contains(out, "style beta stroke:#2a2a2a,stroke-width:2px") {
		t.Fatalf("missing beta style line in:\n%s", out)
	}
	if !strings.Contains(out, "linkStyle 0 stroke:#1f1f1f,stroke-width:2px") {
		t.Fatalf("missing linkStyle line in:\n%s", out)
	}
	if !strings.Contains(out, generatedStyleComment) {
		t.Fatalf("missing generated style comment in:\n%s", out)
	}
	if strings.Index(out, "linkStyle 0") < strings.Index(out, "alpha --> beta") {
		t.Fatalf("expected styles after edges in:\n%s", out)
	}
	if strings.Index(out, generatedStyleComment) < strings.Index(out, "alpha --> beta") {
		t.Fatalf("expected generated style comment after edges in:\n%s", out)
	}
	if strings.Index(out, "style alpha") < strings.Index(out, generatedStyleComment) {
		t.Fatalf("expected generated style comment before style lines in:\n%s", out)
	}
}

func TestMermaidRepository_SaveNoneOnlyStylesEndophobicNodes(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"alpha": "alpha",
			"beta":  "beta",
		},
		Edges: map[string]map[string]bool{
			"alpha": {"beta": true},
		},
		Classes: map[string]map[string]bool{
			"alpha": {"endophobic": true},
		},
	}

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{ColorPalette: port.ColorPaletteNone})

	if !strings.Contains(out, "style alpha stroke-width:2px,stroke-dasharray:5 5") {
		t.Fatalf("missing endophobic style line in:\n%s", out)
	}
	if !strings.Contains(out, generatedStyleComment) {
		t.Fatalf("missing generated style comment in:\n%s", out)
	}
	if strings.Contains(out, "style beta") {
		t.Fatalf("unexpected beta style line in:\n%s", out)
	}
	if strings.Contains(out, "linkStyle ") {
		t.Fatalf("unexpected linkStyle line in:\n%s", out)
	}
}

func TestMermaidRepository_SaveOmitsStyleCommentWithoutStyles(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"alpha": "alpha",
		},
		Edges:   map[string]map[string]bool{},
		Classes: map[string]map[string]bool{},
	}

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{ColorPalette: port.ColorPaletteNone})

	if strings.Contains(out, generatedStyleComment) {
		t.Fatalf("unexpected generated style comment in:\n%s", out)
	}
	if strings.Contains(out, "style alpha") {
		t.Fatalf("unexpected style line in:\n%s", out)
	}
}

func TestMermaidRepository_SaveRepeatsColorsAfterSixteenNodes(t *testing.T) {
	nodes := map[string]string{}
	for i := 0; i < 17; i++ {
		id := string(rune('a' + i))
		nodes[id] = id
	}
	g := &graph.Graph{Nodes: nodes, Edges: map[string]map[string]bool{}, Classes: map[string]map[string]bool{}}

	out := (&MermaidRepository{}).Save(g, port.GraphSaveOptions{ColorPalette: port.ColorPaletteVibrant})

	if !strings.Contains(out, "style a stroke:#0f4cde,stroke-width:2px") {
		t.Fatalf("missing first palette color in:\n%s", out)
	}
	if !strings.Contains(out, "style q stroke:#0f4cde,stroke-width:2px") {
		t.Fatalf("expected repeated palette color for seventeenth node in:\n%s", out)
	}
}

func TestMermaidRepository_LoadIgnoresStyleLines(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  alpha["alpha"]:::endophobic` + "\n" +
		`  beta["beta"]` + "\n" +
		"  alpha --> beta\n" +
		"  style alpha stroke:#1f1f1f,stroke-width:2px,stroke-dasharray:5 5\n" +
		"  style beta stroke:#2a2a2a,stroke-width:2px\n" +
		"  linkStyle 0 stroke:#1f1f1f,stroke-width:2px\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !g.Allows("alpha", "beta") {
		t.Fatalf("expected edge alpha --> beta")
	}
	if !g.IsEndophobic("alpha") {
		t.Fatalf("alpha should remain endophobic")
	}
}

func rawToGraph(nodes map[string]string, edges map[string]map[string]bool) *graph.Graph {
	g := &graph.Graph{
		Nodes:   make(map[string]string, len(nodes)),
		Edges:   make(map[string]map[string]bool, len(edges)),
		Classes: map[string]map[string]bool{},
	}

	for glob, id := range nodes {
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

func TestLoad_AllowsSimpleCycle(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  app["internal/application/&ast;&ast;"]` + "\n" +
		`  domain["internal/domain/&ast;&ast;"]` + "\n" +
		`  app --> domain` + "\n" +
		`  domain --> app` + "\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g == nil {
		t.Fatal("expected parsed graph")
	}
	if !g.Allows("app", "domain") || !g.Allows("domain", "app") {
		t.Fatal("expected cycle edges to be preserved")
	}
	if g.EdgeLines["domain\tapp"] != 6 {
		t.Fatalf("expected edge line metadata for cycle edge, got %d", g.EdgeLines["domain\tapp"])
	}
}

func TestLoad_AllowsMultipleCycles(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a["a/&ast;&ast;"]` + "\n" +
		`  b["b/&ast;&ast;"]` + "\n" +
		`  c["c/&ast;&ast;"]` + "\n" +
		`  d["d/&ast;&ast;"]` + "\n" +
		`  a --> b` + "\n" +
		`  b --> a` + "\n" +
		`  c --> d` + "\n" +
		`  d --> c` + "\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !g.Allows("a", "b") || !g.Allows("b", "a") || !g.Allows("c", "d") || !g.Allows("d", "c") {
		t.Fatal("expected cycle edges to be preserved")
	}
}

func TestCheckCycles_NoCycle(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a["a/&ast;&ast;"]` + "\n" +
		`  b["b/&ast;&ast;"]` + "\n" +
		`  c["c/&ast;&ast;"]` + "\n" +
		`  a --> b` + "\n" +
		`  b --> c` + "\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(g.Nodes))
	}
}

func TestLoad_AllowsLargeCycle(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a["a/&ast;&ast;"]` + "\n" +
		`  b["b/&ast;&ast;"]` + "\n" +
		`  c["c/&ast;&ast;"]` + "\n" +
		`  d["d/&ast;&ast;"]` + "\n" +
		`  a --> b` + "\n" +
		`  b --> c` + "\n" +
		`  c --> d` + "\n" +
		`  d --> a` + "\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !g.Allows("d", "a") {
		t.Fatal("expected closing cycle edge to be preserved")
	}
}

func TestLoad_AllowsEmptyGlobForContractValidation(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a[""]` + "\n" +
		`  b["b/&ast;&ast;"]` + "\n" +
		`  a --> b` + "\n" +
		`  b --> a` + "\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := g.Nodes["a"]; got != "" {
		t.Fatalf("expected empty glob to be preserved for contract validation, got %q", got)
	}
	if !g.Allows("a", "b") || !g.Allows("b", "a") {
		t.Fatal("expected graph to load fully for contract validation")
	}
	if g.EdgeLines["a\tb"] != 5 {
		t.Fatalf("expected edge line metadata to be preserved, got %d", g.EdgeLines["a\tb"])
	}
}

func TestLoad_AllowsUndefinedEdgeNodesForContractValidation(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  app["internal/app/&ast;&ast;"] --> domain` + "\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !g.Allows("app", "domain") {
		t.Fatal("expected edge to be preserved for contract validation")
	}
	if g.EdgeLines["app\tdomain"] != 3 {
		t.Fatalf("expected edge line metadata to be preserved, got %d", g.EdgeLines["app\tdomain"])
	}
}

func TestLoad_Save_PreservesNodeAndEdgeOrder(t *testing.T) {
	repo := &MermaidRepository{}

	md := "```mermaid\nflowchart TD\n" +
		`  z_last["z_last"]` + "\n" +
		`  a_first["a_first"]` + "\n" +
		`  m_mid["m_mid"]` + "\n" +
		"  z_last --> m_mid\n" +
		"  a_first --> z_last\n" +
		"  m_mid --> a_first\n" +
		"```\n"

	g, err := repo.Load(md)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(g.NodeOrder) != 3 {
		t.Fatalf("NodeOrder length: got %d, want 3", len(g.NodeOrder))
	}
	if g.NodeOrder[0] != "z_last" || g.NodeOrder[1] != "a_first" || g.NodeOrder[2] != "m_mid" {
		t.Errorf("NodeOrder = %v, want [z_last a_first m_mid]", g.NodeOrder)
	}
	if len(g.EdgeOrder) != 3 {
		t.Fatalf("EdgeOrder length: got %d, want 3", len(g.EdgeOrder))
	}
	if g.EdgeOrder[0] != "z_last\tm_mid" || g.EdgeOrder[1] != "a_first\tz_last" || g.EdgeOrder[2] != "m_mid\ta_first" {
		t.Errorf("EdgeOrder = %v, want [z_last\\tm_mid a_first\\tz_last m_mid\\ta_first]", g.EdgeOrder)
	}

	saved := repo.Save(g, port.GraphSaveOptions{})

	lines := strings.Split(saved, "\n")
	var nodeOrder []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "z_last[") {
			nodeOrder = append(nodeOrder, "z_last")
		}
		if strings.HasPrefix(trimmed, "a_first[") {
			nodeOrder = append(nodeOrder, "a_first")
		}
		if strings.HasPrefix(trimmed, "m_mid[") {
			nodeOrder = append(nodeOrder, "m_mid")
		}
	}
	if len(nodeOrder) != 3 {
		t.Fatalf("expected 3 node lines in saved output, got %d:\n%s", len(nodeOrder), saved)
	}
	if nodeOrder[0] != "z_last" || nodeOrder[1] != "a_first" || nodeOrder[2] != "m_mid" {
		t.Errorf("saved node order = %v, want [z_last a_first m_mid]", nodeOrder)
	}

	var edgeOrder []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.Contains(trimmed, " --> ") && !strings.HasPrefix(trimmed, "linkStyle") {
			edgeOrder = append(edgeOrder, trimmed)
		}
	}
	if len(edgeOrder) != 3 {
		t.Fatalf("expected 3 edge lines in saved output, got %d:\n%s", len(edgeOrder), saved)
	}
	if edgeOrder[0] != "z_last --> m_mid" || edgeOrder[1] != "a_first --> z_last" || edgeOrder[2] != "m_mid --> a_first" {
		t.Errorf("saved edge order = %v, want [z_last --> m_mid a_first --> z_last m_mid --> a_first]", edgeOrder)
	}
}

func TestLoad_Save_RoundTripDeterministic(t *testing.T) {
	repo := &MermaidRepository{}

	md := "```mermaid\nflowchart TD\n" +
		`  z_last["z_last"]` + "\n" +
		`  a_first["a_first"]` + "\n" +
		`  m_mid["m_mid"]` + "\n" +
		"  z_last --> a_first\n" +
		"```\n"

	g, err := repo.Load(md)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var results []string
	current := g
	for i := 0; i < 5; i++ {
		saved := repo.Save(current, port.GraphSaveOptions{})
		results = append(results, saved)
		current, err = repo.Load(saved)
		if err != nil {
			t.Fatalf("round %d Load: %v", i, err)
		}
	}

	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Errorf("round %d produced different output than round 0", i)
		}
	}
}

func TestCheckCycles_SelfCycle(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a["a/&ast;&ast;"]` + "\n" +
		`  a --> a` + "\n" +
		"```\n"

	_, err := (&MermaidRepository{}).Load(md)
	if err == nil {
		t.Fatal("expected error for self-cycle")
	}
	// Self-cycles are caught at parse time before cycle detection.
	if got := err.Error(); !strings.Contains(got, "same node") {
		t.Fatalf("expected same-node error, got: %q", got)
	}
}

func loadLines(lines ...string) (*graph.Graph, error) {
	return (&MermaidRepository{}).Load("```mermaid\nflowchart TD\n" + strings.Join(lines, "\n") + "\n```\n")
}

func TestLoad_EdgeSyntax(t *testing.T) {
	nodes := []string{`a["a/&ast;&ast;"]`, `b["b/&ast;&ast;"]`, `c["c/&ast;&ast;"]`, `d["d/&ast;&ast;"]`}
	cases := []struct {
		name  string
		line  string
		edges []string
	}{
		{"plain", "a --> b", []string{"a b"}},
		{"long arrow", "a ----> b", []string{"a b"}},
		{"thick", "a ==> b", []string{"a b"}},
		{"dotted", "a -.-> b", []string{"a b"}},
		{"long dotted", "a -..-> b", []string{"a b"}},
		{"pipe label", "a -->|calls| b", []string{"a b"}},
		{"dotted pipe label", "a -.->|calls| b", []string{"a b"}},
		{"inline label", "a -- calls --> b", []string{"a b"}},
		{"dotted inline label", "a -. calls .-> b", []string{"a b"}},
		{"thick inline label", "a == calls ==> b", []string{"a b"}},
		{"trailing semicolon", "a --> b;", []string{"a b"}},
		{"trailing semicolon and spaces", "a --> b ; ", []string{"a b"}},
		{"fan out", "a --> b & c", []string{"a b", "a c"}},
		{"fan in", "a & b --> c", []string{"a c", "b c"}},
		{"fan both ways", "a & b --> c & d", []string{"a c", "a d", "b c", "b d"}},
		{"chained", "a --> b --> c", []string{"a b", "b c"}},
		{"chained with labels", "a -->|x| b -- y --> c", []string{"a b", "b c"}},
		{"chained fan out", "a --> b & c --> d", []string{"a b", "a c", "b d", "c d"}},
		{"inline node in fan out", `a --> b & d["d/&ast;&ast;"]`, []string{"a b", "a d"}},
		{"label containing an arrow", "a -->|a --> c| b", []string{"a b"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := loadLines(append(append([]string{}, nodes...), tc.line)...)
			if err != nil {
				t.Fatalf("load %q: %v", tc.line, err)
			}
			var got []string
			for src, dsts := range g.Edges {
				for dst := range dsts {
					got = append(got, src+" "+dst)
				}
			}
			sort.Strings(got)
			if strings.Join(got, ", ") != strings.Join(tc.edges, ", ") {
				t.Errorf("edges: got %v, want %v", got, tc.edges)
			}
		})
	}
}

func TestLoad_EdgeSyntaxErrors(t *testing.T) {
	cases := []struct{ name, line, wantMsg string }{
		{name: "missing source", line: "--> b"},
		{name: "missing target", line: "a -->"},
		{name: "empty fan member", line: "a --> b &"},
		{name: "two ids in one group", line: "a --> b c"},
		{name: "undirected link", line: "a --- b", wantMsg: "undirected link"},
		{name: "thick undirected link", line: "a === b", wantMsg: "undirected link"},
		{name: "invisible link", line: "a ~~~ b", wantMsg: "invisible link"},
		{name: "circle headed link", line: "a --o b", wantMsg: "circle- or cross-headed link"},
		{name: "cross headed link", line: "a --x b", wantMsg: "circle- or cross-headed link"},
		{name: "bidirectional link", line: "a <--> b", wantMsg: "bidirectional link"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadLines(`a["a/&ast;&ast;"]`, `b["b/&ast;&ast;"]`, tc.line)
			if err == nil {
				t.Fatalf("expected error for %q", tc.line)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg+` in "`+tc.line+`"`) {
				t.Errorf("error %q does not name the construct %q", err, tc.wantMsg)
			}
		})
	}
}

func TestLoad_EdgeFromNodeNamedAfterAHeaderKeyword(t *testing.T) {
	g, err := loadLines(`graph["graph/&ast;&ast;"]`, `b["b/&ast;&ast;"]`, "graph --> b")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !g.Allows("graph", "b") {
		t.Errorf("edge missing: %v", g.Edges)
	}
}

func TestLoad_KeepsHandWrittenNodeIds(t *testing.T) {
	g, err := loadLines(`  n8n["n8n/&ast;&ast;"];`, `  my_dot_pkg["pkg/&ast;&ast;"]`, "  n8n --> my_dot_pkg")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, id := range []string{"n8n", "my_dot_pkg"} {
		if _, ok := g.Nodes[id]; !ok {
			t.Errorf("node %q was rewritten: %v", id, g.Nodes)
		}
	}
	if !g.Allows("n8n", "my_dot_pkg") {
		t.Errorf("edge missing: %v", g.Edges)
	}
}

func TestLoad_GraphHeaderAndConfig(t *testing.T) {
	g, err := (&MermaidRepository{}).Load("```mermaid\ngraph LR\n" +
		"  %% config namespaceMode 'True'\n" +
		`  a["a/&ast;&ast;"]` + "\n```\n")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !g.NamespaceMode {
		t.Error("namespaceMode should be enabled")
	}
}

func TestParseConfigLine(t *testing.T) {
	cases := []struct {
		name, line string
		wantSep    string
		wantNS     bool
		wantErr    string
	}{
		{name: "double quoted separator", line: `config globSeparator "."`, wantSep: "."},
		{name: "single quoted separator", line: `config globSeparator '.'`, wantSep: "."},
		{name: "bare separator", line: `config globSeparator .`, wantSep: "."},
		{name: "extra whitespace", line: "config   globSeparator   '.'", wantSep: "."},
		{name: "namespace mode true", line: `config namespaceMode "true"`, wantNS: true},
		{name: "namespace mode mixed case", line: `config namespaceMode "True"`, wantNS: true},
		{name: "namespace mode bare upper", line: `config namespaceMode TRUE`, wantNS: true},
		{name: "namespace mode false", line: `config namespaceMode 'false'`},
		{name: "missing key", line: "config", wantErr: `config directive is missing a key`},
		{name: "missing value", line: "config namespaceMode", wantErr: `config key "namespaceMode" is missing a value`},
		{name: "unknown key", line: `config nope "1"`, wantErr: `unknown config key "nope"`},
		{name: "unterminated quote", line: `config globSeparator "abc`, wantErr: `config key "globSeparator" has an unterminated quoted value`},
		{name: "empty separator", line: `config globSeparator ""`, wantErr: `config key "globSeparator" must not be empty`},
		{name: "non boolean", line: `config namespaceMode "yes"`, wantErr: `config key "namespaceMode" expects true or false, got "yes"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &graph.Graph{}
			err := parseConfigLine(tc.line, g, 7)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error: got %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if g.GlobSeparator != tc.wantSep {
				t.Errorf("globSeparator: got %q, want %q", g.GlobSeparator, tc.wantSep)
			}
			if g.NamespaceMode != tc.wantNS {
				t.Errorf("namespaceMode: got %v, want %v", g.NamespaceMode, tc.wantNS)
			}
		})
	}
}

const richContract = "# Payments\n" +
	"\n" +
	"Storage is an implementation detail; the API must not reach into it.\n" +
	"\n" +
	"```mermaid\n" +
	"flowchart LR\n" +
	"  %% API must never touch storage\n" +
	`  api["internal/api"]` + "\n" +
	`  storage["internal/storage"]:::endophobic` + "\n" +
	"\n" +
	"  api --> storage\n" +
	"```\n" +
	"\n" +
	"See ADR-7 for the rationale.\n"

func TestRestylePreservesEverythingOutsideStyleTail(t *testing.T) {
	out, err := (&MermaidRepository{}).Restyle(richContract, port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono})
	if err != nil {
		t.Fatalf("Restyle: %v", err)
	}

	prefix := richContract[:strings.Index(richContract, "  api --> storage")+len("  api --> storage")]
	if !strings.HasPrefix(out, prefix) {
		t.Fatalf("prose, direction, comments and nodes must survive verbatim, got:\n%s", out)
	}
	for _, want := range []string{
		generatedStyleComment,
		"  style api stroke:#1f1f1f,stroke-width:2px\n",
		"  style storage stroke:#2a2a2a,stroke-width:2px,stroke-dasharray:5 5\n",
		"  linkStyle 0 stroke:#1f1f1f,stroke-width:2px\n",
		"```\n\nSee ADR-7 for the rationale.\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRestyleReplacesStaleStyleTail(t *testing.T) {
	repo := &MermaidRepository{}
	styled, err := repo.Restyle(richContract, port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono})
	if err != nil {
		t.Fatalf("Restyle: %v", err)
	}

	again, err := repo.Restyle(styled, port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono})
	if err != nil {
		t.Fatalf("Restyle second pass: %v", err)
	}
	if again != styled {
		t.Fatalf("restyle is not idempotent:\n%s", again)
	}

	dropped, err := repo.Restyle(styled, port.GraphSaveOptions{ColorPalette: port.ColorPaletteNone})
	if err != nil {
		t.Fatalf("Restyle none: %v", err)
	}
	if strings.Contains(dropped, "linkStyle") || strings.Contains(dropped, "stroke:#") {
		t.Fatalf("stale palette styling survived:\n%s", dropped)
	}
	if !strings.Contains(dropped, "  style storage stroke-width:2px,stroke-dasharray:5 5\n") {
		t.Fatalf("missing endophobic styling in:\n%s", dropped)
	}
}

func TestRestyleWithoutStylingLeavesContractUntouched(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" + `  api["internal/api"]` + "\n\n```\n"

	out, err := (&MermaidRepository{}).Restyle(md, port.GraphSaveOptions{ColorPalette: port.ColorPaletteNone})
	if err != nil {
		t.Fatalf("Restyle: %v", err)
	}
	if out != md {
		t.Fatalf("expected byte-identical output, got:\n%s", out)
	}
}

func TestRestyleNumbersLinksAsDeclared(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  a["a"]` + "\n" +
		`  b["b"]` + "\n" +
		`  c["c"]` + "\n" +
		"\n  a --> b --> c\n  a --> b\n  a -.-> c\n  a & b --> c\n```\n"

	out, err := (&MermaidRepository{}).Restyle(md, port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono})
	if err != nil {
		t.Fatalf("Restyle: %v", err)
	}
	for _, want := range []string{
		"  a --> b --> c\n  a --> b\n  a -.-> c\n  a & b --> c\n",
		"  linkStyle 0,2,4 stroke:#1f1f1f,stroke-width:2px\n",
		"  linkStyle 1,5 stroke:#2a2a2a,stroke-width:2px\n",
		"  linkStyle 3 stroke:#1f1f1f,stroke-width:2px,stroke-dasharray:5 5\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRestyleKeepsAuthoredClassDefAndGlobs(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  %% config globSeparator "."` + "\n" +
		"  classDef highlight fill:#eee\n" +
		`  api["src.api"]:::highlight` + "\n" +
		`  db["src.db"]` + "\n" +
		"\n  api --> db\n```\n"

	out, err := (&MermaidRepository{}).Restyle(md, port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono})
	if err != nil {
		t.Fatalf("Restyle: %v", err)
	}
	if !strings.HasPrefix(out, md[:strings.Index(md, "\n  api --> db")]) {
		t.Fatalf("classDef, config and globs must survive verbatim, got:\n%s", out)
	}
	if !strings.Contains(out, "  style api stroke:#1f1f1f,stroke-width:2px\n") {
		t.Fatalf("missing generated styling in:\n%s", out)
	}
}

func TestRestyleKeepsCRLFLineEndings(t *testing.T) {
	md := strings.ReplaceAll(richContract, "\n", "\r\n")

	out, err := (&MermaidRepository{}).Restyle(md, port.GraphSaveOptions{ColorPalette: port.ColorPaletteMono})
	if err != nil {
		t.Fatalf("Restyle: %v", err)
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Fatalf("generated lines must keep CRLF endings, got:\n%q", out)
	}
	if !strings.Contains(out, "  linkStyle 0 stroke:#1f1f1f,stroke-width:2px\r\n") {
		t.Fatalf("missing generated styling in:\n%q", out)
	}
}

func TestRestyleReturnsParseError(t *testing.T) {
	for content, want := range map[string]string{
		"no fence here\n": "no ```mermaid block found",
		"```mermaid\nflowchart TD\n  a[\"a\"]\n  ?!?\n```\n": "unrecognized mermaid line: ?!? (line 4)",
	} {
		_, err := (&MermaidRepository{}).Restyle(content, port.GraphSaveOptions{})
		if err == nil || err.Error() != want {
			t.Fatalf("Restyle(%q) = %v, want %q", content, err, want)
		}
	}
}

func TestMermaidRepository_LoadDottedEdgeIsTolerated(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  app["app"]` + "\n" +
		`  domain["domain"]` + "\n" +
		`  legacy["legacy"]` + "\n" +
		"  app --> domain -.-> legacy\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !g.Allows("app", "domain") || g.IsTolerated("app", "domain") {
		t.Errorf("app --> domain should be allowed and not tolerated")
	}
	if !g.Allows("domain", "legacy") || !g.IsTolerated("domain", "legacy") {
		t.Errorf("domain -.-> legacy should be allowed and tolerated")
	}
	if g.EdgeLines["domain\tlegacy"] != 6 {
		t.Errorf("edge line = %d, want 6", g.EdgeLines["domain\tlegacy"])
	}
}

func TestMermaidRepository_LoadSolidDeclarationPromotesDottedEdge(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  app["app"]` + "\n" +
		`  legacy["legacy"]` + "\n" +
		"  app -.-> legacy\n" +
		"  app --> legacy\n" +
		"```\n"

	g, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if g.IsTolerated("app", "legacy") {
		t.Errorf("a solid declaration should promote the dotted edge")
	}
}

func TestMermaidRepository_SaveDottedEdgeRoundTrips(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  app["app"]` + "\n" +
		`  domain["domain"]` + "\n" +
		`  legacy["legacy"]` + "\n" +
		"  app --> domain\n" +
		"  app -.-> legacy\n" +
		"```\n"

	repo := &MermaidRepository{}
	g, err := repo.Load(md)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	saved := repo.Save(g, port.GraphSaveOptions{})
	if !strings.Contains(saved, "app -.-> legacy") {
		t.Fatalf("saved contract lost the dotted edge:\n%s", saved)
	}
	if !strings.Contains(saved, "app --> domain") {
		t.Fatalf("saved contract lost the solid edge:\n%s", saved)
	}
	if !strings.Contains(saved, "stroke-dasharray:5 5") {
		t.Errorf("tolerated edge should be styled dashed:\n%s", saved)
	}

	reloaded, err := repo.Load(saved)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.IsTolerated("app", "legacy") || reloaded.IsTolerated("app", "domain") {
		t.Errorf("toleration did not round-trip: %v", reloaded.Tolerated)
	}
}
