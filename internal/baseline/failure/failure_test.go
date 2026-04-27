package failure

import (
	"math"
	"regexp"
	"testing"
	"time"

	"github.com/dreiboxco/epo-core/internal/baseline/deploy"
	"github.com/dreiboxco/epo-core/internal/baseline/source"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestAnalyze_OverallRate(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{When: ts("2026-04-13T10:00:00Z"), Message: "fix: bug 1", Files: []string{"a.go"}},
		{When: ts("2026-04-13T11:00:00Z"), Message: "feat: x", Files: []string{"a.go"}},
		{When: ts("2026-04-13T12:00:00Z"), Message: "feat: y", Files: []string{"a.go"}},
		{When: ts("2026-04-20T10:00:00Z"), Message: "fix: bug 2", Files: []string{"a.go"}},
	}
	r := Analyze(commits, Options{Now: now})

	if r.TotalCommits != 4 {
		t.Errorf("TotalCommits: got %d, want 4", r.TotalCommits)
	}
	if r.FixCommits != 2 {
		t.Errorf("FixCommits: got %d, want 2", r.FixCommits)
	}
	if math.Abs(r.OverallRate-0.5) > 1e-9 {
		t.Errorf("OverallRate: got %.4f, want 0.5", r.OverallRate)
	}
}

func TestAnalyze_PerBucketRate(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		// Week of 2026-04-13: 4 commits, 1 fix → rate 0.25
		{When: ts("2026-04-13T10:00:00Z"), Message: "feat: a", Files: []string{"a.go"}},
		{When: ts("2026-04-14T10:00:00Z"), Message: "feat: b", Files: []string{"a.go"}},
		{When: ts("2026-04-15T10:00:00Z"), Message: "feat: c", Files: []string{"a.go"}},
		{When: ts("2026-04-16T10:00:00Z"), Message: "fix: d", Files: []string{"a.go"}},

		// Week of 2026-04-20: 2 commits, 2 fixes → rate 1.0 (worst)
		{When: ts("2026-04-20T10:00:00Z"), Message: "fix: e", Files: []string{"a.go"}},
		{When: ts("2026-04-21T10:00:00Z"), Message: "fix: f", Files: []string{"a.go"}},
	}
	r := Analyze(commits, Options{Now: now})

	if got := len(r.Buckets); got != 3 {
		t.Fatalf("buckets count: got %d, want 3", got)
	}
	// Bucket 0: 04-13 → 4 total, 1 fix
	if r.Buckets[0].Total != 4 || r.Buckets[0].Fixes != 1 {
		t.Errorf("bucket 0: total=%d fixes=%d", r.Buckets[0].Total, r.Buckets[0].Fixes)
	}
	// Bucket 1: 04-20 → 2 total, 2 fixes
	if r.Buckets[1].Total != 2 || r.Buckets[1].Fixes != 2 {
		t.Errorf("bucket 1: total=%d fixes=%d", r.Buckets[1].Total, r.Buckets[1].Fixes)
	}
	// Bucket 2: 04-27 → 0 total
	if r.Buckets[2].Total != 0 {
		t.Errorf("bucket 2: total=%d, want 0", r.Buckets[2].Total)
	}

	// Worst bucket should be 04-20 (rate 1.0).
	if len(r.WorstBuckets) != 2 {
		t.Fatalf("WorstBuckets count: got %d, want 2 (zero-total bucket excluded)", len(r.WorstBuckets))
	}
	if !r.WorstBuckets[0].Start.Equal(ts("2026-04-20T00:00:00Z")) {
		t.Errorf("worst bucket: got %v, want 2026-04-20", r.WorstBuckets[0].Start)
	}
}

func TestAnalyze_CustomPattern(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	custom := regexp.MustCompile(`(?i)\[bug\]`)
	commits := []source.Commit{
		{When: ts("2026-04-13T10:00:00Z"), Message: "[BUG] crash", Files: []string{"a.go"}},
		{When: ts("2026-04-13T11:00:00Z"), Message: "feat: y", Files: []string{"a.go"}},
	}
	r := Analyze(commits, Options{Now: now, BugPattern: custom})
	if r.FixCommits != 1 {
		t.Errorf("custom pattern: got %d fixes, want 1", r.FixCommits)
	}
}

func TestAnalyze_RespectsIgnore(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{When: ts("2026-04-13T10:00:00Z"), Message: "fix: vendor only", Files: []string{"vendor/foo.go"}},
		{When: ts("2026-04-13T11:00:00Z"), Message: "feat: x", Files: []string{"a.go"}},
	}
	r := Analyze(commits, Options{Now: now, Ignore: []string{"vendor/"}})
	if r.TotalCommits != 1 {
		t.Errorf("ignore filter: TotalCommits=%d, want 1", r.TotalCommits)
	}
	if r.FixCommits != 0 {
		t.Errorf("ignored fix commit must not count: FixCommits=%d", r.FixCommits)
	}
}

func TestAnalyze_BucketRespectsOption(t *testing.T) {
	now := ts("2026-04-27T00:00:00Z")
	commits := []source.Commit{
		{When: ts("2026-04-26T10:00:00Z"), Message: "feat: x", Files: []string{"a.go"}},
		{When: ts("2026-04-27T10:00:00Z"), Message: "fix: y", Files: []string{"a.go"}},
	}
	r := Analyze(commits, Options{Now: now, Bucket: deploy.BucketDaily})
	if r.Bucket != deploy.BucketDaily {
		t.Errorf("bucket: got %v, want daily", r.Bucket)
	}
	if len(r.Buckets) != 2 {
		t.Errorf("daily buckets count: got %d, want 2", len(r.Buckets))
	}
}

func TestAnalyze_EmptyInput(t *testing.T) {
	r := Analyze(nil, Options{Now: ts("2026-04-27T00:00:00Z")})
	if r.TotalCommits != 0 || r.OverallRate != 0 || len(r.Buckets) != 0 {
		t.Fatalf("empty input must yield zeroed report")
	}
}
