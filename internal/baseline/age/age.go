// Package age computes the distribution of "last touched" timestamps across
// the files of a repository.
//
// For each file, the analyzer finds the most recent commit time and bins
// files into age buckets. Default boundaries (days): 30, 90, 365 — yielding
// the buckets <30d, 30-90d, 90-365d, >365d. Boundaries are configurable via
// Options.BucketBoundaries.
package age

import (
	"path"
	"sort"
	"strings"
	"time"

	"github.com/dreiboxco/epo-core/internal/baseline/source"
)

// DefaultBucketBoundariesDays follows the kpis.md spec: <30d, 30-90d,
// 90-365d, and >365d (the implicit "older than the last boundary" bucket).
var DefaultBucketBoundariesDays = []int{30, 90, 365}

// Options configures an Analyze run.
type Options struct {
	// Now is the reference time for age computation. If zero, time.Now() is
	// used at Analyze entry. Tests pin Now to keep results deterministic.
	Now time.Time
	// BucketBoundariesDays is the ascending list of upper bounds (exclusive)
	// for age buckets. If empty, DefaultBucketBoundariesDays is used.
	BucketBoundariesDays []int
	// ComponentDepth groups files by leading path segments.
	ComponentDepth int
	// Ignore lists path prefixes to skip.
	Ignore []string
}

func (o Options) now() time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}

func (o Options) boundaries() []int {
	if len(o.BucketBoundariesDays) == 0 {
		return DefaultBucketBoundariesDays
	}
	out := append([]int(nil), o.BucketBoundariesDays...)
	sort.Ints(out)
	return out
}

// FileResult describes a file's age.
type FileResult struct {
	Path        string
	AgeDays     int
	LastTouched time.Time
	BucketIndex int // 0 = freshest
}

// Bucket describes one age bucket.
type Bucket struct {
	// Label is a human-readable name like "<30d" or "30-90d".
	Label string
	// MinDays inclusive (0 for the first bucket).
	MinDays int
	// MaxDays exclusive. -1 means "no upper bound" (last bucket).
	MaxDays int
	// FileCount is the number of files in this bucket.
	FileCount int
}

// ComponentResult aggregates files under a path prefix.
type ComponentResult struct {
	Path          string
	FilesAnalyzed int
	MedianAgeDays int
	OldestAgeDays int
	StaleFiles    int // files whose age >= last bucket boundary
}

// Report is the analyzer output.
type Report struct {
	GeneratedAt   time.Time
	WindowStart   time.Time
	WindowEnd     time.Time
	Now           time.Time
	FilesAnalyzed int
	MedianAgeDays int
	OldestAgeDays int
	NewestAgeDays int
	Buckets       []Bucket
	Files         []FileResult // sorted by age desc (oldest first)
	Components    []ComponentResult
}

// Analyze walks commits and produces a code age report.
func Analyze(commits []source.Commit, opts Options) Report {
	now := opts.now()
	boundaries := opts.boundaries()

	type stat struct {
		latest time.Time
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
				s = &stat{}
				stats[f] = s
			}
			if c.When.After(s.latest) {
				s.latest = c.When
			}
		}
	}

	// Build buckets metadata.
	buckets := make([]Bucket, 0, len(boundaries)+1)
	prev := 0
	for _, b := range boundaries {
		buckets = append(buckets, Bucket{
			Label:   labelFor(prev, b),
			MinDays: prev,
			MaxDays: b,
		})
		prev = b
	}
	buckets = append(buckets, Bucket{
		Label:   ">" + days(prev),
		MinDays: prev,
		MaxDays: -1,
	})

	files := make([]FileResult, 0, len(stats))
	for f, s := range stats {
		ageDays := int(now.Sub(s.latest).Hours() / 24)
		if ageDays < 0 {
			ageDays = 0 // commit timestamp ahead of "now"; clamp.
		}
		idx := bucketIndex(ageDays, boundaries)
		buckets[idx].FileCount++
		files = append(files, FileResult{
			Path:        f,
			AgeDays:     ageDays,
			LastTouched: s.latest,
			BucketIndex: idx,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].AgeDays != files[j].AgeDays {
			return files[i].AgeDays > files[j].AgeDays
		}
		return files[i].Path < files[j].Path
	})

	r := Report{
		GeneratedAt:   time.Now().UTC(),
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
		Now:           now,
		FilesAnalyzed: len(files),
		Buckets:       buckets,
		Files:         files,
	}

	if len(files) > 0 {
		ages := make([]int, len(files))
		for i, f := range files {
			ages[i] = f.AgeDays
		}
		sort.Ints(ages)
		r.MedianAgeDays = ages[len(ages)/2]
		r.OldestAgeDays = ages[len(ages)-1]
		r.NewestAgeDays = ages[0]
	}

	r.Components = aggregateComponents(files, opts.ComponentDepth, lastBoundary(boundaries))
	return r
}

// bucketIndex returns the index of the bucket the age belongs to.
// boundaries are exclusive upper bounds; the last bucket has no upper bound.
func bucketIndex(ageDays int, boundaries []int) int {
	for i, b := range boundaries {
		if ageDays < b {
			return i
		}
	}
	return len(boundaries)
}

func aggregateComponents(files []FileResult, depth, staleBoundary int) []ComponentResult {
	if len(files) == 0 {
		return nil
	}
	groups := make(map[string][]FileResult)
	for _, f := range files {
		groups[componentKey(f.Path, depth)] = append(groups[componentKey(f.Path, depth)], f)
	}

	out := make([]ComponentResult, 0, len(groups))
	for k, fs := range groups {
		ages := make([]int, len(fs))
		oldest := 0
		stale := 0
		for i, f := range fs {
			ages[i] = f.AgeDays
			if f.AgeDays > oldest {
				oldest = f.AgeDays
			}
			if staleBoundary >= 0 && f.AgeDays >= staleBoundary {
				stale++
			}
		}
		sort.Ints(ages)
		out = append(out, ComponentResult{
			Path:          k,
			FilesAnalyzed: len(fs),
			MedianAgeDays: ages[len(ages)/2],
			OldestAgeDays: oldest,
			StaleFiles:    stale,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StaleFiles != out[j].StaleFiles {
			return out[i].StaleFiles > out[j].StaleFiles
		}
		if out[i].MedianAgeDays != out[j].MedianAgeDays {
			return out[i].MedianAgeDays > out[j].MedianAgeDays
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func lastBoundary(boundaries []int) int {
	if len(boundaries) == 0 {
		return -1
	}
	return boundaries[len(boundaries)-1]
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

func labelFor(min, max int) string {
	if min == 0 {
		return "<" + days(max)
	}
	return days(min) + "-" + days(max)
}

func days(n int) string {
	// Cheap stringification — keeps "30d", "365d" etc readable in tables.
	// For human reports we never go below 1 day so no special handling needed.
	return itoa(n) + "d"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	negative := false
	if i < 0 {
		negative = true
		i = -i
	}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
