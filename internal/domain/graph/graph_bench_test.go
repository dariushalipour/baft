package graph_test

import (
	"path/filepath"
	"testing"

	"github.com/dariushalipour/baft/internal/domain/graph"
)

// benchmarkGraph creates a realistic graph for benchmarking.
func benchmarkGraph(nNodes, nEdges int) *graph.Graph {
	nodes := make(map[string]string)
	edges := make(map[string]map[string]bool)
	for i := 0; i < nNodes; i++ {
		id := filepath.ToSlash(filepath.Join("pkg", "layer", string(rune('a'+i%26)), "**"))
		nodes[id] = id
	}
	for i := 0; i < nNodes; i++ {
		if _, ok := edges[filepath.ToSlash(filepath.Join("pkg", "layer", string(rune('a'+i%26)), "**"))]; !ok {
			edges[filepath.ToSlash(filepath.Join("pkg", "layer", string(rune('a'+i%26)), "**"))] = make(map[string]bool)
		}
	}
	for i := 0; i < nEdges && i < nNodes; i++ {
		src := filepath.ToSlash(filepath.Join("pkg", "layer", string(rune('a'+i%26)), "**"))
		dst := filepath.ToSlash(filepath.Join("pkg", "layer", string(rune('a'+(i+1)%26)), "**"))
		edges[src][dst] = true
	}
	return graph.NewGraph(nodes, edges)
}

func BenchmarkNodeForDir(b *testing.B) {
	g := benchmarkGraph(100, 50)
	testPaths := []string{
		"pkg/layer/a/sub/deep",
		"pkg/layer/b/nested/path",
		"pkg/layer/c/x/y/z",
		"pkg/layer/d",
		"pkg/layer/e/f/g/h",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = g.NodeForDir(testPaths[i%len(testPaths)])
	}
}

func BenchmarkNodeForPath(b *testing.B) {
	g := benchmarkGraph(100, 50)
	testPaths := []string{
		"pkg/layer/a/file.go",
		"pkg/layer/b/src/main.ts",
		"pkg/layer/c/lib/index.dart",
		"pkg/layer/d/src/kotlin/Main.kt",
		"pkg/layer/e/src/lib.rs",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = g.NodeForPath(testPaths[i%len(testPaths)])
	}
}

func BenchmarkMatchDirGlob(b *testing.B) {
	patterns := []string{
		"pkg/layer/a/**",
		"pkg/layer/*/sub/**",
		"pkg/**",
		"pkg/layer/a/b/c/**",
		"pkg/*/layer/*/sub/**",
	}
	paths := []string{
		"pkg/layer/a/sub/deep",
		"pkg/layer/b/sub/nested",
		"pkg/x/y/z",
		"pkg/layer/a/b/c/d/e",
		"pkg/foo/layer/bar/sub/baz",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = graph.MatchDirGlob(patterns[i%len(patterns)], paths[i%len(paths)])
	}
}

func BenchmarkMatchFileGlob(b *testing.B) {
	patterns := []string{
		"pkg/layer/a/*.go",
		"pkg/layer/*/src/*.ts",
		"pkg/**/*.dart",
		"pkg/layer/a/b/*.kt",
		"pkg/*/layer/*/src/*.rs",
	}
	paths := []string{
		"pkg/layer/a/main.go",
		"pkg/layer/b/src/app.ts",
		"pkg/x/lib/main.dart",
		"pkg/layer/a/b/model.kt",
		"pkg/foo/layer/bar/src/main.rs",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = graph.MatchFileGlob(patterns[i%len(patterns)], paths[i%len(paths)])
	}
}

func BenchmarkGlobsOverlap(b *testing.B) {
	pairs := [][2]string{
		{"pkg/layer/a/**", "pkg/layer/b/**"},
		{"pkg/layer/**", "pkg/layer/a/**"},
		{"pkg/**", "src/**"},
		{"pkg/layer/a/b/**", "pkg/layer/a/**"},
		{"pkg/*/layer/*/sub/**", "pkg/foo/layer/bar/sub/**"},
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := pairs[i%len(pairs)]
		_ = graph.GlobsOverlap(p[0], p[1])
	}
}

func BenchmarkGlobSpecificity(b *testing.B) {
	patterns := []string{
		"pkg/**",
		"pkg/layer/a/**",
		"pkg/layer/*/sub/**",
		"pkg/layer/a/b/c/**",
		"pkg/*/layer/*/sub/deep/**",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = graph.GlobSpecificity(patterns[i%len(patterns)])
	}
}

func BenchmarkIsFileGlob(b *testing.B) {
	patterns := []string{
		"pkg/layer/a/**",
		"pkg/layer/a/file.go",
		"pkg/**/*.ts",
		"pkg/layer/a/b/c/main.dart",
		"src/lib.rs",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = graph.IsFileGlob(patterns[i%len(patterns)])
	}
}

func BenchmarkNodeForPath_Cached(b *testing.B) {
	g := benchmarkGraph(100, 50)
	testPath := "pkg/layer/a/sub/deep"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = g.NodeForDir(testPath)
	}
}
