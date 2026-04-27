// Package deploy computes deployment frequency from a stream of commits.
//
// For v0, "a deploy" is approximated by "a commit on HEAD". The loader
// already filters merges by default and applies --since, so what reaches
// Analyze is effectively the cadence of work hitting the default branch.
package deploy

import (
	"sort"
	"strings"
	"time"

	"github.com/dreiboxco/epo-core/internal/baseline/source"
)

// Bucket is the time-binning resolution.
type Bucket string

const (
	BucketDaily   Bucket = "daily"
	BucketWeekly  Bucket = "weekly"
	BucketMonthly Bucket = "monthly"
)

// Options configures an Analyze run.
type Options struct {
	// Bucket is the binning resolution. Empty falls back to weekly.
	Bucket Bucket
	// Ignore lists path prefixes to skip when deciding whether a commit
	// counts toward deploys (a commit that only touches ignored paths is
	// excluded). Empty list means "count every commit."
	Ignore []string
	// Now is the reference time used to define the trailing edge of the
	// final bucket. Tests pin it; production callers leave zero.
	Now time.Time
}

func (o Options) bucket() Bucket {
	if o.Bucket == "" {
		return BucketWeekly
	}
	return o.Bucket
}

func (o Options) now() time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}

// BucketCount holds the number of deploys in a single time bucket.
type BucketCount struct {
	// Start is the bucket's inclusive lower bound (UTC, normalized to bucket boundary).
	Start time.Time
	// Count is the number of commits in this bucket.
	Count int
}

// Report bundles the analyzer output.
type Report struct {
	GeneratedAt     time.Time
	WindowStart     time.Time
	WindowEnd       time.Time
	Bucket          Bucket
	TotalCommits    int
	BucketsCount    int
	MeanPerBucket   float64
	MedianPerBucket int
	LastBucketCount int
	Buckets         []BucketCount // sorted by Start asc (oldest first)
}

// Analyze counts commits per time bucket.
func Analyze(commits []source.Commit, opts Options) Report {
	bucket := opts.bucket()
	now := opts.now()

	type k struct {
		t time.Time
	}
	counts := make(map[time.Time]int)

	var windowStart, windowEnd time.Time
	considered := 0

	for _, c := range commits {
		if !commitTouchesNonIgnored(c, opts.Ignore) {
			continue
		}
		considered++
		if windowStart.IsZero() || c.When.Before(windowStart) {
			windowStart = c.When
		}
		if c.When.After(windowEnd) {
			windowEnd = c.When
		}
		key := bucketStart(c.When, bucket)
		counts[key]++
	}

	// Build dense bucket list from windowStart to now (inclusive).
	buckets := make([]BucketCount, 0, len(counts)+1)
	if !windowStart.IsZero() {
		from := bucketStart(windowStart, bucket)
		to := bucketStart(now, bucket)
		for t := from; !t.After(to); t = bucketAdvance(t, bucket) {
			buckets = append(buckets, BucketCount{Start: t, Count: counts[t]})
		}
	}

	r := Report{
		GeneratedAt:  time.Now().UTC(),
		WindowStart:  windowStart,
		WindowEnd:    windowEnd,
		Bucket:       bucket,
		TotalCommits: considered,
		BucketsCount: len(buckets),
		Buckets:      buckets,
	}
	if len(buckets) > 0 {
		var total int
		for _, b := range buckets {
			total += b.Count
		}
		r.MeanPerBucket = float64(total) / float64(len(buckets))

		nums := make([]int, len(buckets))
		for i, b := range buckets {
			nums[i] = b.Count
		}
		sort.Ints(nums)
		r.MedianPerBucket = nums[len(nums)/2]

		r.LastBucketCount = buckets[len(buckets)-1].Count
	}
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

// bucketStart normalizes a timestamp to the start of its bucket in UTC.
//
// Daily: 00:00 UTC of the same date.
// Weekly: 00:00 UTC of the Monday of the same ISO week.
// Monthly: 00:00 UTC on the first of the same month.
func bucketStart(t time.Time, b Bucket) time.Time {
	t = t.UTC()
	switch b {
	case BucketDaily:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	case BucketMonthly:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	default: // weekly
		// time.Weekday(): Sunday=0..Saturday=6. We want Monday-based weeks
		// (ISO 8601). So shift: days_back = (weekday + 6) % 7.
		offset := (int(t.Weekday()) + 6) % 7
		monday := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -offset)
		return monday
	}
}

func bucketAdvance(t time.Time, b Bucket) time.Time {
	switch b {
	case BucketDaily:
		return t.AddDate(0, 0, 1)
	case BucketMonthly:
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 0, 7)
	}
}
