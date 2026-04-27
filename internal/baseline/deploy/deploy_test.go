package deploy

import (
	"testing"
	"time"

	"github.com/dreiboxco/epo-core/internal/baseline/source"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestBucketStart_Weekly_Monday(t *testing.T) {
	// 2026-04-27 is a Monday (UTC). Bucket start should be 2026-04-27 00:00 UTC.
	got := bucketStart(ts("2026-04-27T15:30:00Z"), BucketWeekly)
	want := ts("2026-04-27T00:00:00Z")
	if !got.Equal(want) {
		t.Errorf("monday bucket: got %v, want %v", got, want)
	}

	// 2026-04-28 (Tuesday) → bucket start still Monday 2026-04-27.
	got2 := bucketStart(ts("2026-04-28T05:00:00Z"), BucketWeekly)
	if !got2.Equal(want) {
		t.Errorf("tuesday rounds to monday: got %v, want %v", got2, want)
	}

	// 2026-05-03 (Sunday) → still in the week starting 2026-04-27.
	got3 := bucketStart(ts("2026-05-03T23:59:00Z"), BucketWeekly)
	if !got3.Equal(want) {
		t.Errorf("sunday rounds to monday: got %v, want %v", got3, want)
	}
}

func TestBucketStart_Daily(t *testing.T) {
	got := bucketStart(ts("2026-04-27T15:30:00Z"), BucketDaily)
	want := ts("2026-04-27T00:00:00Z")
	if !got.Equal(want) {
		t.Errorf("daily: got %v, want %v", got, want)
	}
}

func TestBucketStart_Monthly(t *testing.T) {
	got := bucketStart(ts("2026-04-27T15:30:00Z"), BucketMonthly)
	want := ts("2026-04-01T00:00:00Z")
	if !got.Equal(want) {
		t.Errorf("monthly: got %v, want %v", got, want)
	}
}

func TestAnalyze_BucketsCommitsByWeek(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z") // Monday
	commits := []source.Commit{
		// Week of 2026-04-13 (3 commits)
		{When: ts("2026-04-13T10:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-15T10:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-19T10:00:00Z"), Files: []string{"a.go"}},
		// Week of 2026-04-20 (1 commit)
		{When: ts("2026-04-22T10:00:00Z"), Files: []string{"a.go"}},
		// Week of 2026-04-27 (2 commits, current week)
		{When: ts("2026-04-27T10:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-27T15:00:00Z"), Files: []string{"a.go"}},
	}
	r := Analyze(commits, Options{Now: now})

	if r.TotalCommits != 6 {
		t.Errorf("TotalCommits: got %d, want 6", r.TotalCommits)
	}
	if r.BucketsCount != 3 {
		t.Errorf("BucketsCount: got %d, want 3", r.BucketsCount)
	}
	wantCounts := []int{3, 1, 2}
	for i, want := range wantCounts {
		if r.Buckets[i].Count != want {
			t.Errorf("bucket %d: got %d, want %d", i, r.Buckets[i].Count, want)
		}
	}
	if r.LastBucketCount != 2 {
		t.Errorf("LastBucketCount: got %d, want 2", r.LastBucketCount)
	}
}

func TestAnalyze_GapsAreZeroBuckets(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{When: ts("2026-04-06T10:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-27T10:00:00Z"), Files: []string{"a.go"}},
	}
	r := Analyze(commits, Options{Now: now})
	// Buckets: 2026-04-06 (Mon), 2026-04-13, 2026-04-20, 2026-04-27 → 4 buckets,
	// counts 1, 0, 0, 1.
	if r.BucketsCount != 4 {
		t.Fatalf("BucketsCount: got %d, want 4", r.BucketsCount)
	}
	expect := []int{1, 0, 0, 1}
	for i, want := range expect {
		if r.Buckets[i].Count != want {
			t.Errorf("bucket %d: got %d, want %d", i, r.Buckets[i].Count, want)
		}
	}
}

func TestAnalyze_MeanAndMedian(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{When: ts("2026-04-06T10:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-13T10:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-13T11:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-13T12:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-20T10:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-20T11:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-27T10:00:00Z"), Files: []string{"a.go"}},
	}
	r := Analyze(commits, Options{Now: now})
	// Buckets: 1, 3, 2, 1 (4 buckets). Mean = 7/4 = 1.75. Median (sorted: 1,1,2,3) index 2 = 2.
	if r.MeanPerBucket != 1.75 {
		t.Errorf("mean: got %.4f, want 1.75", r.MeanPerBucket)
	}
	if r.MedianPerBucket != 2 {
		t.Errorf("median: got %d, want 2", r.MedianPerBucket)
	}
}

func TestAnalyze_IgnoreFiltersCommitsThatTouchOnlyIgnored(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{When: ts("2026-04-13T10:00:00Z"), Files: []string{"vendor/foo.go"}},         // skipped
		{When: ts("2026-04-13T11:00:00Z"), Files: []string{"vendor/foo.go", "a.go"}}, // kept (touches a.go)
		{When: ts("2026-04-13T12:00:00Z"), Files: []string{"a.go"}},                  // kept
	}
	r := Analyze(commits, Options{Now: now, Ignore: []string{"vendor/"}})
	if r.TotalCommits != 2 {
		t.Errorf("TotalCommits: got %d, want 2", r.TotalCommits)
	}
}

func TestAnalyze_EmptyInput(t *testing.T) {
	r := Analyze(nil, Options{Now: ts("2026-04-27T00:00:00Z")})
	if r.TotalCommits != 0 || r.BucketsCount != 0 {
		t.Fatalf("empty input must yield empty report")
	}
}

func TestAnalyze_DailyBucket(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{When: ts("2026-04-25T10:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-26T10:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-26T11:00:00Z"), Files: []string{"a.go"}},
		{When: ts("2026-04-27T05:00:00Z"), Files: []string{"a.go"}},
	}
	r := Analyze(commits, Options{Now: now, Bucket: BucketDaily})
	// Buckets: 04-25, 04-26, 04-27 → counts 1, 2, 1
	if r.BucketsCount != 3 {
		t.Fatalf("BucketsCount: got %d, want 3", r.BucketsCount)
	}
	expect := []int{1, 2, 1}
	for i, want := range expect {
		if r.Buckets[i].Count != want {
			t.Errorf("bucket %d: got %d, want %d", i, r.Buckets[i].Count, want)
		}
	}
}
