package port

import "github.com/dariushalipour/strata/internal/domain/graph"

// GraphRepository persists and loads Graph objects.
type GraphRepository interface {
	Load(content string) (*graph.Graph, error)
	Save(g *graph.Graph) string
}
