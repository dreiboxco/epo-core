package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dreiboxco/epo-core/internal/baseline/age"
	"github.com/dreiboxco/epo-core/internal/baseline/busfactor"
	"github.com/dreiboxco/epo-core/internal/baseline/coupling"
	"github.com/dreiboxco/epo-core/internal/baseline/deploy"
	"github.com/dreiboxco/epo-core/internal/baseline/failure"
	gitsource "github.com/dreiboxco/epo-core/internal/baseline/git"
	"github.com/dreiboxco/epo-core/internal/baseline/hotspots"
	"github.com/dreiboxco/epo-core/internal/baseline/report"
	"github.com/dreiboxco/epo-core/internal/baseline/silos"
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
	path              string
	metric            string
	out               string
	since             string
	threshold         float64
	componentDepth    int
	ignore            []string
	top               int
	includeMerges     bool
	repoLabel         string
	bugPattern        string
	minSupport        int
	minConfidence     float64
	maxFilesPerCommit int
	maxCommitsPerFile int
	bucket            string
	ref               string
	excludeAuthors    []string
}

func newBaselineScanCommand() *cobra.Command {
	var f scanFlags

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan a repository and emit a baseline report",
		Long: `Scan walks a git repository and computes the requested metric, writing
the result as markdown to --out (or stdout if not provided).

Supported metrics:
  busfactor   per-file authorship concentration (default)
  hotspots    churn × bug-fix density per file
  silos       churn / unique contributors per file
  age         distribution of last-touched timestamps
  coupling    pairs of files that change together
  dora        Phase-1 DORA: deployment frequency + change failure rate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd.OutOrStdout(), f)
		},
	}

	cmd.Flags().StringVar(&f.path, "path", ".", "path to the local git repository to scan")
	cmd.Flags().StringVar(&f.metric, "metric", "busfactor", "metric to compute (busfactor, hotspots, silos, age, coupling, dora)")
	cmd.Flags().StringVar(&f.out, "out", "", "output file path (default: stdout)")
	cmd.Flags().StringVar(&f.since, "since", "12 months", "time window: '12 months', '6 weeks', '90 days', or a date YYYY-MM-DD")
	cmd.Flags().Float64Var(&f.threshold, "threshold", 0.5, "coverage threshold for bus factor (busfactor only)")
	cmd.Flags().IntVar(&f.componentDepth, "component-depth", 2, "path-segment depth for component aggregation")
	cmd.Flags().StringSliceVar(&f.ignore, "ignore",
		[]string{"vendor/", "node_modules/", "third_party/", "CHANGELOG.md"},
		"path prefixes to skip. Default ignores common dependency caches and root-level CHANGELOG.md "+
			"(catch-all that inflates churn and coupling without signal).")
	cmd.Flags().IntVar(&f.top, "top", 20, "show only the top N riskiest files (0 = all)")
	cmd.Flags().BoolVar(&f.includeMerges, "include-merges", false, "include merge commits in the analysis")
	cmd.Flags().StringVar(&f.repoLabel, "repo-label", "", "name to show in the report heading (default: derived from --path)")
	cmd.Flags().StringVar(&f.bugPattern, "bug-pattern", "", "override the regex used to classify bug-fix commits (hotspots only)")
	cmd.Flags().IntVar(&f.minSupport, "min-support", 0, "minimum co-occurrence count for a coupled pair to surface (coupling only; default 5)")
	cmd.Flags().Float64Var(&f.minConfidence, "min-confidence", 0, "minimum average bidirectional confidence (coupling only; default 0.5)")
	cmd.Flags().IntVar(&f.maxFilesPerCommit, "max-files-per-commit", 0, "skip commits touching more files than this (coupling only; default 50)")
	cmd.Flags().IntVar(&f.maxCommitsPerFile, "max-commits-per-file", 0, "treat files appearing in more commits than this as catch-all (coupling only; default 200)")
	cmd.Flags().StringVar(&f.bucket, "bucket", "weekly", "time-binning resolution for dora (daily, weekly, monthly)")
	cmd.Flags().StringVar(&f.ref, "ref", "", "branch, tag, or commit to walk from (default: HEAD). "+
		"Pass 'auto' to resolve the integration branch from origin/HEAD with fallback heuristic "+
		"(main → master → trunk → develop → development).")
	cmd.Flags().StringSliceVar(&f.excludeAuthors, "exclude-author", []string{`\[bot\]`},
		"regex patterns matched against author name and email; commits matching any pattern are dropped. "+
			"Default catches GitHub bot accounts (dependabot[bot], renovate[bot], github-actions[bot]). "+
			"Pass --exclude-author='' to disable.")

	return cmd
}

func runScan(stdout io.Writer, f scanFlags) error {
	since, err := parseSince(f.since)
	if err != nil {
		return fmt.Errorf("--since: %w", err)
	}

	excludeAuthors, err := compilePatterns(f.excludeAuthors)
	if err != nil {
		return fmt.Errorf("--exclude-author: %w", err)
	}

	ref := f.ref
	if strings.EqualFold(strings.TrimSpace(ref), "auto") {
		branch, resolveErr := gitsource.ResolveIntegrationBranch(f.path)
		if resolveErr != nil {
			return fmt.Errorf("--ref auto: %w", resolveErr)
		}
		ref = "origin/" + branch
		fmt.Fprintf(os.Stderr, "epo: --ref auto resolved to %s\n", ref)
	}

	commits, err := gitsource.Load(gitsource.LoadOptions{
		Path:           f.path,
		Ref:            ref,
		Since:          since,
		IncludeMerges:  f.includeMerges,
		ExcludeAuthors: excludeAuthors,
	})
	if err != nil {
		return err
	}

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

	switch f.metric {
	case "busfactor":
		r := busfactor.Analyze(commits, busfactor.Options{
			Threshold:      f.threshold,
			ComponentDepth: f.componentDepth,
			Ignore:         f.ignore,
		})
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

	case "hotspots":
		var pattern *regexp.Regexp
		if f.bugPattern != "" {
			pattern, err = regexp.Compile(f.bugPattern)
			if err != nil {
				return fmt.Errorf("--bug-pattern: %w", err)
			}
		}
		r := hotspots.Analyze(commits, hotspots.Options{
			BugPattern:     pattern,
			ComponentDepth: f.componentDepth,
			Ignore:         f.ignore,
		})
		if err := report.RenderHotspots(out, r, report.HotspotsOptions{
			RepoLabel: label,
			TopN:      f.top,
		}); err != nil {
			return err
		}
		if f.out != "" {
			fmt.Fprintf(stdout, "wrote %s (%d files, %d fix commits / %d total)\n",
				f.out, r.FilesAnalyzed, r.FixCommits, r.CommitsAnalyzed)
		}

	case "silos":
		r := silos.Analyze(commits, silos.Options{
			ComponentDepth: f.componentDepth,
			Ignore:         f.ignore,
		})
		if err := report.RenderSilos(out, r, report.SilosOptions{
			RepoLabel: label,
			TopN:      f.top,
		}); err != nil {
			return err
		}
		if f.out != "" {
			fmt.Fprintf(stdout, "wrote %s (%d files, %d single-contributor)\n",
				f.out, r.FilesAnalyzed, r.SingleContribFiles)
		}

	case "age":
		r := age.Analyze(commits, age.Options{
			ComponentDepth: f.componentDepth,
			Ignore:         f.ignore,
		})
		if err := report.RenderAge(out, r, report.AgeOptions{
			RepoLabel: label,
			TopN:      f.top,
		}); err != nil {
			return err
		}
		if f.out != "" {
			stale := 0
			if len(r.Buckets) > 0 {
				stale = r.Buckets[len(r.Buckets)-1].FileCount
			}
			fmt.Fprintf(stdout, "wrote %s (%d files, median age %dd, %d stale files >365d)\n",
				f.out, r.FilesAnalyzed, r.MedianAgeDays, stale)
		}

	case "coupling":
		r := coupling.Analyze(commits, coupling.Options{
			MinSupport:        f.minSupport,
			MinConfidence:     f.minConfidence,
			MaxFilesPerCommit: f.maxFilesPerCommit,
			MaxCommitsPerFile: f.maxCommitsPerFile,
			ComponentDepth:    f.componentDepth,
			Ignore:            f.ignore,
		})
		if err := report.RenderCoupling(out, r, report.CouplingOptions{
			RepoLabel: label,
			TopN:      f.top,
		}); err != nil {
			return err
		}
		if f.out != "" {
			fmt.Fprintf(stdout, "wrote %s (%d pairs, %d catch-all files excluded, %d mass-edit commits excluded)\n",
				f.out, len(r.Pairs), len(r.FilesExcludedAsCatchall), r.CommitsExcluded)
		}

	case "dora":
		bucket, err := parseBucket(f.bucket)
		if err != nil {
			return err
		}
		var pattern *regexp.Regexp
		if f.bugPattern != "" {
			pattern, err = regexp.Compile(f.bugPattern)
			if err != nil {
				return fmt.Errorf("--bug-pattern: %w", err)
			}
		}
		dep := deploy.Analyze(commits, deploy.Options{
			Bucket: bucket,
			Ignore: f.ignore,
		})
		fail := failure.Analyze(commits, failure.Options{
			Bucket:     bucket,
			BugPattern: pattern,
			Ignore:     f.ignore,
		})
		if err := report.RenderDora(out, dep, fail, report.DoraOptions{
			RepoLabel: label,
			TopN:      f.top,
		}); err != nil {
			return err
		}
		if f.out != "" {
			fmt.Fprintf(stdout, "wrote %s (deploys: %d / %d buckets, failure rate: %.1f%%)\n",
				f.out, dep.TotalCommits, dep.BucketsCount, fail.OverallRate*100)
		}

	default:
		return fmt.Errorf("metric %q is not supported (use: busfactor, hotspots, silos, age, coupling, dora)", f.metric)
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

// compilePatterns turns user-supplied regex strings into compiled patterns.
// Empty strings are dropped so `--exclude-author=` cleanly disables the default.
func compilePatterns(raw []string) ([]*regexp.Regexp, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]*regexp.Regexp, 0, len(raw))
	for _, p := range raw {
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// parseBucket maps a string flag value to the deploy.Bucket constant.
func parseBucket(s string) (deploy.Bucket, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "weekly", "week":
		return deploy.BucketWeekly, nil
	case "daily", "day":
		return deploy.BucketDaily, nil
	case "monthly", "month":
		return deploy.BucketMonthly, nil
	}
	return "", fmt.Errorf("--bucket: unknown value %q (use daily, weekly, monthly)", s)
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
