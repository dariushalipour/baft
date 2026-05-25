package graph

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestIsEndophobic(t *testing.T) {
	tests := []struct {
		label   string
		classes map[string]map[string]bool
		node    string
		want    bool
	}{
		{
			label:   "missing node is not endophobic",
			classes: map[string]map[string]bool{},
			node:    "missing",
			want:    false,
		},
		{
			label: "endophobic class is true",
			classes: map[string]map[string]bool{
				"core": {"endophobic": true},
			},
			node: "core",
			want: true,
		},
		{
			label: "endophobic class is false",
			classes: map[string]map[string]bool{
				"api": {"endophobic": false},
			},
			node: "api",
			want: false,
		},
		{
			label: "missing class key is false",
			classes: map[string]map[string]bool{
				"api": {"other": true},
			},
			node: "api",
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			g := &Graph{Classes: tc.classes}
			if got := g.IsEndophobic(tc.node); got != tc.want {
				t.Errorf("IsEndophobic(%q) = %v, want %v", tc.node, got, tc.want)
			}
		})
	}
}

func TestNodeForDir_MostSpecificWins(t *testing.T) {
	g := &Graph{
		Nodes: map[string]string{
			"main":          ".",
			"httpapi":       "internal/adapter/inbound/httpapi/**",
			"adapter":       "internal/adapter/**",
			"usecase":       "internal/usecase/**",
			"domain":        "internal/domain/**",
			"valueobject":   "internal/valueobject/**",
			"infra_port":    "internal/infra/*",
			"infra_adapter": "internal/infra/*/**",
			"migration":     "internal/migration/**",
		},
	}
	gi := NewGraphIndex(g)
	tests := map[string]string{
		".":                                  "main",
		"internal/adapter/inbound/httpapi":   "httpapi",
		"internal/adapter/inbound/httpapi/x": "httpapi",
		"internal/usecase":                   "usecase",
		"internal/usecase/sub":               "usecase",
		"internal/domain":                    "domain",
		"internal/valueobject":               "valueobject",
		"internal/infra/clock":               "infra_port",
		"internal/infra/clock/clock_dev":     "infra_adapter",
		"internal/infra/repository/sub/deep": "infra_adapter",
		"internal/migration":                 "migration",
		"internal/migration/sql":             "migration",
		"internal/not_a_node":                "",
	}
	for dirPath, want := range tests {
		if got := gi.NodeForDir(dirPath); got != want {
			t.Errorf("NodeForDir(%q) = %q, want %q", dirPath, got, want)
		}
	}
}

func TestNodeForDir_EmptyPathIsRoot(t *testing.T) {
	g := &Graph{Nodes: map[string]string{"root": "."}}
	gi := NewGraphIndex(g)
	if got := gi.NodeForDir(""); got != "root" {
		t.Errorf("NodeForDir(\"\") = %q, want root", got)
	}
}

func TestMatchDirGlob(t *testing.T) {
	tests := []struct {
		pattern, dir string
		want         bool
	}{
		{".", ".", true},
		{".", "internal/x", false},
		{"internal/usecase/**", "internal/usecase", true},
		{"internal/usecase/**", "internal/usecase/sub/deep", true},
		{"internal/usecase/**", "internal/service", false},
		{"internal/infra/*", "internal/infra/clock", true},
		{"internal/infra/*", "internal/infra/clock/clock_dev", false},
		{"internal/infra/*/**", "internal/infra/clock", false},
		{"internal/infra/*/**", "internal/infra/clock/clock_dev", true},
	}
	for _, tc := range tests {
		if got := MatchDirGlob(tc.pattern, tc.dir); got != tc.want {
			t.Errorf("MatchDirGlob(%q, %q) = %v, want %v", tc.pattern, tc.dir, got, tc.want)
		}
	}
}

func TestMatchSegment(t *testing.T) {
	tests := []struct {
		pattern, text string
		want          bool
	}{
		{"foo", "foo", true},
		{"foo", "bar", false},
		{"*oo", "foo", true},
		{"f*", "foo", true},
		{"f*o", "food", false},
		{"f*m", "foobarbam", true},
		{"f*m", "foobaz", false},
		{"*", "anything", true},
	}
	for _, tc := range tests {
		if got := MatchSegment(tc.pattern, tc.text); got != tc.want {
			t.Errorf("MatchSegment(%q, %q) = %v, want %v", tc.pattern, tc.text, got, tc.want)
		}
	}
}

func TestGlobSpecificity(t *testing.T) {
	tests := map[string]int{
		".":              10,
		"internal":       10,
		"internal/**":    11,
		"internal/*":     13,
		"internal/core":  20,
		"internal/*/**":  14,
		"lib/src/*.dart": 23,
	}
	for pattern, want := range tests {
		if got := GlobSpecificity(pattern); got != want {
			t.Errorf("GlobSpecificity(%q) = %d, want %d", pattern, got, want)
		}
	}
}

func TestIsFileGlob(t *testing.T) {
	tests := map[string]bool{
		".":                      false,
		"":                       false,
		"lib":                    false,
		"lib/src":                false,
		"lib/src/**":             false,
		"lib/src/*":              false,
		"lib/src/providers.dart": true,
		"lib/src/*.dart":         true,
		"main.go":                true,
		"lib/..":                 false,
	}
	for pattern, want := range tests {
		if got := IsFileGlob(pattern); got != want {
			t.Errorf("IsFileGlob(%q) = %v, want %v", pattern, got, want)
		}
	}
}

func TestNodeForPath_FileGlobBeatsDirGlob(t *testing.T) {
	g := &Graph{
		Nodes: map[string]string{
			"providers": "lib/src/providers.dart",
			"root":      "lib/src",
			"subtree":   "lib/src/**",
		},
	}
	gi := NewGraphIndex(g)
	tests := map[string]string{
		"lib/src/providers.dart":   "providers",
		"lib/src/other.dart":       "subtree",
		"lib/src/widgets/foo.dart": "subtree",
		"lib/src":                  "subtree",
	}
	for path, want := range tests {
		if got := gi.NodeForPath(path); got != want {
			t.Errorf("NodeForPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestNodeForPath_WildcardFileGlob(t *testing.T) {
	g := &Graph{
		Nodes: map[string]string{
			"anyDart": "lib/src/*.dart",
			"subtree": "lib/src/**",
		},
	}
	gi := NewGraphIndex(g)
	tests := map[string]string{
		"lib/src/foo.dart":        "anyDart",
		"lib/src/nested/foo.dart": "subtree",
	}
	for path, want := range tests {
		if got := gi.NodeForPath(path); got != want {
			t.Errorf("NodeForPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestNodeForPath_EmptyPath(t *testing.T) {
	g := &Graph{Nodes: map[string]string{"root": "."}}
	gi := NewGraphIndex(g)
	if got := gi.NodeForPath(""); got != "root" {
		t.Errorf("NodeForPath(\"\") = %q, want root", got)
	}
}

func TestFileGlobNodes(t *testing.T) {
	g := &Graph{
		Nodes: map[string]string{
			"zeta":  "lib/src/zeta.dart",
			"root":  "lib/src",
			"alpha": "lib/src/alpha.dart",
		},
	}
	gi := NewGraphIndex(g)
	got := gi.FileGlobNodes()
	want := []string{"alpha", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAllows(t *testing.T) {
	g := &Graph{
		Nodes: map[string]string{"a": ".", "b": "lib", "c": "lib/src"},
		Edges: map[string]map[string]bool{
			"a": {"b": true},
			"b": {"c": true},
		},
	}
	tests := []struct {
		src, dst string
		want     bool
	}{
		{"a", "a", true},
		{"a", "b", true},
		{"b", "c", true},
		{"a", "c", false},
		{"c", "a", false},
	}
	for _, tc := range tests {
		if got := g.Allows(tc.src, tc.dst); got != tc.want {
			t.Errorf("Allows(%q, %q) = %v, want %v", tc.src, tc.dst, got, tc.want)
		}
	}
}

func TestNodeKeyForDir(t *testing.T) {
	tests := []struct {
		path, want string
	}{
		{"internal/domain/model.go", "internal/domain"},
		{"internal/domain", "internal/domain"},
		{"main.go", "."},
		{"src/api/handler.ts", "src/api"},
	}
	for _, tc := range tests {
		if got := NodeKeyForDir(tc.path); got != tc.want {
			t.Errorf("NodeKeyForDir(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestNodeKeyForFile(t *testing.T) {
	tests := []struct {
		path, want string
	}{
		{"lib/src/auth_service.dart", "lib/src/auth_service.dart"},
		{"lib/main.dart", "lib/main.dart"},
		{"lib/src/providers.dart", "lib/src/providers.dart"},
	}
	for _, tc := range tests {
		if got := NodeKeyForFile(tc.path); got != tc.want {
			t.Errorf("NodeKeyForFile(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestNewGraph_EdgeCount(t *testing.T) {
	g := NewGraph(nil, map[string]map[string]bool{}, nil, nil)
	if got := NewGraphIndex(g).EdgeCount(); got != 0 {
		t.Errorf("EdgeCount(empty) = %d, want 0", got)
	}

	g = NewGraph(nil, map[string]map[string]bool{"a": {"b": true}}, nil, nil)
	if got := NewGraphIndex(g).EdgeCount(); got != 1 {
		t.Errorf("EdgeCount(single) = %d, want 1", got)
	}

	g = NewGraph(nil, map[string]map[string]bool{
		"a": {"b": true},
		"b": {"c": true},
		"c": {"a": true},
	}, nil, nil)
	if got := NewGraphIndex(g).EdgeCount(); got != 3 {
		t.Errorf("EdgeCount(three) = %d, want 3", got)
	}
}

func TestValidate(t *testing.T) {
	g := &Graph{
		Nodes: map[string]string{
			"clean":  "internal/core/**",
			"dotdot": "internal/../../etc",
		},
	}
	errs := g.Validate()
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidate_CleanGraph(t *testing.T) {
	g := &Graph{
		Nodes: map[string]string{
			"core": "internal/core/**",
			"api":  "internal/api/**",
		},
	}
	if errs := g.Validate(); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestGlobsOverlap(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"internal/core/**", "internal/core/**", true},
		{"internal/core/**", "internal/api/**", false},
		{"internal/**", "internal/core/**", true},
		{"internal/core/**", "internal/core", true},
		{"internal/core", "internal/api", false},
		{"internal/*", "internal/core", true},
		{"internal/*", "internal/*", true},
		{"lib/src/*.dart", "lib/src/*.dart", true},
		{"lib/src/*.dart", "lib/src/**", false},
	}
	for _, tc := range tests {
		if got := GlobsOverlap(tc.a, tc.b); got != tc.want {
			t.Errorf("GlobsOverlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPairCanOverlap(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"foo", "foo", true},
		{"foo", "bar", false},
		{"f*", "foo", true},
		{"f*", "bar", false},
		{"*o", "foo", true},
		{"*", "anything", true},
		{"f*m", "foobarbam", true},
		{"f*m", "foobaz", false},
	}
	for _, tc := range tests {
		if got := pairCanOverlap(tc.a, tc.b); got != tc.want {
			t.Errorf("pairCanOverlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestDirOf(t *testing.T) {
	tests := map[string]string{
		"internal/domain/model.go": "internal/domain",
		"main.go":                  ".",
		"internal/domain":          "internal",
	}
	for path, want := range tests {
		if got := DirOf(path); got != want {
			t.Errorf("DirOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestMatchFileGlob(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"lib/src/*.dart", "lib/src/foo.dart", true},
		{"lib/src/*.dart", "lib/src/nested/foo.dart", false},
		{"lib/src/*.dart", ".", false},
		{"lib/src/*.dart", "", false},
		{"main.go", "main.go", true},
		{"main.go", "other.go", false},
	}
	for _, tc := range tests {
		if got := MatchFileGlob(tc.pattern, tc.path); got != tc.want {
			t.Errorf("MatchFileGlob(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestNodeForDir_ConcurrentNoRedundantWork(t *testing.T) {
	nodes := make(map[string]string, 500)
	for i := 0; i < 500; i++ {
		nodes[fmt.Sprintf("node_%d", i)] = fmt.Sprintf("pkg/layer_%d/**", i)
	}
	g := &Graph{Nodes: nodes}
	gi := NewGraphIndex(g)
	var computeCount int64
	gi.onCompute = func() { atomic.AddInt64(&computeCount, 1) }

	const goroutines = 100
	var barrier sync.WaitGroup
	barrier.Add(goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			barrier.Done()
			barrier.Wait()
			_ = gi.NodeForDir("pkg/layer_0/sub")
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&computeCount); got > 1 {
		t.Errorf("NodeForDir: computation ran %d times, want <= 1 (double-check should prevent thundering herd)", got)
	}
}

func TestNodeForPath_ConcurrentNoRedundantWork(t *testing.T) {
	nodes := make(map[string]string, 500)
	for i := 0; i < 500; i++ {
		nodes[fmt.Sprintf("node_%d", i)] = fmt.Sprintf("pkg/layer_%d/*.go", i)
	}
	g := &Graph{Nodes: nodes}
	gi := NewGraphIndex(g)
	var computeCount int64
	gi.onCompute = func() { atomic.AddInt64(&computeCount, 1) }

	const goroutines = 100
	var barrier sync.WaitGroup
	barrier.Add(goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			barrier.Done()
			barrier.Wait()
			_ = gi.NodeForPath("pkg/layer_0/main.go")
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&computeCount); got > 1 {
		t.Errorf("NodeForPath: computation ran %d times, want <= 1 (double-check should prevent thundering herd)", got)
	}
}

func TestNewGraphIndex_DeterministicOrder(t *testing.T) {
	nodes := map[string]string{
		"z_last":  "pkg/last/**",
		"a_first": "pkg/first/**",
		"m_mid":   "pkg/mid/**",
	}
	g := &Graph{
		Nodes:     nodes,
		NodeOrder: []string{"a_first", "m_mid", "z_last"},
	}
	gi := NewGraphIndex(g)

	ordered := append([]string{}, gi.dirNodes...)
	if len(ordered) != 3 {
		t.Fatalf("expected 3 dir nodes, got %d", len(ordered))
	}
	if ordered[0] != "a_first" || ordered[1] != "m_mid" || ordered[2] != "z_last" {
		t.Errorf("dirNodes order = %v, want [a_first m_mid z_last]", ordered)
	}
}

func TestNewGraphIndex_DeterministicOrder_Fallback(t *testing.T) {
	nodes := map[string]string{
		"z_last":  "pkg/last/**",
		"a_first": "pkg/first/**",
		"m_mid":   "pkg/mid/**",
	}
	g := &Graph{
		Nodes: nodes,
		NodeLines: map[string]int{
			"z_last":  10,
			"a_first": 1,
			"m_mid":   5,
		},
	}
	gi := NewGraphIndex(g)

	ordered := append([]string{}, gi.dirNodes...)
	if len(ordered) != 3 {
		t.Fatalf("expected 3 dir nodes, got %d", len(ordered))
	}
	if ordered[0] != "a_first" || ordered[1] != "m_mid" || ordered[2] != "z_last" {
		t.Errorf("dirNodes order = %v, want [a_first m_mid z_last]", ordered)
	}
}

func TestNewGraph_AlphabeticalNodeAndEdgeOrder(t *testing.T) {
	nodes := map[string]string{
		"z_last":  "pkg/last/**",
		"a_first": "pkg/first/**",
		"m_mid":   "pkg/mid/**",
	}
	edges := map[string]map[string]bool{
		"z_last":  {"a_first": true},
		"a_first": {"m_mid": true, "z_last": true},
		"m_mid":   {"a_first": true},
	}
	g := NewGraph(nodes, edges, nil, nil)

	if g.NodeOrder[0] != "a_first" || g.NodeOrder[1] != "m_mid" || g.NodeOrder[2] != "z_last" {
		t.Errorf("NodeOrder = %v, want [a_first m_mid z_last]", g.NodeOrder)
	}

	wantEdges := []string{
		"a_first\tm_mid",
		"a_first\tz_last",
		"m_mid\ta_first",
		"z_last\ta_first",
	}
	if len(g.EdgeOrder) != len(wantEdges) {
		t.Fatalf("EdgeOrder length: got %d, want %d", len(g.EdgeOrder), len(wantEdges))
	}
	for i, want := range wantEdges {
		if g.EdgeOrder[i] != want {
			t.Errorf("EdgeOrder[%d] = %q, want %q", i, g.EdgeOrder[i], want)
		}
	}
}
