package silos

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

func TestAnalyze_ScoreIsChurnOverContributors(t *testing.T) {
	commits := []source.Commit{
		// a.go: 4 commits, all by alice → churn 4, contribs 1, score 4
		{Author: "alice", When: ts("2025-01-01T00:00:00Z"), Files: []string{"a.go"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-02T00:00:00Z"), Files: []string{"a.go"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-03T00:00:00Z"), Files: []string{"a.go"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-04T00:00:00Z"), Files: []string{"a.go"}, Message: "x"},

		// b.go: 4 commits, 4 different authors → churn 4, contribs 4, score 1
		{Author: "alice", When: ts("2025-01-05T00:00:00Z"), Files: []string{"b.go"}, Message: "x"},
		{Author: "bob", When: ts("2025-01-06T00:00:00Z"), Files: []string{"b.go"}, Message: "x"},
		{Author: "carol", When: ts("2025-01-07T00:00:00Z"), Files: []string{"b.go"}, Message: "x"},
		{Author: "dave", When: ts("2025-01-08T00:00:00Z"), Files: []string{"b.go"}, Message: "x"},

		// c.go: 1 commit, alice → churn 1, contribs 1, score 1
		{Author: "alice", When: ts("2025-01-09T00:00:00Z"), Files: []string{"c.go"}, Message: "x"},

		// d.go: 6 commits, 2 contributors (alice ×4, bob ×2) → churn 6, contribs 2, score 3
		{Author: "alice", When: ts("2025-01-10T00:00:00Z"), Files: []string{"d.go"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-11T00:00:00Z"), Files: []string{"d.go"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-12T00:00:00Z"), Files: []string{"d.go"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-13T00:00:00Z"), Files: []string{"d.go"}, Message: "x"},
		{Author: "bob", When: ts("2025-01-14T00:00:00Z"), Files: []string{"d.go"}, Message: "x"},
		{Author: "bob", When: ts("2025-01-15T00:00:00Z"), Files: []string{"d.go"}, Message: "x"},
	}

	r := Analyze(commits, Options{})

	if r.FilesAnalyzed != 4 {
		t.Fatalf("FilesAnalyzed: got %d, want 4", r.FilesAnalyzed)
	}
	if r.SingleContribFiles != 2 { // a.go and c.go
		t.Errorf("SingleContribFiles: got %d, want 2", r.SingleContribFiles)
	}

	// Files sort: a.go (4) > d.go (3) > b.go (1) ≈ c.go (1).
	// Tie between b.go and c.go: tiebreak by churn desc, then path asc.
	// b.go has churn=4, c.go has churn=1, so b.go comes first.
	if got := r.Files[0].Path; got != "a.go" {
		t.Errorf("first file: got %q, want a.go", got)
	}
	if math.Abs(r.Files[0].Score-4.0) > 1e-9 {
		t.Errorf("a.go score: got %.4f, want 4.0", r.Files[0].Score)
	}
	if got := r.Files[1].Path; got != "d.go" {
		t.Errorf("second file: got %q, want d.go", got)
	}
	if math.Abs(r.Files[1].Score-3.0) > 1e-9 {
		t.Errorf("d.go score: got %.4f, want 3.0", r.Files[1].Score)
	}
	if got := r.Files[2].Path; got != "b.go" {
		t.Errorf("third file: got %q, want b.go", got)
	}
	if got := r.Files[3].Path; got != "c.go" {
		t.Errorf("fourth file: got %q, want c.go", got)
	}
}

func TestAnalyze_DedupesFilesWithinCommit(t *testing.T) {
	commits := []source.Commit{
		{Author: "alice", When: ts("2025-01-01T00:00:00Z"), Files: []string{"a.go", "a.go", "a.go"}, Message: "x"},
	}
	r := Analyze(commits, Options{})
	if r.Files[0].Churn != 1 {
		t.Fatalf("churn with dup files in same commit: got %d, want 1", r.Files[0].Churn)
	}
	if r.Files[0].Contributors != 1 {
		t.Fatalf("contributors: got %d, want 1", r.Files[0].Contributors)
	}
}

func TestAnalyze_RespectsIgnore(t *testing.T) {
	commits := []source.Commit{
		{Author: "alice", When: ts("2025-01-01T00:00:00Z"), Files: []string{"vendor/foo.go", "internal/cli/root.go"}, Message: "x"},
	}
	r := Analyze(commits, Options{Ignore: []string{"vendor/"}})
	if r.FilesAnalyzed != 1 {
		t.Fatalf("FilesAnalyzed: got %d, want 1", r.FilesAnalyzed)
	}
	if r.Files[0].Path != "internal/cli/root.go" {
		t.Errorf("kept wrong file: %q", r.Files[0].Path)
	}
}

func TestAnalyze_EmptyInput(t *testing.T) {
	r := Analyze(nil, Options{})
	if r.FilesAnalyzed != 0 || len(r.Files) != 0 {
		t.Fatalf("empty input must yield empty report")
	}
}

func TestComponents_RankByMaxScore(t *testing.T) {
	commits := []source.Commit{
		// app/svc has one very lonely file (score 5) and one collaborative (score 1)
		{Author: "alice", When: ts("2025-01-01T00:00:00Z"), Files: []string{"app/svc/lonely.rb"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-02T00:00:00Z"), Files: []string{"app/svc/lonely.rb"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-03T00:00:00Z"), Files: []string{"app/svc/lonely.rb"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-04T00:00:00Z"), Files: []string{"app/svc/lonely.rb"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-05T00:00:00Z"), Files: []string{"app/svc/lonely.rb"}, Message: "x"},
		{Author: "alice", When: ts("2025-01-06T00:00:00Z"), Files: []string{"app/svc/shared.rb"}, Message: "x"},
		{Author: "bob", When: ts("2025-01-07T00:00:00Z"), Files: []string{"app/svc/shared.rb"}, Message: "x"},
		{Author: "carol", When: ts("2025-01-08T00:00:00Z"), Files: []string{"app/svc/shared.rb"}, Message: "x"},

		// app/api has only collaborative files (low max score)
		{Author: "alice", When: ts("2025-01-09T00:00:00Z"), Files: []string{"app/api/x.rb"}, Message: "x"},
		{Author: "bob", When: ts("2025-01-10T00:00:00Z"), Files: []string{"app/api/x.rb"}, Message: "x"},
	}
	r := Analyze(commits, Options{ComponentDepth: 2})

	if r.Components[0].Path != "app/svc" {
		t.Errorf("top component: got %q, want app/svc", r.Components[0].Path)
	}
	if math.Abs(r.Components[0].MaxScore-5.0) > 1e-9 {
		t.Errorf("app/svc max score: got %.4f, want 5.0", r.Components[0].MaxScore)
	}
	if r.Components[0].SingleContribFiles != 1 {
		t.Errorf("app/svc single-contrib files: got %d, want 1", r.Components[0].SingleContribFiles)
	}
}
