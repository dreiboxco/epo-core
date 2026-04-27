package busfactor

import (
	"testing"
	"time"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// commitsFromTable expands a compact spec ([author, file...] per row) into
// []Commit with deterministic timestamps spaced one hour apart.
func commitsFromTable(rows [][]string) []Commit {
	base := ts("2025-01-01T00:00:00Z")
	out := make([]Commit, 0, len(rows))
	for i, r := range rows {
		out = append(out, Commit{
			SHA:    "c" + itoa(i),
			Author: r[0],
			When:   base.Add(time.Duration(i) * time.Hour),
			Files:  r[1:],
		})
	}
	return out
}

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

func TestComputeBusFactor_SingleAuthor(t *testing.T) {
	bf, top, share := computeBusFactor(map[string]int{"alice": 7}, 7, 0.5)
	if bf != 1 {
		t.Fatalf("bus factor: got %d, want 1", bf)
	}
	if top != "alice" || share != 1.0 {
		t.Fatalf("top author/share: got %q %.2f", top, share)
	}
}

func TestComputeBusFactor_TwoAuthorsBalanced(t *testing.T) {
	bf, _, _ := computeBusFactor(map[string]int{"alice": 5, "bob": 5}, 10, 0.5)
	if bf != 1 {
		t.Fatalf("bus factor: got %d, want 1 (alice alone reaches 50%%)", bf)
	}
}

func TestComputeBusFactor_NeedsTwo(t *testing.T) {
	bf, _, _ := computeBusFactor(map[string]int{"alice": 4, "bob": 4, "carol": 2}, 10, 0.5)
	if bf != 2 {
		t.Fatalf("bus factor: got %d, want 2 (alice 40%% < 50%%, alice+bob 80%% >= 50%%)", bf)
	}
}

func TestComputeBusFactor_Threshold80(t *testing.T) {
	// alice 6, bob 3, carol 1 → at 80% we need alice+bob (90% ≥ 80%); alice alone is 60% < 80%.
	bf, _, _ := computeBusFactor(map[string]int{"alice": 6, "bob": 3, "carol": 1}, 10, 0.8)
	if bf != 2 {
		t.Fatalf("bus factor at 0.8 threshold: got %d, want 2", bf)
	}
}

func TestAnalyze_MarksBusFactorOne(t *testing.T) {
	commits := commitsFromTable([][]string{
		{"alice", "internal/cli/root.go"},
		{"alice", "internal/cli/root.go"},
		{"alice", "internal/cli/root.go"},
		{"bob", "internal/cli/baseline.go"},
		{"alice", "internal/cli/baseline.go"},
	})
	r := Analyze(commits, Options{})

	if r.FilesAnalyzed != 2 {
		t.Fatalf("files: got %d, want 2", r.FilesAnalyzed)
	}
	if r.BusFactorOneFiles != 2 {
		t.Fatalf("bus-factor-1 files: got %d, want 2", r.BusFactorOneFiles)
	}

	// Files are sorted by bus factor asc, then commits desc; root.go has 3 commits, baseline.go has 2.
	if got := r.Files[0].Path; got != "internal/cli/root.go" {
		t.Fatalf("first file: got %q, want internal/cli/root.go", got)
	}
	if r.Files[0].BusFactor != 1 || r.Files[0].TopAuthor != "alice" {
		t.Fatalf("root.go bus factor/top: got %d/%s", r.Files[0].BusFactor, r.Files[0].TopAuthor)
	}
}

func TestAnalyze_ComponentAggregation(t *testing.T) {
	commits := commitsFromTable([][]string{
		{"alice", "internal/cli/root.go"},
		{"alice", "internal/cli/root.go"},
		{"bob", "internal/cli/baseline.go"},
		{"bob", "internal/cli/baseline.go"},
		{"carol", "cmd/epo/main.go"},
		{"alice", "cmd/epo/main.go"},
	})
	r := Analyze(commits, Options{ComponentDepth: 2})

	want := map[string]bool{"internal/cli": false, "cmd/epo": false}
	for _, c := range r.Components {
		if _, ok := want[c.Path]; ok {
			want[c.Path] = true
		}
		switch c.Path {
		case "internal/cli":
			if c.MinBusFactor != 1 || c.FilesAnalyzed != 2 {
				t.Errorf("internal/cli: bf=%d files=%d", c.MinBusFactor, c.FilesAnalyzed)
			}
		case "cmd/epo":
			if c.FilesAnalyzed != 1 {
				t.Errorf("cmd/epo: files=%d", c.FilesAnalyzed)
			}
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("component %q missing from result", k)
		}
	}
}

func TestAnalyze_IgnoreVendor(t *testing.T) {
	commits := commitsFromTable([][]string{
		{"alice", "vendor/github.com/foo/foo.go"},
		{"alice", "internal/cli/root.go"},
	})
	r := Analyze(commits, Options{Ignore: []string{"vendor/"}})
	if r.FilesAnalyzed != 1 {
		t.Fatalf("files: got %d, want 1 (vendor must be skipped)", r.FilesAnalyzed)
	}
	if r.Files[0].Path != "internal/cli/root.go" {
		t.Fatalf("unexpected file: %q", r.Files[0].Path)
	}
}

func TestAnalyze_DedupesFilesWithinCommit(t *testing.T) {
	commits := []Commit{{
		SHA:    "abc",
		Author: "alice",
		When:   ts("2025-01-01T00:00:00Z"),
		Files:  []string{"a.go", "a.go", "a.go"},
	}}
	r := Analyze(commits, Options{})
	if r.Files[0].Commits != 1 {
		t.Fatalf("commits: got %d, want 1 (duplicates within a commit must collapse)", r.Files[0].Commits)
	}
}

func TestAnalyze_EmptyInput(t *testing.T) {
	r := Analyze(nil, Options{})
	if r.FilesAnalyzed != 0 || len(r.Files) != 0 {
		t.Fatalf("empty input should yield empty report")
	}
}

func TestComponentKey(t *testing.T) {
	cases := []struct {
		in    string
		depth int
		want  string
	}{
		{"internal/cli/root.go", 2, "internal/cli"},
		{"internal/cli/root.go", 1, "internal"},
		{"internal/cli/foo/bar.go", 2, "internal/cli"},
		{"main.go", 2, "."},
		{"cmd/epo/main.go", 2, "cmd/epo"},
		{"cmd/epo/main.go", 0, "cmd/epo/main.go"},
	}
	for _, c := range cases {
		got := componentKey(c.in, c.depth)
		if got != c.want {
			t.Errorf("componentKey(%q,%d)=%q, want %q", c.in, c.depth, got, c.want)
		}
	}
}
