package mermaid

import (
	"strings"
	"testing"

	"github.com/dariushalipour/baft/internal/domain/graph"
)

func TestMermaidRepository_Load(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  main["."]` + "\n" +
		`  api["internal/api/**"]` + "\n" +
		`  domain["internal/domain/**"]` + "\n" +
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

func TestMermaidRepository_LoadEndophobicClass(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  uc["internal/usecase/**"]:::endophobic` + "\n" +
		`  svc["internal/service/**"]` + "\n" +
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

	out := (&MermaidRepository{}).Save(g)

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

func TestMermaidRepository_SaveDirGlobSuffix(t *testing.T) {
	g := &graph.Graph{
		Nodes: map[string]string{
			"api":    "internal/api",
			"domain": "internal/domain/model.ts",
		},
		Edges:   map[string]map[string]bool{},
		Classes: map[string]map[string]bool{},
	}

	out := (&MermaidRepository{}).Save(g)

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

	out := (&MermaidRepository{}).Save(g)

	aIdx := strings.Index(out, "  a[")
	mIdx := strings.Index(out, "  m[")
	zIdx := strings.Index(out, "  z[")
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("nodes not sorted: a=%d m=%d z=%d", aIdx, mIdx, zIdx)
	}
}

func TestRoundTrip_LoadSaveLoad(t *testing.T) {
	md := "```mermaid\nflowchart TD\n" +
		`  main["."]` + "\n" +
		`  api["internal/api/**"]` + "\n" +
		`  domain["internal/domain/**"]` + "\n" +
		`  usecase["internal/usecase/**"]:::endophobic` + "\n" +
		"  classDef endophobic stroke-dasharray: 5 5\n" +
		"  main --> api --> usecase --> domain\n" +
		"  main --> usecase\n" +
		"```\n"

	original, err := (&MermaidRepository{}).Load(md)
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}

	saved := (&MermaidRepository{}).Save(original)
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

	origEdges := original.EdgeCount()
	rtEdges := roundTrip.EdgeCount()
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
	saved := (&MermaidRepository{}).Save(graph)
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
		if roundTrip.Nodes[id] != want {
			t.Errorf("node %q: got %q, want %q", id, roundTrip.Nodes[id], want)
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

			saved := (&MermaidRepository{}).Save(g)
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

			saved := (&MermaidRepository{}).Save(g)
			loaded, err := (&MermaidRepository{}).Load(saved)
			if err != nil {
				t.Fatalf("load after save: %v\nsaved:\n%s", err, saved)
			}
			if loaded.Nodes[tc.id] != tc.glob {
				t.Errorf("round-trip mismatch: got %q, want %q\nsaved:\n%s", loaded.Nodes[tc.id], tc.glob, saved)
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

	saved := (&MermaidRepository{}).Save(g)
	loaded, err := (&MermaidRepository{}).Load(saved)
	if err != nil {
		t.Fatalf("load after save: %v\nsaved:\n%s", err, saved)
	}

	if !loaded.Allows("@scope/pkg", "src/domain") {
		t.Error("missing edge @scope/pkg --> src/domain")
	}
	if !loaded.Allows("my-pkg[ver]", "@scope/pkg") {
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

	saved := (&MermaidRepository{}).Save(g)

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
		`  app["internal/application/**"] --> domain["internal/domain/**"]` + "\n" +
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
		`  httpapi["internal/adapter/inbound/httpapi/**"]` + "\n" +
		`  usecase["internal/usecase/**"]` + "\n" +
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
		`  a["internal/x/**"]` + "\n" +
		`  b["internal/x/**"]` + "\n" +
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
		`  usecase["internal/usecase/**"]:::endophobic` + "\n" +
		`  service["internal/service/**"]` + "\n" +
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

func TestEncodeDecodeNodeId(t *testing.T) {
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
	}
	for _, tc := range cases {
		enc := encodeNodeId(tc.raw)
		if enc != tc.encoded {
			t.Errorf("encodeNodeId(%q) = %q, want %q", tc.raw, enc, tc.encoded)
		}
		dec := decodeNodeId(tc.encoded)
		if dec != tc.raw {
			t.Errorf("decodeNodeId(%q) = %q, want %q", tc.encoded, dec, tc.raw)
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
	g := graph.NewGraph(nodes, edges)

	out := (&MermaidRepository{}).Save(g)

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
	}, nil)

	out := (&MermaidRepository{}).Save(g)
	if !strings.Contains(out, "src_slash_domain") {
		t.Error("missing node in output")
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
