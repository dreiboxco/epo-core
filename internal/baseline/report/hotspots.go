package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/dreiboxco/epo-core/internal/baseline/hotspots"
)

// HotspotsOptions tunes the hotspots renderer.
type HotspotsOptions struct {
	Title     string // default: "Hotspots Report"
	RepoLabel string
	TopN      int
}

// RenderHotspots writes the hotspots report as markdown to w.
func RenderHotspots(w io.Writer, r hotspots.Report, opts HotspotsOptions) error {
	title := opts.Title
	if title == "" {
		title = "Hotspots Report"
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
	sb.WriteString("Score formula: `churn × bug_commits`\n\n")

	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Commits analyzed | %d |\n", r.CommitsAnalyzed)
	fmt.Fprintf(&sb, "| Commits classified as fix | %d (%.1f%%) |\n", r.FixCommits, r.FixRate*100)
	fmt.Fprintf(&sb, "| Files analyzed | %d |\n", r.FilesAnalyzed)
	fmt.Fprintf(&sb, "| Files touched by ≥1 fix | %d |\n\n", r.FilesTouchedByFix)

	sb.WriteString(renderHotspotFiles(r.Files, opts.TopN))
	sb.WriteString(renderHotspotComponents(r.Components))
	sb.WriteString(renderAudit(r.Audit))

	_, err := io.WriteString(w, sb.String())
	return err
}

func renderHotspotFiles(files []hotspots.FileResult, topN int) string {
	var sb strings.Builder
	heading := "## Top hotspots\n\n"
	if topN > 0 {
		heading = fmt.Sprintf("## Top %d hotspots\n\n", topN)
	}
	sb.WriteString(heading)

	if len(files) == 0 {
		sb.WriteString("_No files analyzed._\n\n")
		return sb.String()
	}

	sb.WriteString("| File | Score | Churn | Fix commits | Fix rate | Last touched |\n")
	sb.WriteString("|---|---:|---:|---:|---:|---|\n")

	limit := len(files)
	if topN > 0 && topN < limit {
		limit = topN
	}
	for i := 0; i < limit; i++ {
		f := files[i]
		fmt.Fprintf(&sb, "| `%s` | %d | %d | %d | %.1f%% | %s |\n",
			f.Path, f.Score, f.Churn, f.BugCommits, f.FixRate*100,
			f.LastTouched.Format("2006-01-02"))
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderHotspotComponents(components []hotspots.ComponentResult) string {
	var sb strings.Builder
	sb.WriteString("## Components by hotspot pressure\n\n")
	if len(components) == 0 {
		sb.WriteString("_No components aggregated._\n\n")
		return sb.String()
	}
	sb.WriteString("| Component | Files | Churn | Fix commits | Score sum | Files w/ fixes |\n")
	sb.WriteString("|---|---:|---:|---:|---:|---:|\n")
	for _, c := range components {
		fmt.Fprintf(&sb, "| `%s` | %d | %d | %d | %d | %d |\n",
			c.Path, c.FilesAnalyzed, c.TotalChurn, c.TotalBugs, c.ScoreSum, c.FilesWithFixes)
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderAudit(audit hotspots.AuditExamples) string {
	var sb strings.Builder
	sb.WriteString("## Bug-classification audit\n\n")
	sb.WriteString("Sample of commits to sanity-check the regex. If something looks off, override `--bug-pattern`.\n\n")
	sb.WriteString("**Classified as fix:**\n\n")
	if len(audit.Fixes) == 0 {
		sb.WriteString("_(none)_\n\n")
	} else {
		for _, m := range audit.Fixes {
			fmt.Fprintf(&sb, "- %s\n", escape(m))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("**Classified as non-fix:**\n\n")
	if len(audit.NonFixes) == 0 {
		sb.WriteString("_(none)_\n\n")
	} else {
		for _, m := range audit.NonFixes {
			fmt.Fprintf(&sb, "- %s\n", escape(m))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// escape minimizes markdown breakage from commit subjects that contain pipes
// or backticks. Cheap and good enough for an audit list.
func escape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "`", "\\`")
	return s
}
