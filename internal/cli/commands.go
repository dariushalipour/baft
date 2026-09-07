package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/dariushalipour/baft/internal/adapter/fs/dryrunfs"
	"github.com/dariushalipour/baft/internal/adapter/fs/overlayfs"
	"github.com/dariushalipour/baft/internal/adapter/fs/realfs"
	"github.com/dariushalipour/baft/internal/adapter/graph_repositories/mermaid"
	"github.com/dariushalipour/baft/internal/application/usecase/check"
	"github.com/dariushalipour/baft/internal/application/usecase/dump"
	integrateusecase "github.com/dariushalipour/baft/internal/application/usecase/integrate"
	"github.com/dariushalipour/baft/internal/application/usecase/restyle"
	"github.com/dariushalipour/baft/internal/integrations"
	"github.com/dariushalipour/baft/internal/port"
)

func (a *app) check(args []string) int {
	var langs stringList
	fset := newFlagSet("check")
	reporter := fset.String("reporter", "text", "output format")
	overlayStdin := fset.Bool("overlay-stdin", false, "read an unsaved-file overlay from stdin")
	fset.Var(&langs, "lang", "language filter (repeatable)")

	operands, err := parse(fset, args)
	if err != nil {
		return a.parsed("check", err)
	}
	root, err := rootDir(operands)
	if err != nil {
		return a.usageErr("check", err)
	}
	renderer := newRenderer(*reporter, a.color)
	if renderer == nil {
		return a.usageErr("check", fmt.Errorf("unknown reporter: %s", *reporter))
	}
	languages, err := resolveLangs(langs)
	if err != nil {
		return a.usageErr("check", err)
	}

	var fsys port.FileSystem = realfs.New()
	if *overlayStdin {
		payload, decodeErr := overlayfs.Decode(a.in)
		if decodeErr != nil {
			return a.fail("invalid overlay stdin: %v", decodeErr)
		}
		fsys = overlayfs.NewFromPayload(fsys, payload)
	}

	discovery := newDiscovery(languages)
	var result *port.CheckResult
	if scoped, warnings, wrapErr := ignoreAware(fsys, root, discovery); wrapErr != nil {
		result = &port.CheckResult{Errors: []string{wrapErr.Error()}}
	} else {
		result = check.Run(scoped, root, languages, &mermaid.MermaidRepository{}, discovery)
		result.Warnings = append(warnings, result.Warnings...)
	}
	fmt.Fprint(a.out, renderer.Render(result))

	if len(result.Capsules) == 0 && len(result.Errors) == 0 {
		fmt.Fprintf(a.errOut, "warning: nothing was checked under %s — no capsule with a BAFT.md contract was found\n", root)
	}
	if result.Failed() {
		return exitFail
	}
	return exitOK
}

func (a *app) dump(args []string) int {
	var langs stringList
	fset := newFlagSet("dump")
	palette := paletteFlag(fset)
	dryRun := fset.Bool("dry-run", false, "report what would change without writing")
	fset.Var(&langs, "lang", "language filter (repeatable)")

	operands, err := parse(fset, args)
	if err != nil {
		return a.parsed("dump", err)
	}
	root, err := rootDir(operands)
	if err != nil {
		return a.usageErr("dump", err)
	}
	saveOpts, err := palette.options()
	if err != nil {
		return a.usageErr("dump", err)
	}
	languages, err := resolveLangs(langs)
	if err != nil {
		return a.usageErr("dump", err)
	}

	discovery := newDiscovery(languages)
	fsys, warnings, err := ignoreAware(realfs.New(), root, discovery)
	if err != nil {
		return a.fail("error: %v", err)
	}
	for _, w := range warnings {
		fmt.Fprintln(a.errOut, "warning: "+w)
	}
	if *dryRun {
		fsys = dryrunfs.Wrap(fsys)
	}

	result, err := dump.RunWithOptions(fsys, root, languages, &mermaid.MermaidRepository{}, discovery, dump.Options{Save: saveOpts, Log: a.errOut})
	if err != nil {
		return a.fail("error: %v", err)
	}
	for _, c := range result.Contracts {
		status := "amended"
		if c.IsNew {
			status = "new"
		}
		if *dryRun {
			status = "would amend"
			if c.IsNew {
				status = "would create"
			}
		}
		if c.AmendDiff == nil {
			fmt.Fprintf(a.out, "[%s] %s (%d files, %d nodes, %d edges)\n", status, c.ContractPath, c.FilesScanned, c.Nodes, c.Edges)
			continue
		}
		fmt.Fprintf(a.out, "[%s] %s (+%d nodes, +%d edges)\n", status, c.ContractPath, len(c.AmendDiff.Nodes), len(c.AmendDiff.Edges))
		for _, node := range c.AmendDiff.Nodes {
			fmt.Fprintf(a.out, "    + node %s\n", node)
		}
		for _, edge := range c.AmendDiff.Edges {
			fmt.Fprintf(a.out, "    + edge %s\n", edge)
		}
	}
	if len(result.Errors) > 0 {
		return exitFail
	}
	return exitOK
}

func (a *app) restyle(args []string) int {
	fset := newFlagSet("restyle")
	palette := paletteFlag(fset)
	stdin := fset.Bool("stdin", false, "read one contract from stdin")
	contractPath := fset.String("path", "", "file path label for --stdin mode")

	operands, err := parse(fset, args)
	if err != nil {
		return a.parsed("restyle", err)
	}
	root, err := rootDir(operands)
	if err != nil {
		return a.usageErr("restyle", err)
	}
	saveOpts, err := palette.options()
	if err != nil {
		return a.usageErr("restyle", err)
	}
	repo := &mermaid.MermaidRepository{}

	if *stdin {
		if *contractPath == "" {
			return a.usageErr("restyle", errors.New("--stdin requires --path"))
		}
		if len(operands) > 0 {
			return a.usageErr("restyle", errors.New("--stdin does not accept a root-dir"))
		}
		raw, readErr := io.ReadAll(a.in)
		if readErr != nil {
			return a.fail("restyle: %s: %v", *contractPath, readErr)
		}
		restyled, _, restyleErr := restyle.RestyleContract(string(raw), repo, saveOpts)
		if restyleErr != nil {
			return a.fail("restyle: %s: %v", *contractPath, restyleErr)
		}
		fmt.Fprint(a.out, restyled)
		return exitOK
	}
	if *contractPath != "" {
		return a.usageErr("restyle", errors.New("--path requires --stdin"))
	}

	result, err := restyle.Run(realfs.New(), root, repo, saveOpts)
	if err != nil {
		return a.fail("error: %v", err)
	}
	for _, contract := range result.Contracts {
		status := "unchanged"
		if contract.Changed {
			status = "restyled"
		}
		fmt.Fprintf(a.out, "[%s] %s\n", status, contract.ContractPath)
	}
	if len(result.Errors) > 0 {
		for _, restyleErr := range result.Errors {
			fmt.Fprintf(a.errOut, "restyle: %s\n", restyleErr)
		}
		return exitFail
	}
	return exitOK
}

func (a *app) integrate(args []string) int {
	fset := newFlagSet("integrate")
	verifyCompatible := fset.Bool("verify-compatible", false, "verify plugin compatibility")
	integration := fset.String("integration", "", "target integration")
	pluginVersion := fset.String("plugin-version", "", "installed plugin version")
	protocol := fset.Int("protocol", 0, "plugin protocol version")
	var autoSelect bool
	fset.BoolVar(&autoSelect, "yes", false, "auto-select first detected IDE")
	fset.BoolVar(&autoSelect, "y", false, "auto-select first detected IDE")

	operands, err := parse(fset, args)
	if err != nil {
		return a.parsed("integrate", err)
	}
	if err := noOperands(operands); err != nil {
		return a.usageErr("integrate", err)
	}

	catalog := integrations.NewCatalog(a.cliVersion())
	if *verifyCompatible {
		if *integration == "" || *pluginVersion == "" || *protocol == 0 {
			return a.usageErr("integrate", errors.New("--verify-compatible requires --integration, --plugin-version, and --protocol"))
		}
		report := catalog.VerifyCompatibility(*integration, *pluginVersion, *protocol)
		encoded, encodeErr := json.Marshal(report)
		if encodeErr != nil {
			return a.fail("could not encode compatibility report: %v", encodeErr)
		}
		fmt.Fprintln(a.out, string(encoded))
		if !report.Compatible {
			return exitFail
		}
		return exitOK
	}

	err = integrateusecase.Run(context.Background(), catalog, integrateusecase.Options{
		In:          a.in,
		Out:         a.out,
		AutoSelect:  autoSelect,
		Integration: *integration,
	})
	if err != nil {
		return a.fail("%v", err)
	}
	return exitOK
}

func (a *app) manual(args []string) int {
	operands, err := parse(newFlagSet("manual"), args)
	if errors.Is(err, flag.ErrHelp) {
		err = nil
	}
	if err == nil {
		err = noOperands(operands)
	}
	if err != nil {
		return a.usageErr("manual", err)
	}
	fmt.Fprint(a.out, a.doc("manual.md"))
	return exitOK
}

type palette struct{ name *string }

func paletteFlag(fset *flag.FlagSet) palette {
	return palette{fset.String("color-palette", "vibrant", "styling palette")}
}

func (p palette) options() (port.GraphSaveOptions, error) {
	parsed, ok := port.ParseGraphColorPalette(*p.name)
	if !ok {
		return port.GraphSaveOptions{}, fmt.Errorf("unknown color palette: %s", *p.name)
	}
	return port.GraphSaveOptions{ColorPalette: parsed}, nil
}
