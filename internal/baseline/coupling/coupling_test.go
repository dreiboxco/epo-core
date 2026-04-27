package coupling

import (
	"math"
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

func mkCommit(when string, files ...string) source.Commit {
	return source.Commit{
		Author:  "x",
		When:    ts(when),
		Files:   files,
		Message: "x",
	}
}

func TestAnalyze_StrongPairSurfaces(t *testing.T) {
	// A and B always change together, 6 times. C is independent, 6 times.
	commits := []source.Commit{
		mkCommit("2025-01-01T00:00:00Z", "a.go", "b.go"),
		mkCommit("2025-01-02T00:00:00Z", "a.go", "b.go"),
		mkCommit("2025-01-03T00:00:00Z", "a.go", "b.go"),
		mkCommit("2025-01-04T00:00:00Z", "a.go", "b.go"),
		mkCommit("2025-01-05T00:00:00Z", "a.go", "b.go"),
		mkCommit("2025-01-06T00:00:00Z", "a.go", "b.go"),
		mkCommit("2025-01-07T00:00:00Z", "c.go"),
		mkCommit("2025-01-08T00:00:00Z", "c.go"),
		mkCommit("2025-01-09T00:00:00Z", "c.go"),
		mkCommit("2025-01-10T00:00:00Z", "c.go"),
		mkCommit("2025-01-11T00:00:00Z", "c.go"),
		mkCommit("2025-01-12T00:00:00Z", "c.go"),
	}

	r := Analyze(commits, Options{})
	if len(r.Pairs) != 1 {
		t.Fatalf("pair count: got %d, want 1; pairs: %+v", len(r.Pairs), r.Pairs)
	}
	p := r.Pairs[0]
	if p.FileA != "a.go" || p.FileB != "b.go" {
		t.Errorf("pair files: got (%q,%q), want (a.go,b.go)", p.FileA, p.FileB)
	}
	if p.Support != 6 {
		t.Errorf("support: got %d, want 6", p.Support)
	}
	if math.Abs(p.ConfidenceAvg-1.0) > 1e-9 {
		t.Errorf("confidence avg: got %.4f, want 1.0", p.ConfidenceAvg)
	}
}

func TestAnalyze_BelowSupportSkipped(t *testing.T) {
	// A and B coupled only 4 times — below default min support of 5.
	commits := []source.Commit{
		mkCommit("2025-01-01T00:00:00Z", "a.go", "b.go"),
		mkCommit("2025-01-02T00:00:00Z", "a.go", "b.go"),
		mkCommit("2025-01-03T00:00:00Z", "a.go", "b.go"),
		mkCommit("2025-01-04T00:00:00Z", "a.go", "b.go"),
	}
	r := Analyze(commits, Options{})
	if len(r.Pairs) != 0 {
		t.Errorf("expected no pairs (support 4 < default 5), got %+v", r.Pairs)
	}
}

func TestAnalyze_BelowConfidenceSkipped(t *testing.T) {
	// A appears 10 times, B appears 10 times, but they only co-occur 5
	// times. confidence each direction = 0.5 — exactly default min, so it
	// should surface (>= 0.5). Tighten threshold and verify it drops out.
	commits := []source.Commit{}
	for i := 0; i < 5; i++ {
		commits = append(commits, mkCommit("2025-01-01T00:00:00Z", "a.go", "b.go"))
	}
	for i := 0; i < 5; i++ {
		commits = append(commits, mkCommit("2025-01-02T00:00:00Z", "a.go"))
	}
	for i := 0; i < 5; i++ {
		commits = append(commits, mkCommit("2025-01-03T00:00:00Z", "b.go"))
	}

	r := Analyze(commits, Options{})
	if len(r.Pairs) != 1 {
		t.Errorf("expected 1 pair at confidence 0.5, got %d", len(r.Pairs))
	}

	r2 := Analyze(commits, Options{MinConfidence: 0.6})
	if len(r2.Pairs) != 0 {
		t.Errorf("expected 0 pairs at confidence 0.6, got %d", len(r2.Pairs))
	}
}

func TestAnalyze_PairOrderingIsLexicographic(t *testing.T) {
	commits := []source.Commit{}
	for i := 0; i < 6; i++ {
		commits = append(commits, mkCommit("2025-01-01T00:00:00Z", "z.go", "a.go"))
	}
	r := Analyze(commits, Options{})
	if len(r.Pairs) != 1 {
		t.Fatalf("pair count: got %d, want 1", len(r.Pairs))
	}
	if r.Pairs[0].FileA != "a.go" || r.Pairs[0].FileB != "z.go" {
		t.Errorf("pair ordering: got (%q,%q), want (a.go,z.go)", r.Pairs[0].FileA, r.Pairs[0].FileB)
	}
}

func TestAnalyze_CatchAllFilesExcluded(t *testing.T) {
	// a.go ↔ b.go coupled in 10 commits, each touched 10 times.
	// CHANGELOG.md appears in those 10 commits *plus* 6 standalone commits
	// → 16 touches total. With MaxCommitsPerFile=12, only CHANGELOG should
	// be flagged as catch-all and pruned from pair generation.
	commits := []source.Commit{}
	for i := 0; i < 10; i++ {
		commits = append(commits, mkCommit("2025-01-01T00:00:00Z", "a.go", "b.go", "CHANGELOG.md"))
	}
	for i := 0; i < 6; i++ {
		commits = append(commits, mkCommit("2025-01-02T00:00:00Z", "CHANGELOG.md"))
	}

	r := Analyze(commits, Options{MaxCommitsPerFile: 12})

	if len(r.FilesExcludedAsCatchall) != 1 || r.FilesExcludedAsCatchall[0] != "CHANGELOG.md" {
		t.Errorf("expected CHANGELOG excluded, got %v", r.FilesExcludedAsCatchall)
	}
	if len(r.Pairs) != 1 {
		t.Fatalf("expected 1 pair (a.go↔b.go), got %d", len(r.Pairs))
	}
	for _, p := range r.Pairs {
		if p.FileA == "CHANGELOG.md" || p.FileB == "CHANGELOG.md" {
			t.Errorf("CHANGELOG should not appear in pairs: %+v", p)
		}
	}
}

func TestAnalyze_LargeCommitsExcluded(t *testing.T) {
	// One mass-edit commit touching 100 files should be skipped.
	manyFiles := make([]string, 100)
	for i := range manyFiles {
		manyFiles[i] = "f" + itoa(i) + ".go"
	}
	commits := []source.Commit{
		mkCommit("2025-01-01T00:00:00Z", manyFiles...),
	}
	for i := 0; i < 6; i++ {
		commits = append(commits, mkCommit("2025-01-02T00:00:00Z", "a.go", "b.go"))
	}

	r := Analyze(commits, Options{MaxFilesPerCommit: 50})
	if r.CommitsExcluded != 1 {
		t.Errorf("CommitsExcluded: got %d, want 1", r.CommitsExcluded)
	}
	if len(r.Pairs) != 1 {
		t.Errorf("expected 1 pair (a.go ↔ b.go), got %d", len(r.Pairs))
	}
}

func TestAnalyze_RespectsIgnore(t *testing.T) {
	commits := []source.Commit{}
	for i := 0; i < 6; i++ {
		commits = append(commits, mkCommit("2025-01-01T00:00:00Z", "vendor/foo.go", "internal/bar.go"))
	}
	r := Analyze(commits, Options{Ignore: []string{"vendor/"}})
	if len(r.Pairs) != 0 {
		t.Errorf("vendor pair should be filtered; got %+v", r.Pairs)
	}
}

func TestAnalyze_ComponentPairs(t *testing.T) {
	// Coupled pair across two components.
	commits := []source.Commit{}
	for i := 0; i < 6; i++ {
		commits = append(commits, mkCommit("2025-01-01T00:00:00Z", "app/controllers/orders.rb", "app/services/order_service.rb"))
	}
	r := Analyze(commits, Options{ComponentDepth: 2})
	if len(r.ComponentPairs) != 1 {
		t.Fatalf("component pair count: got %d, want 1", len(r.ComponentPairs))
	}
	cp := r.ComponentPairs[0]
	if cp.CompA != "app/controllers" || cp.CompB != "app/services" {
		t.Errorf("component pair: got (%q,%q), want (app/controllers,app/services)", cp.CompA, cp.CompB)
	}
	if cp.PairCount != 1 || cp.TotalSupport != 6 {
		t.Errorf("component pair counts: pair=%d support=%d", cp.PairCount, cp.TotalSupport)
	}
}

func TestAnalyze_EmptyInput(t *testing.T) {
	r := Analyze(nil, Options{})
	if len(r.Pairs) != 0 || len(r.ComponentPairs) != 0 {
		t.Fatalf("empty input must yield empty output")
	}
}

// helper kept terse to avoid pulling fmt for a single int conversion
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
