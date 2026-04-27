// Package hotspots scores files by combining how often they change with how
// often those changes are bug fixes.
//
// score(file) = churn(file) * bugCommits(file)
//
// The package is independent of any VCS library: callers convert their input
// into []source.Commit (with Message populated) and pass it in.
package hotspots

import (
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dreiboxco/epo-core/internal/baseline/source"
)

// DefaultBugPattern is the case-insensitive regex used to classify a commit
// as a bug fix when no override is supplied. The shape captures three
// conventions:
//
//  1. Conventional Commits: "fix:", "fix(scope):", "fix!:", and the same
//     for bug/hotfix/revert/patch/regression at the line start.
//  2. GitHub auto-close: "Fixes #123", "Closes #45", "fix #7".
//  3. Freeform: a word-boundary mention of fix, bug, hotfix, or regression
//     anywhere in the message.
var DefaultBugPattern = regexp.MustCompile(
	`(?im)` +
		`^\s*(fix|bug|hotfix|revert|patch|regression)(\(|:|!|\s|$)` + // conventional + freeform start
		`|\bfix(es)?\s+#\d+` + // "Fixes #123"
		`|\bclos(e|es|ed)\s+#\d+` + // "Closes #45"
		`|\bhotfix\b` +
		`|\bregression\b`,
)

// Options configures an Analyze run.
type Options struct {
	// BugPattern overrides the default classifier. If nil, DefaultBugPattern
	// is used. The pattern must already be case-insensitive if that's the
	// desired behavior.
	BugPattern *regexp.Regexp
	// ComponentDepth groups results by leading path segments (see busfactor).
	ComponentDepth int
	// Ignore is a list of path prefixes to skip.
	Ignore []string
}

func (o Options) bugPattern() *regexp.Regexp {
	if o.BugPattern != nil {
		return o.BugPattern
	}
	return DefaultBugPattern
}

// FileResult is the per-file output.
type FileResult struct {
	Path        string
	Churn       int
	BugCommits  int
	Score       int
	FixRate     float64
	LastTouched time.Time
}

// ComponentResult aggregates files under a path prefix.
type ComponentResult struct {
	Path           string
	FilesAnalyzed  int
	TotalChurn     int
	TotalBugs      int
	ScoreSum       int
	FilesWithFixes int
}

// AuditExamples surfaces a few classified commit subjects so reviewers can
// sanity-check whether the regex picked the right things in their repo.
type AuditExamples struct {
	Fixes    []string
	NonFixes []string
}

// Report bundles the analysis output.
type Report struct {
	GeneratedAt       time.Time
	WindowStart       time.Time
	WindowEnd         time.Time
	CommitsAnalyzed   int
	FixCommits        int
	FixRate           float64
	FilesAnalyzed     int
	FilesTouchedByFix int
	Files             []FileResult // sorted by score desc, then churn desc
	Components        []ComponentResult
	Audit             AuditExamples
}

// Analyze walks commits and scores each file.
func Analyze(commits []source.Commit, opts Options) Report {
	pattern := opts.bugPattern()

	type stat struct {
		churn  int
		bugs   int
		latest time.Time
	}
	stats := make(map[string]*stat)

	var (
		windowStart, windowEnd time.Time
		fixCommits             int
		audit                  AuditExamples
	)

	const auditCap = 10

	for _, c := range commits {
		if windowStart.IsZero() || c.When.Before(windowStart) {
			windowStart = c.When
		}
		if c.When.After(windowEnd) {
			windowEnd = c.When
		}

		isFix := pattern.MatchString(c.Message)
		if isFix {
			fixCommits++
			if len(audit.Fixes) < auditCap {
				audit.Fixes = append(audit.Fixes, firstLine(c.Message))
			}
		} else if len(audit.NonFixes) < auditCap {
			audit.NonFixes = append(audit.NonFixes, firstLine(c.Message))
		}

		seen := make(map[string]struct{}, len(c.Files))
		for _, f := range c.Files {
			if shouldIgnore(f, opts.Ignore) {
				continue
			}
			if _, dup := seen[f]; dup {
				continue
			}
			seen[f] = struct{}{}

			s, ok := stats[f]
			if !ok {
				s = &stat{}
				stats[f] = s
			}
			s.churn++
			if isFix {
				s.bugs++
			}
			if c.When.After(s.latest) {
				s.latest = c.When
			}
		}
	}

	files := make([]FileResult, 0, len(stats))
	filesWithFix := 0
	for f, s := range stats {
		score := s.churn * s.bugs
		var fixRate float64
		if s.churn > 0 {
			fixRate = float64(s.bugs) / float64(s.churn)
		}
		if s.bugs > 0 {
			filesWithFix++
		}
		files = append(files, FileResult{
			Path:        f,
			Churn:       s.churn,
			BugCommits:  s.bugs,
			Score:       score,
			FixRate:     fixRate,
			LastTouched: s.latest,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Score != files[j].Score {
			return files[i].Score > files[j].Score
		}
		if files[i].Churn != files[j].Churn {
			return files[i].Churn > files[j].Churn
		}
		return files[i].Path < files[j].Path
	})

	components := aggregateComponents(files, opts.ComponentDepth)

	r := Report{
		GeneratedAt:       time.Now().UTC(),
		WindowStart:       windowStart,
		WindowEnd:         windowEnd,
		CommitsAnalyzed:   len(commits),
		FixCommits:        fixCommits,
		FilesAnalyzed:     len(files),
		FilesTouchedByFix: filesWithFix,
		Files:             files,
		Components:        components,
		Audit:             audit,
	}
	if len(commits) > 0 {
		r.FixRate = float64(fixCommits) / float64(len(commits))
	}
	return r
}

func aggregateComponents(files []FileResult, depth int) []ComponentResult {
	if len(files) == 0 {
		return nil
	}
	groups := make(map[string][]FileResult)
	for _, f := range files {
		groups[componentKey(f.Path, depth)] = append(groups[componentKey(f.Path, depth)], f)
	}
	out := make([]ComponentResult, 0, len(groups))
	for k, fs := range groups {
		var c ComponentResult
		c.Path = k
		c.FilesAnalyzed = len(fs)
		for _, f := range fs {
			c.TotalChurn += f.Churn
			c.TotalBugs += f.BugCommits
			c.ScoreSum += f.Score
			if f.BugCommits > 0 {
				c.FilesWithFixes++
			}
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ScoreSum != out[j].ScoreSum {
			return out[i].ScoreSum > out[j].ScoreSum
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func componentKey(filePath string, depth int) string {
	if depth <= 0 {
		return filePath
	}
	clean := path.Clean(filePath)
	parts := strings.Split(clean, "/")
	if len(parts) <= depth {
		if len(parts) == 1 {
			return "."
		}
		return path.Join(parts[:len(parts)-1]...)
	}
	return path.Join(parts[:depth]...)
}

func shouldIgnore(filePath string, ignore []string) bool {
	for _, p := range ignore {
		if p == "" {
			continue
		}
		if strings.HasPrefix(filePath, p) {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
