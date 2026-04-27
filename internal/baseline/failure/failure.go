// Package failure computes a Git-only proxy of the DORA Change Failure Rate.
//
// For each commit, the analyzer checks whether the message matches a bug-fix
// pattern (the same regex used by hotspots, overrideable). The headline rate
// is fix-commits / total-commits over the window. The same buckets used by
// the deploy package are used here so the two metrics line up in time.
//
// This is a heuristic, not the rigorous DORA definition (failed deploys /
// total deploys). Teams should treat it as a directional signal.
package failure

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dreiboxco/epo-core/internal/baseline/deploy"
	"github.com/dreiboxco/epo-core/internal/baseline/hotspots"
	"github.com/dreiboxco/epo-core/internal/baseline/source"
)

// Options configures an Analyze run.
type Options struct {
	// Bucket is passed through to the deploy package's bucket logic for
	// consistent time alignment.
	Bucket deploy.Bucket
	// BugPattern overrides the default regex used to classify a fix commit.
	// Nil falls back to hotspots.DefaultBugPattern.
	BugPattern *regexp.Regexp
	// Ignore lists path prefixes to skip. A commit that only touches
	// ignored paths is excluded from both numerator and denominator.
	Ignore []string
	// Now is the reference time for the trailing edge of the final bucket.
	// Tests pin it; production callers leave zero.
	Now time.Time
}

func (o Options) bugPattern() *regexp.Regexp {
	if o.BugPattern != nil {
		return o.BugPattern
	}
	return hotspots.DefaultBugPattern
}

// BucketRate is the failure-rate breakdown for one time bucket.
type BucketRate struct {
	Start    time.Time
	Total    int
	Fixes    int
	FailRate float64
}

// Report bundles the analyzer output.
type Report struct {
	GeneratedAt  time.Time
	WindowStart  time.Time
	WindowEnd    time.Time
	Bucket       deploy.Bucket
	TotalCommits int
	FixCommits   int
	OverallRate  float64
	Buckets      []BucketRate // sorted by Start asc
	WorstBuckets []BucketRate // sorted by FailRate desc, ties by Total desc
}

// Analyze classifies each commit and computes per-bucket failure rates.
func Analyze(commits []source.Commit, opts Options) Report {
	pattern := opts.bugPattern()
	bucket := opts.Bucket
	if bucket == "" {
		bucket = deploy.BucketWeekly
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	type stat struct {
		total int
		fixes int
	}
	stats := make(map[time.Time]*stat)

	var (
		windowStart, windowEnd time.Time
		total, fixes           int
	)

	for _, c := range commits {
		if !commitTouchesNonIgnored(c, opts.Ignore) {
			continue
		}
		total++
		if windowStart.IsZero() || c.When.Before(windowStart) {
			windowStart = c.When
		}
		if c.When.After(windowEnd) {
			windowEnd = c.When
		}
		key := bucketStartLocal(c.When, bucket)
		s, ok := stats[key]
		if !ok {
			s = &stat{}
			stats[key] = s
		}
		s.total++
		if pattern.MatchString(c.Message) {
			s.fixes++
			fixes++
		}
	}

	// Build dense bucket list to keep zero-count weeks visible.
	out := make([]BucketRate, 0, len(stats)+1)
	if !windowStart.IsZero() {
		from := bucketStartLocal(windowStart, bucket)
		to := bucketStartLocal(now, bucket)
		for t := from; !t.After(to); t = bucketAdvanceLocal(t, bucket) {
			s := stats[t]
			br := BucketRate{Start: t}
			if s != nil {
				br.Total = s.total
				br.Fixes = s.fixes
				if s.total > 0 {
					br.FailRate = float64(s.fixes) / float64(s.total)
				}
			}
			out = append(out, br)
		}
	}

	r := Report{
		GeneratedAt:  time.Now().UTC(),
		WindowStart:  windowStart,
		WindowEnd:    windowEnd,
		Bucket:       bucket,
		TotalCommits: total,
		FixCommits:   fixes,
		Buckets:      out,
	}
	if total > 0 {
		r.OverallRate = float64(fixes) / float64(total)
	}

	// Build worst buckets (only those with at least one commit).
	worst := make([]BucketRate, 0, len(out))
	for _, b := range out {
		if b.Total > 0 {
			worst = append(worst, b)
		}
	}
	sort.Slice(worst, func(i, j int) bool {
		if worst[i].FailRate != worst[j].FailRate {
			return worst[i].FailRate > worst[j].FailRate
		}
		if worst[i].Total != worst[j].Total {
			return worst[i].Total > worst[j].Total
		}
		return worst[i].Start.Before(worst[j].Start)
	})
	r.WorstBuckets = worst

	return r
}

func commitTouchesNonIgnored(c source.Commit, ignore []string) bool {
	if len(ignore) == 0 {
		return true
	}
	for _, f := range c.Files {
		if !shouldIgnore(f, ignore) {
			return true
		}
	}
	return false
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

// bucketStartLocal duplicates deploy.bucketStart's logic to avoid an
// internal-only export. Cheap and avoids a circular dependency risk.
func bucketStartLocal(t time.Time, b deploy.Bucket) time.Time {
	t = t.UTC()
	switch b {
	case deploy.BucketDaily:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	case deploy.BucketMonthly:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		offset := (int(t.Weekday()) + 6) % 7
		monday := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -offset)
		return monday
	}
}

func bucketAdvanceLocal(t time.Time, b deploy.Bucket) time.Time {
	switch b {
	case deploy.BucketDaily:
		return t.AddDate(0, 0, 1)
	case deploy.BucketMonthly:
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 0, 7)
	}
}
