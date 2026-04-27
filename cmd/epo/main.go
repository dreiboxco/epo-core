// Command epo is the CLI entrypoint of EPO (Engineering Platform Orchestration).
//
// It currently exposes a single, in-development subcommand: `baseline scan`,
// which is the first dogfooding target of the project.
package main

import (
	"fmt"
	"os"

	"github.com/dreiboxco/epo-core/internal/cli"
)

// Version is set at build time via -ldflags.
var Version = "v0.0.1-dev"

func main() {
	root := cli.NewRootCommand(Version)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "epo: %v\n", err)
		os.Exit(1)
	}
}
