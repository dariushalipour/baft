package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"
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
	integrateusecase "github.com/dariushalipour/baft/internal/application/usecase/integrate"
	"github.com/dariushalipour/baft/internal/application/usecase/restyle"
	"github.com/dariushalipour/baft/internal/integrations"
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

//go:embed docs/cli-assets/integrate-usage.txt
var integrateUsageText string

//go:embed docs/cli-assets/help-intro.txt
var helpIntroText string

//go:embed docs/manual.md
var manualText string

const (
	exitSuccess = 0
	exitError   = 1
)

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		printUsage()
		os.Exit(exitSuccess)
	}

	var exitCode int
	switch args[0] {
	case "--help", "-h":
		printUsage()
	case "--version", "-v":
		printVersion()
	case "check":
		exitCode = runCheck(args[1:])
	case "dump":
		exitCode = runDump(args[1:])
	case "restyle":
		exitCode = runRestyle(args[1:])
	case "integrate":
		exitCode = runIntegrate(args[1:])
	case "manual":
		exitCode = runManual(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\nRun 'baft --help' for usage\n", args[0])
		exitCode = exitError
	}

	os.Exit(exitCode)
}

func runCheck(args []string) int {
	var root string
	var reporterName = "text"
	var langs []string
	var overlayStdin bool

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help", "-h":
			printCheckUsage()
			return exitSuccess
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
						return exitError
					}
				}
				langs = append(langs, val)
			} else if strings.HasPrefix(a, "--") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'baft check --help' for usage\n", a)
				return exitError
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
		return exitError
	}

	var fs port.FileSystem = realfs.New()
	if overlayStdin {
		payload, err := overlayfs.Decode(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid overlay stdin: %v\n", err)
			return exitError
		}
		fs = overlayfs.NewFromPayload(fs, payload)
	}
	languages, err := resolveLangs(langs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return exitError
	}
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
		return exitError
	}
	return exitSuccess
}

func runDump(args []string) int {
	var root string
	var langs []string
	saveOpts := port.GraphSaveOptions{ColorPalette: port.ColorPaletteVibrant}

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help", "-h":
			printDumpUsage()
			return exitSuccess
		default:
			if strings.HasPrefix(a, "--lang") {
				val := strings.TrimPrefix(a, "--lang")
				if val == "" {
					if i+1 < len(args) {
						i++
						val = args[i]
					} else {
						fmt.Fprintf(os.Stderr, "--lang requires a value\n\nRun 'baft dump --help' for usage\n")
						return exitError
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
					return exitError
				}
				palette, ok := port.ParseGraphColorPalette(val)
				if !ok {
					fmt.Fprintf(os.Stderr, "unknown color palette: %s\n\nRun 'baft dump --help' for usage\n", val)
					return exitError
				}
				saveOpts.ColorPalette = palette
			} else if strings.HasPrefix(a, "--") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'baft dump --help' for usage\n", a)
				return exitError
			} else if root == "" {
				root = a
			}
		}
	}

	if root == "" {
		root = "."
	}

	fs := realfs.New()
	languages, err := resolveLangs(langs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return exitError
	}
	repo := &mermaid.MermaidRepository{}

	discovery := service.NewCapsuleDiscovery()
	for _, lang := range languages {
		lang.Register(discovery)
	}

	result, err := dump.RunWithOptions(fs, root, languages, repo, discovery, saveOpts, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitError
	}
	if len(result.Contracts) == 0 && len(result.Errors) == 0 {
		return exitSuccess
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
	return exitSuccess
}

func runManual(args []string) int {
	for _, a := range args {
		switch a {
		case "--help", "-h":
			printManual()
			return exitSuccess
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'baft manual' for the Baft manual\n", a)
			return exitError
		}
	}

	printManual()
	return exitSuccess
}

func runIntegrate(args []string) int {
	var verifyCompatible bool
	var autoSelect bool
	var family string
	var integrationID string
	var pluginVersion string
	var protocol int

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help", "-h":
			printIntegrateUsage()
			return exitSuccess
		case "--verify-compatible":
			verifyCompatible = true
		case "--yes", "-y":
			autoSelect = true
		default:
			if strings.HasPrefix(a, "--integration=") {
				family = strings.TrimPrefix(a, "--integration=")
				integrationID = family
			} else if a == "--integration" {
				if i+1 >= len(args) {
					fmt.Fprintf(os.Stderr, "--integration requires a value\n")
					return exitError
				}
				i++
				family = args[i]
				integrationID = family
			} else if strings.HasPrefix(a, "--plugin-version=") {
				pluginVersion = strings.TrimPrefix(a, "--plugin-version=")
			} else if a == "--plugin-version" {
				if i+1 >= len(args) {
					fmt.Fprintf(os.Stderr, "--plugin-version requires a value\n")
					return exitError
				}
				i++
				pluginVersion = args[i]
			} else if strings.HasPrefix(a, "--protocol=") {
				value := strings.TrimPrefix(a, "--protocol=")
				parsed, err := strconv.Atoi(value)
				if err != nil {
					fmt.Fprintf(os.Stderr, "invalid protocol value: %s\n", value)
					return exitError
				}
				protocol = parsed
			} else if a == "--protocol" {
				if i+1 >= len(args) {
					fmt.Fprintf(os.Stderr, "--protocol requires a value\n")
					return exitError
				}
				i++
				parsed, err := strconv.Atoi(args[i])
				if err != nil {
					fmt.Fprintf(os.Stderr, "invalid protocol value: %s\n", args[i])
					return exitError
				}
				protocol = parsed
			} else if strings.HasPrefix(a, "--") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'baft integrate --help' for usage\n", a)
				return exitError
			} else {
				fmt.Fprintf(os.Stderr, "unknown argument: %s\n\nRun 'baft integrate --help' for usage\n", a)
				return exitError
			}
		}
	}

	catalog := integrations.NewCatalog(cliVersion())
	if verifyCompatible {
		if integrationID == "" || pluginVersion == "" || protocol == 0 {
			fmt.Fprintln(os.Stderr, "--verify-compatible requires --integration, --plugin-version, and --protocol")
			return exitError
		}
		report := catalog.VerifyCompatibility(integrationID, pluginVersion, protocol)
		encoded, err := json.Marshal(report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not encode compatibility report: %v\n", err)
			return exitError
		}
		fmt.Println(string(encoded))
		if !report.Compatible {
			return exitError
		}
		return exitSuccess
	}

	err := integrateusecase.Run(context.Background(), catalog, integrateusecase.Options{
		In:         os.Stdin,
		Out:        os.Stdout,
		AutoSelect: autoSelect,
		Family:     family,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitError
	}
	return exitSuccess
}

func runRestyle(args []string) int {
	var root string
	var contractPath string
	var stdin bool
	saveOpts := port.GraphSaveOptions{ColorPalette: port.ColorPaletteVibrant}

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help", "-h":
			printRestyleUsage()
			return exitSuccess
		case "--stdin":
			stdin = true
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
					return exitError
				}
				palette, ok := port.ParseGraphColorPalette(val)
				if !ok {
					fmt.Fprintf(os.Stderr, "unknown color palette: %s\n\nRun 'baft restyle --help' for usage\n", val)
					return exitError
				}
				saveOpts.ColorPalette = palette
			} else if a == "--path" || strings.HasPrefix(a, "--path=") {
				val := ""
				if strings.HasPrefix(a, "--path=") {
					val = strings.TrimPrefix(a, "--path=")
				} else if i+1 < len(args) {
					i++
					val = args[i]
				} else {
					fmt.Fprintf(os.Stderr, "--path requires a value\n\nRun 'baft restyle --help' for usage\n")
					return exitError
				}
				contractPath = val
			} else if strings.HasPrefix(a, "--") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\nRun 'baft restyle --help' for usage\n", a)
				return exitError
			} else if root == "" {
				root = a
			}
		}
	}

	if root == "" {
		root = "."
	}

	if stdin {
		if contractPath == "" {
			fmt.Fprintf(os.Stderr, "--stdin requires --path\n\nRun 'baft restyle --help' for usage\n")
			return exitError
		}
		if root != "." {
			fmt.Fprintf(os.Stderr, "--stdin does not accept a root-dir\n\nRun 'baft restyle --help' for usage\n")
			return exitError
		}

		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "restyle: %s: %v\n", contractPath, err)
			return exitError
		}

		repo := &mermaid.MermaidRepository{}
		restyled, _, err := restyle.RestyleContract(string(raw), repo, saveOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "restyle: %s: %v\n", contractPath, err)
			return exitError
		}

		fmt.Print(restyled)
		return exitSuccess
	}
	if contractPath != "" {
		fmt.Fprintf(os.Stderr, "--path requires --stdin\n\nRun 'baft restyle --help' for usage\n")
		return exitError
	}

	fs := realfs.New()
	repo := &mermaid.MermaidRepository{}

	result, err := restyle.Run(fs, root, repo, saveOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitError
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
		return exitError
	}
	return exitSuccess
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

func printIntegrateUsage() {
	fmt.Print(integrateUsageText)
}

func printVersion() {
	fmt.Println(cliVersion())
}

func cliVersion() string {
	v := version
	if v == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			v = info.Main.Version
		}
		if v == "" {
			v = "dev"
		}
	}
	return v
}

func resolveLangs(names []string) ([]port.Language, error) {
	if len(names) == 0 {
		return []port.Language{golang.Language{}, dart.Language{}, kotlin.Language{}, &typescript.Language{}, rust.Language{}}, nil
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
			return nil, fmt.Errorf("unknown language: %s", n)
		}
	}
	return out, nil
}
