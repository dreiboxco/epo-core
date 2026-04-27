package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/dreiboxco/epo-core/internal/baseline/coupling"
)

// CouplingOptions tunes the coupling renderer.
type CouplingOptions struct {
	Title     string
	RepoLabel string
	TopN      int
}

// RenderCoupling writes the coupling report as markdown to w.
func RenderCoupling(w io.Writer, r coupling.Report, opts CouplingOptions) error {
	title := opts.Title
	if title == "" {
		title = "Temporal Coupling Report"
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
	sb.WriteString("Definition: pair surfaces when both files change in the same commit.\n\n")

	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Commits analyzed | %d |\n", r.CommitsAnalyzed)
	fmt.Fprintf(&sb, "| Mass-edit commits excluded | %d |\n", r.CommitsExcluded)
	fmt.Fprintf(&sb, "| Catch-all files excluded | %d |\n", len(r.FilesExcludedAsCatchall))
	fmt.Fprintf(&sb, "| Coupled pairs surfaced | %d |\n\n", len(r.Pairs))

	sb.WriteString(renderPairs(r.Pairs, opts.TopN))
	sb.WriteString(renderComponentPairs(r.ComponentPairs))
	sb.WriteString(renderExcludedFiles(r.FilesExcludedAsCatchall))

	_, err := io.WriteString(w, sb.String())
	return err
}

func renderPairs(pairs []coupling.PairResult, topN int) string {
	var sb strings.Builder
	heading := "## Top coupled pairs\n\n"
	if topN > 0 {
		heading = fmt.Sprintf("## Top %d coupled pairs\n\n", topN)
	}
	sb.WriteString(heading)
	if len(pairs) == 0 {
		sb.WriteString("_No pairs cleared the support and confidence thresholds._\n\n")
		return sb.String()
	}

	sb.WriteString("| File A | File B | Co-occur | Conf A→B | Conf B→A | Conf avg |\n")
	sb.WriteString("|---|---|---:|---:|---:|---:|\n")
	limit := len(pairs)
	if topN > 0 && topN < limit {
		limit = topN
	}
	for i := 0; i < limit; i++ {
		p := pairs[i]
		fmt.Fprintf(&sb, "| `%s` | `%s` | %d | %.1f%% | %.1f%% | %.1f%% |\n",
			p.FileA, p.FileB, p.Support,
			p.ConfidenceAtoB*100, p.ConfidenceBtoA*100, p.ConfidenceAvg*100)
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderComponentPairs(pairs []coupling.ComponentPair) string {
	var sb strings.Builder
	sb.WriteString("## Component-pair pressure\n\n")
	if len(pairs) == 0 {
		sb.WriteString("_No component pairs to aggregate._\n\n")
		return sb.String()
	}
	sb.WriteString("| Component A | Component B | Pair count | Total co-occur |\n|---|---|---:|---:|\n")
	for _, c := range pairs {
		fmt.Fprintf(&sb, "| `%s` | `%s` | %d | %d |\n",
			c.CompA, c.CompB, c.PairCount, c.TotalSupport)
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderExcludedFiles(files []string) string {
	var sb strings.Builder
	sb.WriteString("## Catch-all files excluded\n\n")
	if len(files) == 0 {
		sb.WriteString("_(none — no file exceeded `--max-commits-per-file`)_\n\n")
		return sb.String()
	}
	sb.WriteString("These files were touched too often to produce useful coupling signal. Reviewers should sanity-check they're truly catch-all (CHANGELOG, schema, lockfiles) and not architectural hubs the analyzer should keep.\n\n")
	for _, f := range files {
		fmt.Fprintf(&sb, "- `%s`\n", f)
	}
	sb.WriteString("\n")
	return sb.String()
}
