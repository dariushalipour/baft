package port

import (
	"fmt"
	"strings"

	"github.com/dariushalipour/baft/internal/domain/graph"
)

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

type GraphRepository interface {
	Load(content string) (*graph.Graph, error)
	Save(g *graph.Graph, opts GraphSaveOptions) string
	Restyle(content string, opts GraphSaveOptions) (string, error)
}

// ParseError reports a contract line a GraphRepository could not parse. File
// is empty when the repository is handed bare content; callers that know which
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
