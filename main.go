package main

import (
	"embed"
	"os"

	"github.com/dariushalipour/baft/internal/cli"
)

var version string // set by -ldflags at build time

//go:embed docs/cli-assets docs/manual.md
var docs embed.FS

func main() {
	os.Exit(cli.Main(os.Args[1:], docs, version))
}
