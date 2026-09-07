// Package cli parses baft's command line and dispatches to the use cases.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime/debug"
	"strings"
)

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

type app struct {
	in      io.Reader
	out     io.Writer
	errOut  io.Writer
	docs    fs.FS
	version string
	color   bool
}

// Main runs baft with the process streams. docs carries the embedded
// docs/ tree (help assets and manual).
func Main(args []string, docs fs.FS, version string) int {
	a := &app{in: os.Stdin, out: os.Stdout, errOut: os.Stderr, docs: docs, version: version, color: colorEnabled(os.Stdout)}
	return a.run(args)
}

func (a *app) run(args []string) int {
	if len(args) == 0 {
		args = []string{"--help"}
	}
	switch cmd := args[0]; cmd {
	case "--help", "-h":
		fmt.Fprint(a.out, a.doc("cli-assets/help-intro.txt"), "\n")
		a.help("")
	case "--version", "-v":
		fmt.Fprintln(a.out, a.cliVersion())
	case "check":
		return a.check(args[1:])
	case "dump":
		return a.dump(args[1:])
	case "restyle":
		return a.restyle(args[1:])
	case "integrate":
		return a.integrate(args[1:])
	case "manual":
		return a.manual(args[1:])
	default:
		fmt.Fprintf(a.errOut, "unknown command: %s\n\nRun 'baft --help' for usage\n", cmd)
		return exitUsage
	}
	return exitOK
}

func (a *app) doc(name string) string {
	b, err := fs.ReadFile(a.docs, "docs/"+name)
	if err != nil {
		return ""
	}
	return string(b)
}

// help prints the usage text of a subcommand, or the root usage when cmd is "".
func (a *app) help(cmd string) int {
	if cmd != "" {
		cmd += "-"
	}
	fmt.Fprint(a.out, a.doc("cli-assets/"+cmd+"usage.txt"))
	return exitOK
}

func (a *app) fail(format string, args ...interface{}) int {
	fmt.Fprintf(a.errOut, format+"\n", args...)
	return exitFail
}

func (a *app) usageErr(cmd string, err error) int {
	fmt.Fprintf(a.errOut, "%v\n\nRun 'baft %s --help' for usage\n", err, cmd)
	return exitUsage
}

func (a *app) cliVersion() string {
	if a.version != "" {
		return a.version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

func newFlagSet(cmd string) *flag.FlagSet {
	fset := flag.NewFlagSet("baft "+cmd, flag.ContinueOnError)
	fset.SetOutput(io.Discard)
	fset.Usage = func() {}
	return fset
}

// parse accepts flags and operands in any order and returns the operands.
func parse(fset *flag.FlagSet, args []string) ([]string, error) {
	var operands []string
	for {
		if err := fset.Parse(args); err != nil {
			return nil, err
		}
		if fset.NArg() == 0 {
			return operands, nil
		}
		operands = append(operands, fset.Arg(0))
		args = fset.Args()[1:]
	}
}

// parsed reports the exit code for a failed parse: 0 after printing help,
// 2 for a usage error.
func (a *app) parsed(cmd string, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return a.help(cmd)
	}
	return a.usageErr(cmd, err)
}

// rootDir returns the single optional root-dir operand.
func rootDir(operands []string) (string, error) {
	switch len(operands) {
	case 0:
		return ".", nil
	case 1:
		return operands[0], nil
	default:
		return "", fmt.Errorf("unexpected argument: %s", operands[1])
	}
}

func noOperands(operands []string) error {
	if len(operands) > 0 {
		return fmt.Errorf("unexpected argument: %s", operands[0])
	}
	return nil
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func colorEnabled(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
