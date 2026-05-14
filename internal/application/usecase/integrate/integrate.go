package integrate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/dariushalipour/baft/internal/integrations"
)

type Manager interface {
	Detect(context.Context) ([]integrations.IDEInstallation, error)
	Install(context.Context, integrations.IDEInstallation) error
	Verify(context.Context, integrations.IDEInstallation) error
}

type Options struct {
	In  io.Reader
	Out io.Writer
}

func Run(ctx context.Context, manager Manager, opts Options) error {
	in := opts.In
	if in == nil {
		in = strings.NewReader("")
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}

	installations, err := manager.Detect(ctx)
	if len(installations) == 0 {
		if err != nil {
			return err
		}
		return errors.New("no supported integrations detected")
	}

	sort.SliceStable(installations, func(i, j int) bool {
		if installations[i].Family == installations[j].Family {
			return displayLabel(installations[i]) < displayLabel(installations[j])
		}
		return installations[i].Family < installations[j].Family
	})

	if err != nil {
		fmt.Fprintf(out, "Some integrations could not be inspected: %v\n\n", err)
	}

	fmt.Fprintln(out, "Available integrations:")
	fmt.Fprintln(out)
	for i, ide := range installations {
		fmt.Fprintf(out, "%d. %s\n", i+1, displayLabel(ide))
	}

	selection, err := promptForSelection(in, out, installations)
	if err != nil {
		return err
	}

	if err := manager.Install(ctx, selection); err != nil {
		return err
	}
	if err := manager.Verify(ctx, selection); err != nil {
		return err
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "BAFT integration installed successfully for %s\n", displayLabel(selection))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Restart the IDE to activate the plugin.")
	return nil
}

func displayLabel(ide integrations.IDEInstallation) string {
	if ide.Version == "" {
		return ide.DisplayName
	}
	return ide.DisplayName + " " + ide.Version
}

func promptForSelection(in io.Reader, out io.Writer, installations []integrations.IDEInstallation) (integrations.IDEInstallation, error) {
	reader := bufio.NewReader(in)
	for {
		fmt.Fprint(out, "\nSelect integration: ")
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if errors.Is(err, io.EOF) {
				return integrations.IDEInstallation{}, errors.New("no integration selected")
			}
			if err != nil {
				return integrations.IDEInstallation{}, err
			}
			continue
		}

		selection, convErr := strconv.Atoi(trimmed)
		if convErr == nil && selection >= 1 && selection <= len(installations) {
			return installations[selection-1], nil
		}

		fmt.Fprintf(out, "Enter a number between 1 and %d.\n", len(installations))
		if errors.Is(err, io.EOF) {
			return integrations.IDEInstallation{}, fmt.Errorf("invalid selection: %q", trimmed)
		}
		if err != nil {
			return integrations.IDEInstallation{}, err
		}
	}
}
