package main

import (
	_ "embed"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/dariushalipour/baft/internal/adapter/fs/overlayfs"
	"github.com/dariushalipour/baft/internal/adapter/fs/realfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/baft/internal/adapter/languages/dart"
	"github.com/dariushalipour/baft/internal/adapter/languages/golang"
	"github.com/dariushalipour/baft/internal/adapter/languages/kotlin"
	"github.com/dariushalipour/baft/internal/adapter/languages/rust"
	"github.com/dariushalipour/baft/internal/adapter/languages/typescript"
	"github.com/dariushalipour/baft/internal/adapter/reporters/intellijreporter"
	"github.com/dariushalipour/baft/internal/adapter/reporters/jsonreporter"
	"github.com/dariushalipour/baft/internal/adapter/reporters/textreporter"
	"github.com/dariushalipour/baft/internal/adapter/reporters/vscereporter"
	"github.com/dariushalipour/baft/internal/application/service"
	"github.com/dariushalipour/baft/internal/application/usecase/check"
	"github.com/dariushalipour/baft/internal/application/usecase/dump"
	"github.com/dariushalipour/baft/internal/application/usecase/restyle"
	"github.com/dariushalipour/baft/internal/port"
)

var version string // set by -ldflags at build time

//go:embed docs/cli-assets/usage.txt
var usageText string

//go:embed docs/cli-assets/check-usage.txt
var checkUsageText string

//go:embed docs/cli-assets/dump-usage.txt
var dumpUsageText string

//go:embed docs/cli-assets/restyle-usage.txt
var restyleUsageText string

//go:embed docs/cli-assets/help-intro.txt
var helpIntroText string

//go:embed docs/manual.md
var manualText string

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
	case "dump":
		runDump(args[1:])
	case "restyle":
		runRestyle(args[1:])
	case "manual":
		runManual(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\nRun 'baft --help' for usage\n", args[0])
		os.Exit(1)
	}
}

func runCheck(args []string) {
	var root string
	var reporterName = "text"
	var langs []string
	var overlayStdin bool

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help", "-h":
			printCheckUsage()
			os.Exit(0)
		case "--overlay-stdin":
			overlayStdin = true
		default:
			if strings.HasPrefix(a, "--reporter=") {
				reporterName = strings.TrimPrefix(a, "--reporter=")
			} else if strings.HasPrefix(a, "--lang") {
				val := strings.TrimPrefix(a, "--lang")
				if val == "" {
					if i+1 < len(args) {
						i++
						val = args[i]
					} else {
						fmt.Fprintf(os.Stderr, "--lang requires a value\n\nRun 'baft check --help' for usage\n")
						os.Exit(1)
					}
				}
				langs = append(langs, val)
			} else if strings.HasPrefix(a, "--") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'baft check --help' for usage\n", a)
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
		fmt.Fprintf(os.Stderr, "unknown reporter: %s\n\nRun 'baft check --help' for usage\n", reporterName)
		os.Exit(1)
	}

	var fs port.FileSystem = realfs.New()
	if overlayStdin {
		payload, err := overlayfs.Decode(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid overlay stdin: %v\n", err)
			os.Exit(1)
		}
		fs = overlayfs.NewFromPayload(fs, payload)
	}
	languages := resolveLangs(langs)
	repo := &mermaid.MermaidRepository{}

	discovery := service.NewCapsuleDiscovery()
	for _, lang := range languages {
		lang.Register(discovery)
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

func runDump(args []string) {
	var root string
	var langs []string
	saveOpts := port.GraphSaveOptions{ColorPalette: port.ColorPaletteVibrant}

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help", "-h":
			printDumpUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(a, "--lang") {
				val := strings.TrimPrefix(a, "--lang")
				if val == "" {
					if i+1 < len(args) {
						i++
						val = args[i]
					} else {
						fmt.Fprintf(os.Stderr, "--lang requires a value\n\nRun 'baft dump --help' for usage\n")
						os.Exit(1)
					}
				}
				langs = append(langs, val)
			} else if a == "--color-palette" || strings.HasPrefix(a, "--color-palette=") {
				val := ""
				if strings.HasPrefix(a, "--color-palette=") {
					val = strings.TrimPrefix(a, "--color-palette=")
				} else if i+1 < len(args) {
					i++
					val = args[i]
				} else {
					fmt.Fprintf(os.Stderr, "--color-palette requires a value\n\nRun 'baft dump --help' for usage\n")
					os.Exit(1)
				}
				palette, ok := port.ParseGraphColorPalette(val)
				if !ok {
					fmt.Fprintf(os.Stderr, "unknown color palette: %s\n\nRun 'baft dump --help' for usage\n", val)
					os.Exit(1)
				}
				saveOpts.ColorPalette = palette
			} else if strings.HasPrefix(a, "--") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'baft dump --help' for usage\n", a)
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
		lang.Register(discovery)
	}

	result, err := dump.RunWithOptions(fs, root, languages, repo, discovery, saveOpts, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(result.Contracts) == 0 && len(result.Errors) == 0 {
		os.Exit(0)
	}

	for _, c := range result.Contracts {
		status := "amended"
		if c.IsNew {
			status = "new"
		}
		if c.AmendDiff != nil {
			fmt.Printf("[%s] %s (+%d nodes, +%d edges)\n", status, c.ContractPath, c.AmendDiff.Nodes, c.AmendDiff.Edges)
		} else {
			fmt.Printf("[%s] %s (%d files, %d nodes, %d edges)\n", status, c.ContractPath, c.FilesScanned, c.Nodes, c.Edges)
		}
	}
}

func runManual(args []string) {
	for _, a := range args {
		switch a {
		case "--help", "-h":
			printManual()
			os.Exit(0)
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'baft manual' for the BAFT.md manual\n", a)
			os.Exit(1)
		}
	}

	printManual()
}

func runRestyle(args []string) {
	var root string
	saveOpts := port.GraphSaveOptions{ColorPalette: port.ColorPaletteVibrant}

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help", "-h":
			printRestyleUsage()
			os.Exit(0)
		default:
			if a == "--color-palette" || strings.HasPrefix(a, "--color-palette=") {
				val := ""
				if strings.HasPrefix(a, "--color-palette=") {
					val = strings.TrimPrefix(a, "--color-palette=")
				} else if i+1 < len(args) {
					i++
					val = args[i]
				} else {
					fmt.Fprintf(os.Stderr, "--color-palette requires a value\n\nRun 'baft restyle --help' for usage\n")
					os.Exit(1)
				}
				palette, ok := port.ParseGraphColorPalette(val)
				if !ok {
					fmt.Fprintf(os.Stderr, "unknown color palette: %s\n\nRun 'baft restyle --help' for usage\n", val)
					os.Exit(1)
				}
				saveOpts.ColorPalette = palette
			} else if strings.HasPrefix(a, "--") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'baft restyle --help' for usage\n", a)
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
	repo := &mermaid.MermaidRepository{}

	result, err := restyle.Run(fs, root, repo, saveOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	for _, contract := range result.Contracts {
		status := "unchanged"
		if contract.Changed {
			status = "restyled"
		}
		fmt.Printf("[%s] %s\n", status, contract.ContractPath)
	}
	if len(result.Errors) > 0 {
		for _, restyleErr := range result.Errors {
			fmt.Fprintf(os.Stderr, "restyle: %s\n", restyleErr)
		}
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(helpIntroText)
	fmt.Println()
	fmt.Print(usageText)
}

func printManual() {
	fmt.Print(manualText)
}

func printCheckUsage() {
	fmt.Print(checkUsageText)
}

func printDumpUsage() {
	fmt.Print(dumpUsageText)
}

func printRestyleUsage() {
	fmt.Print(restyleUsageText)
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
		return []port.Language{golang.Language{}, dart.Language{}, kotlin.Language{}, &typescript.Language{}, rust.Language{}}
	}
	var out []port.Language
	for _, n := range names {
		switch n {
		case "go":
			out = append(out, golang.Language{})
		case "typescript":
			out = append(out, &typescript.Language{})
		case "dart":
			out = append(out, dart.Language{})
		case "kotlin":
			out = append(out, kotlin.Language{})
		case "rust":
			out = append(out, rust.Language{})
		default:
			fmt.Fprintf(os.Stderr, "unknown language: %s\n", n)
			os.Exit(1)
		}
	}
	return out
}
