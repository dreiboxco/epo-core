package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dreiboxco/epo-core/internal/baseline/busfactor"
	gitsource "github.com/dreiboxco/epo-core/internal/baseline/git"
	"github.com/dreiboxco/epo-core/internal/baseline/report"
)

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

type scanFlags struct {
	path           string
	metric         string
	out            string
	since          string
	threshold      float64
	componentDepth int
	ignore         []string
	top            int
	includeMerges  bool
	repoLabel      string
}

func newBaselineScanCommand() *cobra.Command {
	var f scanFlags

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan a repository and emit a baseline report",
		Long: `Scan walks a git repository and computes the requested metric, writing
the result as markdown to --out (or stdout if not provided).

Supported metrics:
  busfactor   per-file authorship concentration (default)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd.OutOrStdout(), f)
		},
	}

	cmd.Flags().StringVar(&f.path, "path", ".", "path to the local git repository to scan")
	cmd.Flags().StringVar(&f.metric, "metric", "busfactor", "metric to compute (busfactor)")
	cmd.Flags().StringVar(&f.out, "out", "", "output file path (default: stdout)")
	cmd.Flags().StringVar(&f.since, "since", "12 months", "time window: '12 months', '6 weeks', '90 days', or a date YYYY-MM-DD")
	cmd.Flags().Float64Var(&f.threshold, "threshold", 0.5, "coverage threshold for bus factor")
	cmd.Flags().IntVar(&f.componentDepth, "component-depth", 2, "path-segment depth for component aggregation")
	cmd.Flags().StringSliceVar(&f.ignore, "ignore", []string{"vendor/", "node_modules/", "third_party/"}, "path prefixes to skip")
	cmd.Flags().IntVar(&f.top, "top", 20, "show only the top N riskiest files (0 = all)")
	cmd.Flags().BoolVar(&f.includeMerges, "include-merges", false, "include merge commits in the analysis")
	cmd.Flags().StringVar(&f.repoLabel, "repo-label", "", "name to show in the report heading (default: derived from --path)")

	return cmd
}

func runScan(stdout io.Writer, f scanFlags) error {
	if f.metric != "busfactor" {
		return fmt.Errorf("metric %q is not supported yet (only busfactor)", f.metric)
	}

	since, err := parseSince(f.since)
	if err != nil {
		return fmt.Errorf("--since: %w", err)
	}

	commits, err := gitsource.Load(gitsource.LoadOptions{
		Path:          f.path,
		Since:         since,
		IncludeMerges: f.includeMerges,
	})
	if err != nil {
		return err
	}

	r := busfactor.Analyze(commits, busfactor.Options{
		Threshold:      f.threshold,
		ComponentDepth: f.componentDepth,
		Ignore:         f.ignore,
	})

	out, closer, err := openOutput(f.out)
	if err != nil {
		return err
	}
	defer closer()

	label := f.repoLabel
	if label == "" {
		abs, absErr := filepath.Abs(f.path)
		if absErr == nil {
			label = filepath.Base(abs)
		}
	}

	if err := report.RenderMarkdown(out, r, report.MarkdownOptions{
		RepoLabel: label,
		TopN:      f.top,
	}); err != nil {
		return err
	}

	if f.out != "" {
		fmt.Fprintf(stdout, "wrote %s (%d files analyzed, %d with bus factor 1)\n",
			f.out, r.FilesAnalyzed, r.BusFactorOneFiles)
	}
	return nil
}

func openOutput(path string) (io.Writer, func(), error) {
	if path == "" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

// parseSince accepts:
//   - "" or "0"     → zero time (no filter)
//   - YYYY-MM-DD    → that calendar date, UTC midnight
//   - "N units"     → relative offset back from now (units: day(s)/week(s)/month(s)/year(s))
func parseSince(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return time.Time{}, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}

	parts := strings.Fields(s)
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("expected 'N unit' or YYYY-MM-DD, got %q", s)
	}
	var n int
	if _, err := fmt.Sscanf(parts[0], "%d", &n); err != nil {
		return time.Time{}, fmt.Errorf("invalid number in %q: %w", s, err)
	}

	unit := strings.TrimSuffix(strings.ToLower(parts[1]), "s")
	now := time.Now()
	switch unit {
	case "day":
		return now.AddDate(0, 0, -n), nil
	case "week":
		return now.AddDate(0, 0, -n*7), nil
	case "month":
		return now.AddDate(0, -n, 0), nil
	case "year":
		return now.AddDate(-n, 0, 0), nil
	}
	return time.Time{}, fmt.Errorf("unknown unit %q in %q", parts[1], s)
}
