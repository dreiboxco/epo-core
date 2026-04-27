// Package busfactor computes the bus factor of files and components in a
// codebase from a stream of commits.
//
// A file's bus factor is the smallest number of unique authors whose combined
// share of the commits touching that file meets or exceeds a threshold
// (default 0.5). Bus factor 1 means a single author wrote more than half of
// all changes — losing that author orphans the file.
//
// The package is intentionally free of any git library dependency: callers
// turn whatever source they have into a []Commit and pass it in. This keeps
// the analyzer trivially testable and reusable for non-git inputs later.
package busfactor

import (
	"path"
	"sort"
	"strings"
	"time"
)

// Commit is the minimal slice of a VCS commit the analyzer needs.
type Commit struct {
	// SHA is the commit identifier; only used for diagnostics.
	SHA string
	// Author identifies who made the commit. Email is the typical key, but
	// callers may pass a normalized identity (e.g. after .mailmap merging).
	Author string
	// When is the commit timestamp (used for windowing).
	When time.Time
	// Files lists the files touched by the commit.
	Files []string
}

// Options configures an Analyze run.
type Options struct {
	// Threshold is the cumulative share that defines coverage; defaults to 0.5.
	Threshold float64
	// ComponentDepth is the number of leading path segments that define a
	// component. A depth of 2 groups "internal/cli/foo.go" and
	// "internal/cli/bar.go" under "internal/cli". Zero treats every file as
	// its own component (unusual; mostly for tests).
	ComponentDepth int
	// Ignore is a set of glob-style prefixes that, when matched, cause the
	// file to be skipped. The match is a simple HasPrefix on the path; this
	// is enough for "vendor/", "node_modules/", "third_party/" — full glob
	// support is deferred until we actually need it.
	Ignore []string
}

func (o Options) threshold() float64 {
	if o.Threshold <= 0 {
		return 0.5
	}
	return o.Threshold
}

// FileResult describes the bus factor of a single file.
type FileResult struct {
	Path        string
	BusFactor   int
	TopAuthor   string
	TopShare    float64
	Commits     int
	LastTouched time.Time
}

// ComponentResult is an aggregate over all files in a component path prefix.
type ComponentResult struct {
	Path            string
	FilesAnalyzed   int
	MinBusFactor    int
	MedianBusFactor int
	BusFactorOneCnt int
	BusFactorOnePct float64
}

// Report is the full output of Analyze.
type Report struct {
	GeneratedAt       time.Time
	WindowStart       time.Time
	WindowEnd         time.Time
	Threshold         float64
	FilesAnalyzed     int
	BusFactorOneFiles int
	BusFactorOnePct   float64
	MedianFileBusFact int
	Files             []FileResult
	Components        []ComponentResult
	Distribution      map[int]int // bus factor → count of files
}

// Analyze walks the commits and computes per-file and per-component bus
// factor under opts.
//
// Commits with no files (e.g. signed empty commits) are ignored. Files are
// counted once per commit even if listed multiple times.
func Analyze(commits []Commit, opts Options) Report {
	threshold := opts.threshold()

	type fileStat struct {
		authorCommits map[string]int
		total         int
		latest        time.Time
	}

	stats := make(map[string]*fileStat)
	var (
		windowStart, windowEnd time.Time
	)

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
				s = &fileStat{authorCommits: make(map[string]int)}
				stats[f] = s
			}
			s.authorCommits[c.Author]++
			s.total++
			if c.When.After(s.latest) {
				s.latest = c.When
			}
		}
	}

	files := make([]FileResult, 0, len(stats))
	dist := make(map[int]int)
	bfOneFiles := 0

	for f, s := range stats {
		bf, top, share := computeBusFactor(s.authorCommits, s.total, threshold)
		files = append(files, FileResult{
			Path:        f,
			BusFactor:   bf,
			TopAuthor:   top,
			TopShare:    share,
			Commits:     s.total,
			LastTouched: s.latest,
		})
		dist[bf]++
		if bf == 1 {
			bfOneFiles++
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].BusFactor != files[j].BusFactor {
			return files[i].BusFactor < files[j].BusFactor
		}
		if files[i].Commits != files[j].Commits {
			return files[i].Commits > files[j].Commits
		}
		return files[i].Path < files[j].Path
	})

	components := aggregateComponents(files, opts.ComponentDepth)

	report := Report{
		GeneratedAt:       time.Now().UTC(),
		WindowStart:       windowStart,
		WindowEnd:         windowEnd,
		Threshold:         threshold,
		FilesAnalyzed:     len(files),
		BusFactorOneFiles: bfOneFiles,
		Files:             files,
		Components:        components,
		Distribution:      dist,
	}
	if len(files) > 0 {
		report.BusFactorOnePct = float64(bfOneFiles) / float64(len(files))
		report.MedianFileBusFact = medianBusFactor(files)
	}
	return report
}

// computeBusFactor returns (busFactor, topAuthor, topShare).
// Authors are sorted by descending commit count; ties broken alphabetically
// for determinism. The bus factor is the smallest prefix of that ranking
// whose cumulative share >= threshold.
func computeBusFactor(authorCommits map[string]int, total int, threshold float64) (int, string, float64) {
	if total == 0 || len(authorCommits) == 0 {
		return 0, "", 0
	}

	type ac struct {
		author  string
		commits int
	}
	rank := make([]ac, 0, len(authorCommits))
	for a, n := range authorCommits {
		rank = append(rank, ac{author: a, commits: n})
	}
	sort.Slice(rank, func(i, j int) bool {
		if rank[i].commits != rank[j].commits {
			return rank[i].commits > rank[j].commits
		}
		return rank[i].author < rank[j].author
	})

	cumulative := 0
	for i, r := range rank {
		cumulative += r.commits
		if float64(cumulative)/float64(total) >= threshold {
			return i + 1, rank[0].author, float64(rank[0].commits) / float64(total)
		}
	}
	// Should not happen: threshold <= 1 and cumulative reaches total.
	return len(rank), rank[0].author, float64(rank[0].commits) / float64(total)
}

func aggregateComponents(files []FileResult, depth int) []ComponentResult {
	if len(files) == 0 {
		return nil
	}
	groups := make(map[string][]FileResult)
	for _, f := range files {
		key := componentKey(f.Path, depth)
		groups[key] = append(groups[key], f)
	}

	out := make([]ComponentResult, 0, len(groups))
	for k, fs := range groups {
		minBF := fs[0].BusFactor
		bfOne := 0
		bfs := make([]int, 0, len(fs))
		for _, f := range fs {
			if f.BusFactor < minBF {
				minBF = f.BusFactor
			}
			if f.BusFactor == 1 {
				bfOne++
			}
			bfs = append(bfs, f.BusFactor)
		}
		sort.Ints(bfs)
		median := bfs[len(bfs)/2]
		out = append(out, ComponentResult{
			Path:            k,
			FilesAnalyzed:   len(fs),
			MinBusFactor:    minBF,
			MedianBusFactor: median,
			BusFactorOneCnt: bfOne,
			BusFactorOnePct: float64(bfOne) / float64(len(fs)),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MinBusFactor != out[j].MinBusFactor {
			return out[i].MinBusFactor < out[j].MinBusFactor
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
		// File lives shallower than the requested depth; group it under the
		// directory it sits in (or the root marker for files at the root).
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

func medianBusFactor(files []FileResult) int {
	bfs := make([]int, len(files))
	for i, f := range files {
		bfs[i] = f.BusFactor
	}
	sort.Ints(bfs)
	return bfs[len(bfs)/2]
}
