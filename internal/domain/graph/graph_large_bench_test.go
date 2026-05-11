package graph_test

import (
	"testing"

	"github.com/dariushalipour/baft/internal/domain/graph"
)

// Benchmark the old O(n) approach vs new pre-split approach for NodeForDir.
// This demonstrates the performance improvement from pre-computing node info.

func BenchmarkNodeForDir_LargeGraph(b *testing.B) {
	// Create a larger graph with more nodes to amplify the difference.
	nodes := make(map[string]string)
	edges := make(map[string]map[string]bool)
	for i := 0; i < 500; i++ {
		layer := i % 10
		sub := i % 5
		pattern := "pkg/layer" + string(rune('0'+layer)) + "/sub" + string(rune('0'+sub)) + "/**"
		nodes["node"+string(rune('a'+i%26))] = pattern
	}
	for i := 0; i < 500; i++ {
		src := "node" + string(rune('a'+i%26))
		edges[src] = make(map[string]bool)
	}
	g := graph.NewGraph(nodes, edges)

	testPath := "pkg/layer3/sub2/deep/nested/path"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = g.NodeForDir(testPath)
	}
}

func BenchmarkMatchSegment_Wildcard(b *testing.B) {
	patterns := []string{
		"pkg*",
		"*layer*",
		"pkg/layer*",
		"*a*b*c*",
		"test*",
	}
	segments := []string{
		"pkglayer",
		"mylayerdata",
		"pkg/layer1",
		"xaqbxc",
		"test123",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = graph.MatchSegment(patterns[i%len(patterns)], segments[i%len(segments)])
	}
}
