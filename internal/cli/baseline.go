package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// errNotImplemented is returned by placeholder subcommands until their
// underlying analyzers are wired in.
var errNotImplemented = errors.New("not yet implemented")

func newBaselineCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "baseline",
		Short: "Capture and report baseline metrics for a codebase",
		Long: `Baseline subcommands measure a codebase before any EPO product is in place,
producing markdown reports that document the starting point of bus factor,
knowledge silos, hotspots, and basic DORA metrics.`,
	}

	cmd.AddCommand(newBaselineScanCommand())
	return cmd
}

func newBaselineScanCommand() *cobra.Command {
	var (
		path   string
		metric string
		out    string
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan a repository and emit a baseline report",
		Long: `Scan walks a git repository and computes the requested metric, writing
the result as markdown to --out (or stdout if not provided).

Supported metrics will be added incrementally. The first to land is "busfactor".`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = path
			_ = metric
			_ = out
			return errNotImplemented
		},
	}

	cmd.Flags().StringVar(&path, "path", ".", "path to the local git repository to scan")
	cmd.Flags().StringVar(&metric, "metric", "busfactor", "metric to compute (busfactor)")
	cmd.Flags().StringVar(&out, "out", "", "output file path (default: stdout)")

	return cmd
}
