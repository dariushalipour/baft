package main

import (
	_ "embed"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/dariushalipour/strata/internal/adapter/fs/realfs"
	"github.com/dariushalipour/strata/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/strata/internal/adapter/languages/dart"
	"github.com/dariushalipour/strata/internal/adapter/languages/golang"
	"github.com/dariushalipour/strata/internal/adapter/languages/kotlin"
	"github.com/dariushalipour/strata/internal/adapter/languages/rust"
	"github.com/dariushalipour/strata/internal/adapter/languages/typescript"
	"github.com/dariushalipour/strata/internal/adapter/reporters/intellijreporter"
	"github.com/dariushalipour/strata/internal/adapter/reporters/jsonreporter"
	"github.com/dariushalipour/strata/internal/adapter/reporters/textreporter"
	"github.com/dariushalipour/strata/internal/adapter/reporters/vscereporter"
	"github.com/dariushalipour/strata/internal/application/service"
	"github.com/dariushalipour/strata/internal/application/usecase/check"
	"github.com/dariushalipour/strata/internal/application/usecase/draft"
	"github.com/dariushalipour/strata/internal/port"
)

var version string // set by -ldflags at build time

//go:embed docs/usage.md
var usageMD string

//go:embed docs/check-usage.md
var checkUsageMD string

//go:embed docs/draft-usage.md
var draftUsageMD string

//go:embed docs/spec.md
var specMD string

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	switch args[0] {
	case "--help", "-h":
		printUsage()
		os.Exit(0)
	case "--version", "-v":
		printVersion()
		os.Exit(0)
	case "check":
		runCheck(args[1:])
	case "draft":
		runDraft(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\nRun 'strata --help' for usage\n", args[0])
		os.Exit(1)
	}
}

func runCheck(args []string) {
	var root string
	var reporterName = "text"
	var langs []string

	for _, a := range args {
		switch a {
		case "--help", "-h":
			printCheckUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(a, "--reporter=") {
				reporterName = strings.TrimPrefix(a, "--reporter=")
			} else if strings.HasPrefix(a, "--lang") {
				langs = append(langs, strings.TrimPrefix(a, "--lang"))
			} else if strings.HasPrefix(a, "--") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'strata check --help' for usage\n", a)
				os.Exit(1)
			} else if root == "" {
				root = a
			}
		}
	}

	if root == "" {
		root = "."
	}

	if reporterName != "text" && reporterName != "json" && reporterName != "vsce" && reporterName != "intellij" {
		fmt.Fprintf(os.Stderr, "unknown reporter: %s\n\nRun 'strata check --help' for usage\n", reporterName)
		os.Exit(1)
	}

	fs := realfs.New()
	languages := resolveLangs(langs)
	repo := &mermaid.MermaidRepository{}

	discovery := service.NewCapsuleDiscovery()
	for _, lang := range languages {
		switch lang.Name() {
		case "go":
			golang.RegisterDiscovery(discovery)
		case "dart":
			dart.RegisterDiscovery(discovery)
		case "kotlin":
			kotlin.RegisterDiscovery(discovery)
		case "typescript":
			typescript.RegisterDiscovery(discovery)
		case "rust":
			rust.RegisterDiscovery(discovery)
		}
	}

	result := check.Run(fs, root, languages, repo, discovery)

	var renderer port.CheckResultRenderer
	switch reporterName {
	case "json":
		renderer = &jsonreporter.JSONRenderer{}
	case "vsce":
		renderer = &vscereporter.VSCERenderer{}
	case "intellij":
		renderer = &intellijreporter.IntelliJRenderer{}
	default:
		renderer = &textreporter.TextRenderer{}
	}

	fmt.Print(renderer.Render(result))

	if len(result.Violations) > 0 || len(result.Errors) > 0 {
		os.Exit(1)
	}
}

func runDraft(args []string) {
	var root string
	var langs []string

	for _, a := range args {
		switch a {
		case "--help", "-h":
			printDraftUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(a, "--lang") {
				langs = append(langs, strings.TrimPrefix(a, "--lang"))
			} else if strings.HasPrefix(a, "--") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'strata draft --help' for usage\n", a)
				os.Exit(1)
			} else if root == "" {
				root = a
			}
		}
	}

	if root == "" {
		root = "."
	}

	fs := realfs.New()
	languages := resolveLangs(langs)
	repo := &mermaid.MermaidRepository{}

	discovery := service.NewCapsuleDiscovery()
	for _, lang := range languages {
		switch lang.Name() {
		case "go":
			golang.RegisterDiscovery(discovery)
		case "dart":
			dart.RegisterDiscovery(discovery)
		case "kotlin":
			kotlin.RegisterDiscovery(discovery)
		case "typescript":
			typescript.RegisterDiscovery(discovery)
		case "rust":
			rust.RegisterDiscovery(discovery)
		}
	}

	result, err := draft.Run(fs, root, languages, repo, discovery)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(result.Capsules) == 0 {
		fmt.Fprintln(os.Stderr, "no capsules found")
		os.Exit(1)
	}

	for _, c := range result.Capsules {
		fmt.Printf("drafted: %s (%d files, %d nodes, %d edges)\n", c.ConfigPath, c.FilesScanned, c.Nodes, c.Edges)
	}
}

func printUsage() {
	fmt.Print(specMD)
	fmt.Println()
	fmt.Print(usageMD)
}

func printCheckUsage() {
	fmt.Print(checkUsageMD)
}

func printDraftUsage() {
	fmt.Print(draftUsageMD)
}

func printVersion() {
	v := version
	if v == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			v = info.Main.Version
		}
		if v == "" {
			v = "dev"
		}
	}
	fmt.Println(v)
}

func resolveLangs(names []string) []port.Language {
	if len(names) == 0 {
		return []port.Language{golang.Language{}, dart.Language{}, kotlin.Language{}, typescript.Language{}, rust.Language{}}
	}
	var out []port.Language
	for _, n := range names {
		switch n {
		case "go", "golang":
			out = append(out, golang.Language{})
		case "typescript", "ts":
			out = append(out, typescript.Language{})
		case "dart":
			out = append(out, dart.Language{})
		case "kotlin", "kt":
			out = append(out, kotlin.Language{})
		case "rust", "rs":
			out = append(out, rust.Language{})
		default:
			fmt.Fprintf(os.Stderr, "unknown language: %s\n", n)
			os.Exit(1)
		}
	}
	return out
}
