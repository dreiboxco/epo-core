package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/dreiboxco/epo-core/internal/baseline/deploy"
	"github.com/dreiboxco/epo-core/internal/baseline/failure"
)

// DoraOptions tunes the combined DORA-Phase-1 renderer.
type DoraOptions struct {
	Title     string
	RepoLabel string
	TopN      int // top N worst buckets
}

// RenderDora writes a combined Deployment Frequency + Change Failure Rate
// report as markdown to w.
func RenderDora(w io.Writer, dep deploy.Report, fail failure.Report, opts DoraOptions) error {
	title := opts.Title
	if title == "" {
		title = "DORA Phase-1 Report"
	}

	var sb strings.Builder

	if opts.RepoLabel != "" {
		fmt.Fprintf(&sb, "# %s — %s\n\n", title, opts.RepoLabel)
	} else {
		fmt.Fprintf(&sb, "# %s\n\n", title)
	}
	fmt.Fprintf(&sb, "Generated: %s  \n", dep.GeneratedAt.Format("2006-01-02 15:04 UTC"))
	if !dep.WindowStart.IsZero() {
		fmt.Fprintf(&sb, "Window: %s → %s  \n",
			dep.WindowStart.Format("2006-01-02"),
			dep.WindowEnd.Format("2006-01-02"))
	}
	fmt.Fprintf(&sb, "Bucket: %s\n\n", dep.Bucket)

	sb.WriteString(renderDeploy(dep))
	sb.WriteString(renderFailure(fail, opts.TopN))

	_, err := io.WriteString(w, sb.String())
	return err
}

func renderDeploy(r deploy.Report) string {
	var sb strings.Builder
	sb.WriteString("## Deployment Frequency\n\n")
	sb.WriteString("| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Total commits in window | %d |\n", r.TotalCommits)
	fmt.Fprintf(&sb, "| Buckets analyzed | %d |\n", r.BucketsCount)
	fmt.Fprintf(&sb, "| Mean per bucket | %.2f |\n", r.MeanPerBucket)
	fmt.Fprintf(&sb, "| Median per bucket | %d |\n", r.MedianPerBucket)
	fmt.Fprintf(&sb, "| Last bucket | %d |\n\n", r.LastBucketCount)

	if len(r.Buckets) == 0 {
		sb.WriteString("_No buckets to display._\n\n")
		return sb.String()
	}

	max := 0
	for _, b := range r.Buckets {
		if b.Count > max {
			max = b.Count
		}
	}
	sb.WriteString("```\n")
	for _, b := range r.Buckets {
		bar := strings.Repeat("█", scaleAgeBar(b.Count, max, 40))
		fmt.Fprintf(&sb, "%s │ %s %d\n", b.Start.Format("2006-01-02"), bar, b.Count)
	}
	sb.WriteString("```\n\n")
	return sb.String()
}

func renderFailure(r failure.Report, topN int) string {
	var sb strings.Builder
	sb.WriteString("## Change Failure Rate (heuristic)\n\n")
	sb.WriteString("> Heuristic proxy: `commits classified as fix / total commits`. The rigorous DORA definition requires deploy success/failure data, which Git alone does not provide. Treat this as a directional signal.\n\n")
	sb.WriteString("| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Total commits | %d |\n", r.TotalCommits)
	fmt.Fprintf(&sb, "| Fix commits | %d |\n", r.FixCommits)
	fmt.Fprintf(&sb, "| Overall failure rate | %.1f%% |\n\n", r.OverallRate*100)

	sb.WriteString("### Per bucket\n\n")
	if len(r.Buckets) == 0 {
		sb.WriteString("_No buckets to display._\n\n")
		return sb.String()
	}
	sb.WriteString("| Bucket | Total | Fixes | Failure rate |\n|---|---:|---:|---:|\n")
	for _, b := range r.Buckets {
		fmt.Fprintf(&sb, "| %s | %d | %d | %.1f%% |\n",
			b.Start.Format("2006-01-02"), b.Total, b.Fixes, b.FailRate*100)
	}
	sb.WriteString("\n")

	heading := "### Worst buckets by failure rate\n\n"
	if topN > 0 {
		heading = fmt.Sprintf("### Top %d worst buckets by failure rate\n\n", topN)
	}
	sb.WriteString(heading)
	if len(r.WorstBuckets) == 0 {
		sb.WriteString("_(none)_\n\n")
		return sb.String()
	}
	sb.WriteString("| Bucket | Total | Fixes | Failure rate |\n|---|---:|---:|---:|\n")
	limit := len(r.WorstBuckets)
	if topN > 0 && topN < limit {
		limit = topN
	}
	for i := 0; i < limit; i++ {
		b := r.WorstBuckets[i]
		fmt.Fprintf(&sb, "| %s | %d | %d | %.1f%% |\n",
			b.Start.Format("2006-01-02"), b.Total, b.Fixes, b.FailRate*100)
	}
	sb.WriteString("\n")
	return sb.String()
}
