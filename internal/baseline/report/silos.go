package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/dreiboxco/epo-core/internal/baseline/silos"
)

// SilosOptions tunes the silos renderer.
type SilosOptions struct {
	Title     string
	RepoLabel string
	TopN      int
}

// RenderSilos writes the silos report as markdown to w.
func RenderSilos(w io.Writer, r silos.Report, opts SilosOptions) error {
	title := opts.Title
	if title == "" {
		title = "Knowledge Silos Report"
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
	sb.WriteString("Score formula: `churn / unique_contributors`\n\n")

	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Files analyzed | %d |\n", r.FilesAnalyzed)
	fmt.Fprintf(&sb, "| Files with single contributor | %d (%.1f%%) |\n", r.SingleContribFiles, r.SingleContribPct*100)
	fmt.Fprintf(&sb, "| Median contributors per file | %d |\n\n", r.MedianContributors)

	sb.WriteString(renderSilosFiles(r.Files, opts.TopN))
	sb.WriteString(renderSilosComponents(r.Components))

	_, err := io.WriteString(w, sb.String())
	return err
}

func renderSilosFiles(files []silos.FileResult, topN int) string {
	var sb strings.Builder
	heading := "## Top silos\n\n"
	if topN > 0 {
		heading = fmt.Sprintf("## Top %d silos\n\n", topN)
	}
	sb.WriteString(heading)

	if len(files) == 0 {
		sb.WriteString("_No files analyzed._\n\n")
		return sb.String()
	}

	sb.WriteString("| File | Score | Churn | Contributors | Last touched |\n")
	sb.WriteString("|---|---:|---:|---:|---|\n")

	limit := len(files)
	if topN > 0 && topN < limit {
		limit = topN
	}
	for i := 0; i < limit; i++ {
		f := files[i]
		fmt.Fprintf(&sb, "| `%s` | %.1f | %d | %d | %s |\n",
			f.Path, f.Score, f.Churn, f.Contributors,
			f.LastTouched.Format("2006-01-02"))
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderSilosComponents(components []silos.ComponentResult) string {
	var sb strings.Builder
	sb.WriteString("## Components by silo pressure\n\n")
	if len(components) == 0 {
		sb.WriteString("_No components aggregated._\n\n")
		return sb.String()
	}
	sb.WriteString("| Component | Files | Max score | Avg score | Single-contributor files |\n")
	sb.WriteString("|---|---:|---:|---:|---:|\n")
	for _, c := range components {
		fmt.Fprintf(&sb, "| `%s` | %d | %.1f | %.2f | %d |\n",
			c.Path, c.FilesAnalyzed, c.MaxScore, c.AvgScore, c.SingleContribFiles)
	}
	sb.WriteString("\n")
	return sb.String()
}
