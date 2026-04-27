// Package silos scores files by how concentrated their ownership is given
// how often they change. The score is churn divided by unique contributors:
//
//	score(file) = churn(file) / unique_contributors(file)
//
// A file that changes 30 times with one author scores 30. The same 30
// commits across 5 contributors scores 6. A file with 1 commit scores ≤ 1
// regardless of ownership. The metric surfaces lonely-but-active files —
// the classic "knowledge silo" pattern.
package silos

import (
	"path"
	"sort"
	"strings"
	"time"

	"github.com/dreiboxco/epo-core/internal/baseline/source"
)

// Options configures an Analyze run.
type Options struct {
	// ComponentDepth is the path-segment depth used for aggregation.
	ComponentDepth int
	// Ignore is a list of path prefixes to skip.
	Ignore []string
}

// FileResult is the per-file output.
type FileResult struct {
	Path         string
	Churn        int
	Contributors int
	Score        float64
	LastTouched  time.Time
}

// ComponentResult aggregates files under a path prefix.
type ComponentResult struct {
	Path               string
	FilesAnalyzed      int
	SingleContribFiles int
	AvgScore           float64
	MaxScore           float64
}

// Report bundles the analysis output.
type Report struct {
	GeneratedAt        time.Time
	WindowStart        time.Time
	WindowEnd          time.Time
	FilesAnalyzed      int
	SingleContribFiles int
	SingleContribPct   float64
	MedianContributors int
	Files              []FileResult
	Components         []ComponentResult
}

// Analyze walks commits and computes the silo score for each file.
func Analyze(commits []source.Commit, opts Options) Report {
	type stat struct {
		churn   int
		authors map[string]struct{}
		latest  time.Time
	}
	stats := make(map[string]*stat)

	var windowStart, windowEnd time.Time

	for _, c := range commits {
		if windowStart.IsZero() || c.When.Before(windowStart) {
			windowStart = c.When
		}
		if c.When.After(windowEnd) {
			windowEnd = c.When
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
				s = &stat{authors: make(map[string]struct{})}
				stats[f] = s
			}
			s.churn++
			s.authors[c.Author] = struct{}{}
			if c.When.After(s.latest) {
				s.latest = c.When
			}
		}
	}

	files := make([]FileResult, 0, len(stats))
	singleContrib := 0
	for f, s := range stats {
		contributors := len(s.authors)
		if contributors == 0 {
			contributors = 1 // defensive; should never happen
		}
		score := float64(s.churn) / float64(contributors)
		if contributors == 1 {
			singleContrib++
		}
		files = append(files, FileResult{
			Path:         f,
			Churn:        s.churn,
			Contributors: contributors,
			Score:        score,
			LastTouched:  s.latest,
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
		GeneratedAt:        time.Now().UTC(),
		WindowStart:        windowStart,
		WindowEnd:          windowEnd,
		FilesAnalyzed:      len(files),
		SingleContribFiles: singleContrib,
		Files:              files,
		Components:         components,
	}
	if len(files) > 0 {
		r.SingleContribPct = float64(singleContrib) / float64(len(files))
		r.MedianContributors = medianContributors(files)
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
		var sumScore, maxScore float64
		for _, f := range fs {
			sumScore += f.Score
			if f.Score > maxScore {
				maxScore = f.Score
			}
			if f.Contributors == 1 {
				c.SingleContribFiles++
			}
		}
		c.AvgScore = sumScore / float64(len(fs))
		c.MaxScore = maxScore
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MaxScore != out[j].MaxScore {
			return out[i].MaxScore > out[j].MaxScore
		}
		if out[i].AvgScore != out[j].AvgScore {
			return out[i].AvgScore > out[j].AvgScore
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

func medianContributors(files []FileResult) int {
	cs := make([]int, len(files))
	for i, f := range files {
		cs[i] = f.Contributors
	}
	sort.Ints(cs)
	return cs[len(cs)/2]
}
