// Package cli wires together the cobra command tree for the epo binary.
package cli

import "github.com/spf13/cobra"

// NewRootCommand returns the top-level `epo` command with all subcommands attached.
func NewRootCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "epo",
		Short:         "EPO — orchestrate the engineering toolchain",
		Long:          "EPO is the orchestration layer for engineering platforms.\nThis CLI is its entrypoint for analysis, catalog, and adapter operations.",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(newBaselineCommand())
	return root
}
