package port

import "github.com/dariushalipour/baft/internal/domain/graph"

type GraphColorPalette string

const (
	ColorPaletteVibrant GraphColorPalette = "vibrant"
	ColorPaletteMuted   GraphColorPalette = "muted"
	ColorPaletteMono    GraphColorPalette = "mono"
	ColorPaletteNone    GraphColorPalette = "none"
)

type GraphSaveOptions struct {
	ColorPalette GraphColorPalette
}

func ParseGraphColorPalette(value string) (GraphColorPalette, bool) {
	palette := GraphColorPalette(value)
	switch palette {
	case ColorPaletteVibrant, ColorPaletteMuted, ColorPaletteMono, ColorPaletteNone:
		return palette, true
	default:
		return "", false
	}
}

// GraphRepository persists and loads Graph objects.
type GraphRepository interface {
	Load(content string) (*graph.Graph, error)
	Save(g *graph.Graph, opts GraphSaveOptions) string
}
