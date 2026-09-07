package cli

import (
	"fmt"

	"github.com/dariushalipour/baft/internal/adapter/languages/csharp"
	"github.com/dariushalipour/baft/internal/adapter/languages/dart"
	"github.com/dariushalipour/baft/internal/adapter/languages/golang"
	"github.com/dariushalipour/baft/internal/adapter/languages/jvm"
	"github.com/dariushalipour/baft/internal/adapter/languages/python"
	"github.com/dariushalipour/baft/internal/adapter/languages/rust"
	"github.com/dariushalipour/baft/internal/adapter/languages/typescript"
	"github.com/dariushalipour/baft/internal/adapter/reporters/diagnosticsreporter"
	"github.com/dariushalipour/baft/internal/adapter/reporters/jsonreporter"
	"github.com/dariushalipour/baft/internal/adapter/reporters/textreporter"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/port"
)

var languageNames = []string{"go", "csharp", "dart", "java", "kotlin", "python", "rust", "typescript"}

func newLanguage(name string) port.Language {
	switch name {
	case "go":
		return golang.Language{}
	case "csharp":
		return csharp.Language{}
	case "dart":
		return dart.Language{}
	case "java", "kotlin":
		return jvm.Language{}
	case "python":
		return python.Language{}
	case "rust":
		return rust.Language{}
	case "typescript":
		return &typescript.Language{}
	}
	return nil
}

func resolveLangs(names []string) ([]port.Language, error) {
	if len(names) == 0 {
		names = languageNames
	}
	langs := make([]port.Language, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		lang := newLanguage(name)
		if lang == nil {
			return nil, fmt.Errorf("unknown language: %s", name)
		}
		if seen[lang.Name()] {
			continue
		}
		seen[lang.Name()] = true
		langs = append(langs, lang)
	}
	return langs, nil
}

func newDiscovery(langs []port.Language) *service.CapsuleDiscovery {
	discovery := service.NewCapsuleDiscovery()
	for _, lang := range langs {
		lang.Register(discovery)
	}
	return discovery
}

// newRenderer resolves a reporter name; "vsce" and "intellij" are aliases of
// the diagnostics reporter kept for the editor integrations.
func newRenderer(name string, color bool) port.CheckResultRenderer {
	switch name {
	case "text":
		return &textreporter.TextRenderer{Color: color}
	case "json":
		return &jsonreporter.JSONRenderer{}
	case "diagnostics", "vsce", "intellij":
		return &diagnosticsreporter.Renderer{}
	}
	return nil
}
