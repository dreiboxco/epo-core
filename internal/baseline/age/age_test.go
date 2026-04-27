package age

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

func TestBucketIndex_BoundaryIsExclusive(t *testing.T) {
	boundaries := []int{30, 90, 365}
	cases := []struct {
		age  int
		want int
	}{
		{0, 0},   // < 30
		{29, 0},  // < 30
		{30, 1},  // exactly 30 → next bucket
		{31, 1},  // 30-90
		{89, 1},  // < 90
		{90, 2},  // exactly 90 → next bucket
		{200, 2}, // 90-365
		{365, 3}, // exactly 365 → last bucket
		{2000, 3},
	}
	for _, c := range cases {
		got := bucketIndex(c.age, boundaries)
		if got != c.want {
			t.Errorf("bucketIndex(%d): got %d, want %d", c.age, got, c.want)
		}
	}
}

func TestAnalyze_TakesLatestCommitPerFile(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		// a.go: oldest commit much older than newest → age = days since 2026-04-20
		{Author: "x", When: ts("2024-01-01T00:00:00Z"), Files: []string{"a.go"}, Message: "old"},
		{Author: "x", When: ts("2026-04-20T00:00:00Z"), Files: []string{"a.go"}, Message: "fresh"},
	}
	r := Analyze(commits, Options{Now: now})

	if r.FilesAnalyzed != 1 {
		t.Fatalf("FilesAnalyzed: got %d, want 1", r.FilesAnalyzed)
	}
	got := r.Files[0].AgeDays
	if got != 7 {
		t.Errorf("a.go age: got %d, want 7", got)
	}
}

func TestAnalyze_BucketCountsAreCorrect(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		// 2 fresh files (<30d)
		{Author: "x", When: now.AddDate(0, 0, -1), Files: []string{"fresh-a.go"}, Message: "x"},
		{Author: "x", When: now.AddDate(0, 0, -29), Files: []string{"fresh-b.go"}, Message: "x"},
		// 1 file in 30-90d
		{Author: "x", When: now.AddDate(0, 0, -45), Files: []string{"midd.go"}, Message: "x"},
		// 1 file in 90-365d
		{Author: "x", When: now.AddDate(0, 0, -180), Files: []string{"old.go"}, Message: "x"},
		// 2 files >365d (stale)
		{Author: "x", When: now.AddDate(-2, 0, 0), Files: []string{"stale-a.go"}, Message: "x"},
		{Author: "x", When: now.AddDate(-3, 0, 0), Files: []string{"stale-b.go"}, Message: "x"},
	}
	r := Analyze(commits, Options{Now: now})

	wantCounts := []int{2, 1, 1, 2}
	if len(r.Buckets) != 4 {
		t.Fatalf("bucket count: got %d, want 4", len(r.Buckets))
	}
	for i, want := range wantCounts {
		if r.Buckets[i].FileCount != want {
			t.Errorf("bucket %d (%s): got %d, want %d", i, r.Buckets[i].Label, r.Buckets[i].FileCount, want)
		}
	}
}

func TestAnalyze_FilesSortedOldestFirst(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{Author: "x", When: now.AddDate(0, 0, -1), Files: []string{"a.go"}, Message: "x"},
		{Author: "x", When: now.AddDate(-2, 0, 0), Files: []string{"b.go"}, Message: "x"},
		{Author: "x", When: now.AddDate(0, 0, -200), Files: []string{"c.go"}, Message: "x"},
	}
	r := Analyze(commits, Options{Now: now})
	if r.Files[0].Path != "b.go" {
		t.Errorf("first (oldest): got %q, want b.go", r.Files[0].Path)
	}
	if r.Files[2].Path != "a.go" {
		t.Errorf("last (newest): got %q, want a.go", r.Files[2].Path)
	}
}

func TestAnalyze_RespectsIgnore(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{Author: "x", When: now.AddDate(0, 0, -1), Files: []string{"vendor/foo.go", "internal/x.go"}, Message: "x"},
	}
	r := Analyze(commits, Options{Now: now, Ignore: []string{"vendor/"}})
	if r.FilesAnalyzed != 1 {
		t.Fatalf("FilesAnalyzed: got %d, want 1", r.FilesAnalyzed)
	}
}

func TestAnalyze_CustomBoundaries(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{Author: "x", When: now.AddDate(0, 0, -3), Files: []string{"a.go"}, Message: "x"},
		{Author: "x", When: now.AddDate(0, 0, -10), Files: []string{"b.go"}, Message: "x"},
	}
	r := Analyze(commits, Options{
		Now:                  now,
		BucketBoundariesDays: []int{7},
	})
	if len(r.Buckets) != 2 {
		t.Fatalf("bucket count: got %d, want 2", len(r.Buckets))
	}
	if r.Buckets[0].FileCount != 1 || r.Buckets[1].FileCount != 1 {
		t.Errorf("custom boundary bucket counts: %d / %d", r.Buckets[0].FileCount, r.Buckets[1].FileCount)
	}
}

func TestAnalyze_SummaryStatistics(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{Author: "x", When: now.AddDate(0, 0, -1), Files: []string{"a.go"}, Message: "x"},
		{Author: "x", When: now.AddDate(0, 0, -10), Files: []string{"b.go"}, Message: "x"},
		{Author: "x", When: now.AddDate(0, 0, -100), Files: []string{"c.go"}, Message: "x"},
	}
	r := Analyze(commits, Options{Now: now})
	// sorted ages: 1, 10, 100. Median (index 1) = 10. Oldest = 100. Newest = 1.
	if r.MedianAgeDays != 10 {
		t.Errorf("median: got %d, want 10", r.MedianAgeDays)
	}
	if r.OldestAgeDays != 100 {
		t.Errorf("oldest: got %d, want 100", r.OldestAgeDays)
	}
	if r.NewestAgeDays != 1 {
		t.Errorf("newest: got %d, want 1", r.NewestAgeDays)
	}
}

func TestAnalyze_EmptyInput(t *testing.T) {
	r := Analyze(nil, Options{Now: ts("2026-04-27T00:00:00Z")})
	if r.FilesAnalyzed != 0 || len(r.Files) != 0 {
		t.Fatalf("empty input must yield empty file list")
	}
	// Buckets should still exist (empty) so renderer always has something to show.
	if len(r.Buckets) != 4 {
		t.Errorf("default buckets even on empty input: got %d", len(r.Buckets))
	}
}

func TestComponentAggregation_StaleCount(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		// app/old has 2 stale, 0 fresh
		{Author: "x", When: now.AddDate(-2, 0, 0), Files: []string{"app/old/a.go"}, Message: "x"},
		{Author: "x", When: now.AddDate(-3, 0, 0), Files: []string{"app/old/b.go"}, Message: "x"},
		// app/fresh has only fresh
		{Author: "x", When: now.AddDate(0, 0, -2), Files: []string{"app/fresh/x.go"}, Message: "x"},
	}
	r := Analyze(commits, Options{Now: now, ComponentDepth: 2})

	for _, c := range r.Components {
		switch c.Path {
		case "app/old":
			if c.StaleFiles != 2 {
				t.Errorf("app/old stale count: got %d, want 2", c.StaleFiles)
			}
		case "app/fresh":
			if c.StaleFiles != 0 {
				t.Errorf("app/fresh stale count: got %d, want 0", c.StaleFiles)
			}
		}
	}
	// First component should be app/old (more stale).
	if r.Components[0].Path != "app/old" {
		t.Errorf("first component: got %q, want app/old", r.Components[0].Path)
	}
}
