// Package coupling computes intra-repository temporal coupling: pairs of
// files that consistently change in the same commit.
//
// For each pair (a, b) it computes:
//
//	support     = co-occurrence count
//	conf(a→b)   = support / commitsTouching(a)
//	conf(b→a)   = support / commitsTouching(b)
//	conf_avg    = average of the two
//
// A pair surfaces only when support and conf_avg both clear configurable
// thresholds. Catch-all files (CHANGELOG, lockfiles, schema.rb) are
// pre-filtered by skipping any file that appears in more than
// MaxCommitsPerFile commits, and giant rename/refactor commits are skipped
// by capping MaxFilesPerCommit.
package coupling

import (
	"path"
	"sort"
	"strings"
	"time"

	"github.com/dreiboxco/epo-core/internal/baseline/source"
)

// Options configures an Analyze run.
type Options struct {
	// MinSupport is the minimum number of co-occurrences a pair needs to
	// surface. 0 falls back to a default of 5.
	MinSupport int
	// MinConfidence is the minimum average bidirectional confidence (0..1).
	// 0 falls back to a default of 0.5. Set explicitly to 0.0 with a
	// negative sentinel if a future caller needs zero-threshold output;
	// for now zero-means-default is fine.
	MinConfidence float64
	// MaxFilesPerCommit caps how many files a commit may touch before being
	// excluded entirely (to skip mass-edit/refactor commits).
	MaxFilesPerCommit int
	// MaxCommitsPerFile caps how many commits a file may appear in before
	// being excluded as a catch-all (CHANGELOG, schema.rb, etc.).
	MaxCommitsPerFile int
	// ComponentDepth groups results into per-component pair cells.
	ComponentDepth int
	// Ignore lists path prefixes to skip.
	Ignore []string
}

const (
	defaultMinSupport        = 5
	defaultMinConfidence     = 0.5
	defaultMaxFilesPerCommit = 50
	defaultMaxCommitsPerFile = 200
)

func (o Options) minSupport() int {
	if o.MinSupport <= 0 {
		return defaultMinSupport
	}
	return o.MinSupport
}

func (o Options) minConfidence() float64 {
	if o.MinConfidence <= 0 {
		return defaultMinConfidence
	}
	return o.MinConfidence
}

func (o Options) maxFilesPerCommit() int {
	if o.MaxFilesPerCommit <= 0 {
		return defaultMaxFilesPerCommit
	}
	return o.MaxFilesPerCommit
}

func (o Options) maxCommitsPerFile() int {
	if o.MaxCommitsPerFile <= 0 {
		return defaultMaxCommitsPerFile
	}
	return o.MaxCommitsPerFile
}

// PairResult is a single coupled pair.
type PairResult struct {
	FileA          string
	FileB          string
	Support        int
	ConfidenceAtoB float64
	ConfidenceBtoA float64
	ConfidenceAvg  float64
}

// ComponentPair is the aggregate over all file pairs whose components are
// (CompA, CompB).
type ComponentPair struct {
	CompA        string
	CompB        string
	PairCount    int
	TotalSupport int
}

// Report bundles the analyzer output.
type Report struct {
	GeneratedAt             time.Time
	WindowStart             time.Time
	WindowEnd               time.Time
	CommitsAnalyzed         int
	CommitsExcluded         int
	FilesExcludedAsCatchall []string
	Pairs                   []PairResult // sorted by ConfidenceAvg desc, then Support desc
	ComponentPairs          []ComponentPair
}

// Analyze walks commits and emits temporally coupled pairs.
func Analyze(commits []source.Commit, opts Options) Report {
	minSupport := opts.minSupport()
	minConf := opts.minConfidence()
	maxFiles := opts.maxFilesPerCommit()
	maxCommitsPerFile := opts.maxCommitsPerFile()

	// Phase 1 — count commits per file (after ignore filter).
	commitsPerFile := make(map[string]int)
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
			commitsPerFile[f]++
		}
	}

	// Build catch-all set.
	excluded := make(map[string]struct{})
	for f, n := range commitsPerFile {
		if n > maxCommitsPerFile {
			excluded[f] = struct{}{}
		}
	}

	// Phase 2 — pair counting.
	type pairKey struct {
		a, b string
	}
	pairCount := make(map[pairKey]int)

	commitsExcluded := 0
	for _, c := range commits {
		// Build the dedup'd, ignore-and-catchall-filtered file slice for this commit.
		seen := make(map[string]struct{}, len(c.Files))
		filtered := make([]string, 0, len(c.Files))
		for _, f := range c.Files {
			if shouldIgnore(f, opts.Ignore) {
				continue
			}
			if _, dup := seen[f]; dup {
				continue
			}
			seen[f] = struct{}{}
			if _, isCatch := excluded[f]; isCatch {
				continue
			}
			filtered = append(filtered, f)
		}
		if len(filtered) > maxFiles {
			commitsExcluded++
			continue
		}
		if len(filtered) < 2 {
			continue
		}
		sort.Strings(filtered) // ensure pair ordering invariant a < b
		for i := 0; i < len(filtered); i++ {
			for j := i + 1; j < len(filtered); j++ {
				pairCount[pairKey{filtered[i], filtered[j]}]++
			}
		}
	}

	// Phase 3 — derive results.
	pairs := make([]PairResult, 0, len(pairCount))
	for k, n := range pairCount {
		if n < minSupport {
			continue
		}
		ca := float64(n) / float64(commitsPerFile[k.a])
		cb := float64(n) / float64(commitsPerFile[k.b])
		avg := (ca + cb) / 2
		if avg < minConf {
			continue
		}
		pairs = append(pairs, PairResult{
			FileA:          k.a,
			FileB:          k.b,
			Support:        n,
			ConfidenceAtoB: ca,
			ConfidenceBtoA: cb,
			ConfidenceAvg:  avg,
		})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].ConfidenceAvg != pairs[j].ConfidenceAvg {
			return pairs[i].ConfidenceAvg > pairs[j].ConfidenceAvg
		}
		if pairs[i].Support != pairs[j].Support {
			return pairs[i].Support > pairs[j].Support
		}
		if pairs[i].FileA != pairs[j].FileA {
			return pairs[i].FileA < pairs[j].FileA
		}
		return pairs[i].FileB < pairs[j].FileB
	})

	excludedList := make([]string, 0, len(excluded))
	for f := range excluded {
		excludedList = append(excludedList, f)
	}
	sort.Strings(excludedList)

	return Report{
		GeneratedAt:             time.Now().UTC(),
		WindowStart:             windowStart,
		WindowEnd:               windowEnd,
		CommitsAnalyzed:         len(commits),
		CommitsExcluded:         commitsExcluded,
		FilesExcludedAsCatchall: excludedList,
		Pairs:                   pairs,
		ComponentPairs:          aggregateComponentPairs(pairs, opts.ComponentDepth),
	}
}

func aggregateComponentPairs(pairs []PairResult, depth int) []ComponentPair {
	if len(pairs) == 0 {
		return nil
	}
	type key struct{ a, b string }
	agg := make(map[key]*ComponentPair)
	for _, p := range pairs {
		ca := componentKey(p.FileA, depth)
		cb := componentKey(p.FileB, depth)
		// Normalize order for symmetric grouping.
		if ca > cb {
			ca, cb = cb, ca
		}
		k := key{ca, cb}
		c, ok := agg[k]
		if !ok {
			c = &ComponentPair{CompA: ca, CompB: cb}
			agg[k] = c
		}
		c.PairCount++
		c.TotalSupport += p.Support
	}
	out := make([]ComponentPair, 0, len(agg))
	for _, c := range agg {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalSupport != out[j].TotalSupport {
			return out[i].TotalSupport > out[j].TotalSupport
		}
		if out[i].PairCount != out[j].PairCount {
			return out[i].PairCount > out[j].PairCount
		}
		if out[i].CompA != out[j].CompA {
			return out[i].CompA < out[j].CompA
		}
		return out[i].CompB < out[j].CompB
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
