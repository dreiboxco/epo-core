// Package report renders a busfactor.Report as markdown suitable for
// inclusion in a baseline document.
package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dreiboxco/epo-core/internal/baseline/busfactor"
)

// MarkdownOptions tunes the renderer output.
type MarkdownOptions struct {
	// Title is the H1 heading. Defaults to "Bus Factor Report".
	Title string
	// RepoLabel is shown in the heading after the title (e.g. "epo-core").
	RepoLabel string
	// TopN bounds the size of the per-file ranking; 0 means show all.
	TopN int
}

// RenderMarkdown writes the report as markdown to w.
func RenderMarkdown(w io.Writer, r busfactor.Report, opts MarkdownOptions) error {
	title := opts.Title
	if title == "" {
		title = "Bus Factor Report"
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
	fmt.Fprintf(&sb, "Threshold: %.0f%%\n\n", r.Threshold*100)

	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Files analyzed | %d |\n", r.FilesAnalyzed)
	fmt.Fprintf(&sb, "| Files with bus factor 1 | %d (%.1f%%) |\n", r.BusFactorOneFiles, r.BusFactorOnePct*100)
	fmt.Fprintf(&sb, "| Median file bus factor | %d |\n", r.MedianFileBusFact)
	fmt.Fprintf(&sb, "| Components analyzed | %d |\n", len(r.Components))
	componentBFOne := 0
	for _, c := range r.Components {
		if c.MinBusFactor == 1 {
			componentBFOne++
		}
	}
	fmt.Fprintf(&sb, "| Components with min bus factor 1 | %d |\n\n", componentBFOne)

	sb.WriteString(renderFiles(r.Files, opts.TopN))
	sb.WriteString(renderComponents(r.Components))
	sb.WriteString(renderDistribution(r.Distribution))

	_, err := io.WriteString(w, sb.String())
	return err
}

func renderFiles(files []busfactor.FileResult, topN int) string {
	var sb strings.Builder
	heading := "## Top single-point-of-failure files\n\n"
	if topN > 0 {
		heading = fmt.Sprintf("## Top %d single-point-of-failure files\n\n", topN)
	}
	sb.WriteString(heading)

	if len(files) == 0 {
		sb.WriteString("_No files analyzed._\n\n")
		return sb.String()
	}

	sb.WriteString("| File | Bus factor | Top author | Top share | Commits | Last touched |\n")
	sb.WriteString("|---|---:|---|---:|---:|---|\n")

	limit := len(files)
	if topN > 0 && topN < limit {
		limit = topN
	}
	for i := 0; i < limit; i++ {
		f := files[i]
		fmt.Fprintf(&sb, "| `%s` | %d | %s | %.1f%% | %d | %s |\n",
			f.Path, f.BusFactor, f.TopAuthor, f.TopShare*100, f.Commits,
			f.LastTouched.Format("2006-01-02"))
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderComponents(components []busfactor.ComponentResult) string {
	var sb strings.Builder
	sb.WriteString("## Components by risk\n\n")
	if len(components) == 0 {
		sb.WriteString("_No components aggregated._\n\n")
		return sb.String()
	}
	sb.WriteString("| Component | Files | Min bus factor | Median bus factor | BF=1 files |\n")
	sb.WriteString("|---|---:|---:|---:|---:|\n")
	for _, c := range components {
		fmt.Fprintf(&sb, "| `%s` | %d | %d | %d | %d |\n",
			c.Path, c.FilesAnalyzed, c.MinBusFactor, c.MedianBusFactor, c.BusFactorOneCnt)
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderDistribution(dist map[int]int) string {
	var sb strings.Builder
	sb.WriteString("## Bus factor distribution\n\n")
	if len(dist) == 0 {
		sb.WriteString("_No data._\n\n")
		return sb.String()
	}

	keys := make([]int, 0, len(dist))
	max := 0
	for k, v := range dist {
		keys = append(keys, k)
		if v > max {
			max = v
		}
	}
	sort.Ints(keys)

	sb.WriteString("```\n")
	for _, k := range keys {
		bar := strings.Repeat("█", scaleBar(dist[k], max, 40))
		fmt.Fprintf(&sb, "BF %2d │ %s %d\n", k, bar, dist[k])
	}
	sb.WriteString("```\n")
	return sb.String()
}

func scaleBar(v, max, width int) int {
	if max == 0 {
		return 0
	}
	scaled := v * width / max
	if scaled == 0 && v > 0 {
		return 1
	}
	return scaled
}
