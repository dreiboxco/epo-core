package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/dreiboxco/epo-core/internal/baseline/age"
)

// AgeOptions tunes the code-age renderer.
type AgeOptions struct {
	Title     string
	RepoLabel string
	TopN      int
}

// RenderAge writes the age report as markdown to w.
func RenderAge(w io.Writer, r age.Report, opts AgeOptions) error {
	title := opts.Title
	if title == "" {
		title = "Code Age Report"
	}

	var sb strings.Builder

	if opts.RepoLabel != "" {
		fmt.Fprintf(&sb, "# %s — %s\n\n", title, opts.RepoLabel)
	} else {
		fmt.Fprintf(&sb, "# %s\n\n", title)
	}
	fmt.Fprintf(&sb, "Generated: %s  \n", r.GeneratedAt.Format("2006-01-02 15:04 UTC"))
	if !r.WindowStart.IsZero() {
		fmt.Fprintf(&sb, "Window: %s → %s  \n",
			r.WindowStart.Format("2006-01-02"),
			r.WindowEnd.Format("2006-01-02"))
	}
	fmt.Fprintf(&sb, "Reference time (now): %s\n\n", r.Now.Format("2006-01-02 15:04 UTC"))

	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Files analyzed | %d |\n", r.FilesAnalyzed)
	fmt.Fprintf(&sb, "| Median age | %d days |\n", r.MedianAgeDays)
	fmt.Fprintf(&sb, "| Oldest file age | %d days |\n", r.OldestAgeDays)
	fmt.Fprintf(&sb, "| Newest file age | %d days |\n\n", r.NewestAgeDays)

	sb.WriteString(renderAgeDistribution(r.Buckets))
	sb.WriteString(renderStalestFiles(r.Files, opts.TopN))
	sb.WriteString(renderAgeComponents(r.Components))

	_, err := io.WriteString(w, sb.String())
	return err
}

func renderAgeDistribution(buckets []age.Bucket) string {
	var sb strings.Builder
	sb.WriteString("## Age distribution\n\n")
	if len(buckets) == 0 {
		sb.WriteString("_No buckets configured._\n\n")
		return sb.String()
	}

	maxCount := 0
	for _, b := range buckets {
		if b.FileCount > maxCount {
			maxCount = b.FileCount
		}
	}

	sb.WriteString("```\n")
	for _, b := range buckets {
		bar := strings.Repeat("█", scaleAgeBar(b.FileCount, maxCount, 40))
		fmt.Fprintf(&sb, "%-9s │ %s %d\n", b.Label, bar, b.FileCount)
	}
	sb.WriteString("```\n\n")
	return sb.String()
}

func renderStalestFiles(files []age.FileResult, topN int) string {
	var sb strings.Builder
	heading := "## Stalest files\n\n"
	if topN > 0 {
		heading = fmt.Sprintf("## Top %d stalest files\n\n", topN)
	}
	sb.WriteString(heading)

	if len(files) == 0 {
		sb.WriteString("_No files analyzed._\n\n")
		return sb.String()
	}

	sb.WriteString("| File | Age (days) | Last touched |\n|---|---:|---|\n")
	limit := len(files)
	if topN > 0 && topN < limit {
		limit = topN
	}
	for i := 0; i < limit; i++ {
		f := files[i]
		fmt.Fprintf(&sb, "| `%s` | %d | %s |\n", f.Path, f.AgeDays, f.LastTouched.Format("2006-01-02"))
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderAgeComponents(components []age.ComponentResult) string {
	var sb strings.Builder
	sb.WriteString("## Components by staleness\n\n")
	if len(components) == 0 {
		sb.WriteString("_No components aggregated._\n\n")
		return sb.String()
	}
	sb.WriteString("| Component | Files | Median age | Oldest age | Stale files |\n|---|---:|---:|---:|---:|\n")
	for _, c := range components {
		fmt.Fprintf(&sb, "| `%s` | %d | %d | %d | %d |\n",
			c.Path, c.FilesAnalyzed, c.MedianAgeDays, c.OldestAgeDays, c.StaleFiles)
	}
	sb.WriteString("\n")
	return sb.String()
}

func scaleAgeBar(v, max, width int) int {
	if max == 0 {
		return 0
	}
	scaled := v * width / max
	if scaled == 0 && v > 0 {
		return 1
	}
	return scaled
}
