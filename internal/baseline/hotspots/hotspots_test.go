package hotspots

import (
	"regexp"
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

func TestDefaultBugPattern_PositiveCases(t *testing.T) {
	positives := []string{
		// Conventional Commits
		"fix: handle nil response",
		"fix(payments): retry on timeout",
		"fix!: drop deprecated field",
		"Fix: typo in readme",
		"FIX: caps still match",
		"hotfix: emergency patch",
		"hotfix(stripe): rollback webhook",
		"revert: undo bad merge",
		"patch: bump dep",
		"regression: scroll resets to top",
		// GitHub auto-close
		"Fixes #123 — payment double-charge",
		"Closes #45",
		"closed #999 finally",
		"fix #7",
		// Freeform with the words we explicitly look for
		"hotfix the carteirinha redirect",
		"investigating regression in checkout",
	}
	for _, msg := range positives {
		if !DefaultBugPattern.MatchString(msg) {
			t.Errorf("expected match: %q", msg)
		}
	}
}

func TestDefaultBugPattern_NegativeCases(t *testing.T) {
	// These must NOT match. The risky ones are messages that contain "fix"
	// as a substring of a different word — we want to avoid false positives.
	negatives := []string{
		"feat: add new endpoint",
		"docs: update CONTRIBUTING",
		"chore: bump go.mod",
		"refactor: extract service",
		"add prefix to env vars",                     // "prefix"
		"affixed sticker",                            // "affixed"
		"build: introduce a fixed-width font helper", // "fixed"
		"feat(api): suffix-aware lookup",
	}
	for _, msg := range negatives {
		if DefaultBugPattern.MatchString(msg) {
			t.Errorf("unexpected match: %q", msg)
		}
	}
}

func TestAnalyze_ScoreIsChurnTimesBugs(t *testing.T) {
	commits := []source.Commit{
		// File a.go: 4 commits, 2 fixes → score 8
		{Author: "x", When: ts("2025-01-01T00:00:00Z"), Files: []string{"a.go"}, Message: "feat: initial"},
		{Author: "x", When: ts("2025-01-02T00:00:00Z"), Files: []string{"a.go"}, Message: "fix: bug 1"},
		{Author: "x", When: ts("2025-01-03T00:00:00Z"), Files: []string{"a.go"}, Message: "refactor: cleanup"},
		{Author: "x", When: ts("2025-01-04T00:00:00Z"), Files: []string{"a.go"}, Message: "fix: bug 2"},

		// File b.go: 3 commits, 0 fixes → score 0
		{Author: "y", When: ts("2025-01-05T00:00:00Z"), Files: []string{"b.go"}, Message: "feat: x"},
		{Author: "y", When: ts("2025-01-06T00:00:00Z"), Files: []string{"b.go"}, Message: "feat: y"},
		{Author: "y", When: ts("2025-01-07T00:00:00Z"), Files: []string{"b.go"}, Message: "docs: z"},

		// File c.go: 2 commits, 1 fix → score 2
		{Author: "z", When: ts("2025-01-08T00:00:00Z"), Files: []string{"c.go"}, Message: "feat: c"},
		{Author: "z", When: ts("2025-01-09T00:00:00Z"), Files: []string{"c.go"}, Message: "Fixes #42"},
	}

	r := Analyze(commits, Options{})

	if r.CommitsAnalyzed != 9 {
		t.Errorf("CommitsAnalyzed: got %d, want 9", r.CommitsAnalyzed)
	}
	if r.FixCommits != 3 {
		t.Errorf("FixCommits: got %d, want 3", r.FixCommits)
	}
	if r.FilesAnalyzed != 3 {
		t.Errorf("FilesAnalyzed: got %d, want 3", r.FilesAnalyzed)
	}
	if r.FilesTouchedByFix != 2 {
		t.Errorf("FilesTouchedByFix: got %d, want 2", r.FilesTouchedByFix)
	}

	if got := r.Files[0].Path; got != "a.go" {
		t.Fatalf("first file: got %q, want a.go", got)
	}
	if r.Files[0].Score != 8 || r.Files[0].Churn != 4 || r.Files[0].BugCommits != 2 {
		t.Errorf("a.go scoring: score=%d churn=%d bugs=%d", r.Files[0].Score, r.Files[0].Churn, r.Files[0].BugCommits)
	}
	if got := r.Files[1].Path; got != "c.go" {
		t.Errorf("second file: got %q, want c.go", got)
	}
	if got := r.Files[2].Path; got != "b.go" {
		t.Errorf("third file: got %q, want b.go", got)
	}
	if r.Files[2].Score != 0 {
		t.Errorf("b.go score: got %d, want 0", r.Files[2].Score)
	}
}

func TestAnalyze_DedupesFilesWithinCommit(t *testing.T) {
	commits := []source.Commit{
		{Author: "x", When: ts("2025-01-01T00:00:00Z"), Files: []string{"a.go", "a.go"}, Message: "fix: dup"},
	}
	r := Analyze(commits, Options{})
	if r.Files[0].Churn != 1 {
		t.Fatalf("churn with dup files: got %d, want 1", r.Files[0].Churn)
	}
}

func TestAnalyze_RespectsIgnore(t *testing.T) {
	commits := []source.Commit{
		{Author: "x", When: ts("2025-01-01T00:00:00Z"), Files: []string{"vendor/foo.go", "internal/cli/root.go"}, Message: "fix: x"},
	}
	r := Analyze(commits, Options{Ignore: []string{"vendor/"}})
	if r.FilesAnalyzed != 1 {
		t.Fatalf("FilesAnalyzed: got %d, want 1 (vendor must be skipped)", r.FilesAnalyzed)
	}
	if r.Files[0].Path != "internal/cli/root.go" {
		t.Errorf("kept wrong file: %q", r.Files[0].Path)
	}
}

func TestAnalyze_EmptyInput(t *testing.T) {
	r := Analyze(nil, Options{})
	if r.FilesAnalyzed != 0 || r.FixCommits != 0 || r.CommitsAnalyzed != 0 {
		t.Fatalf("empty input must yield zeroed report")
	}
}

func TestAnalyze_CustomBugPattern(t *testing.T) {
	custom := regexp.MustCompile(`(?i)\[bug\]`)
	commits := []source.Commit{
		{Author: "x", When: ts("2025-01-01T00:00:00Z"), Files: []string{"a.go"}, Message: "[BUG] crash on null"},
		{Author: "x", When: ts("2025-01-02T00:00:00Z"), Files: []string{"a.go"}, Message: "feat: add metric"},
	}
	r := Analyze(commits, Options{BugPattern: custom})
	if r.FixCommits != 1 {
		t.Errorf("FixCommits with custom pattern: got %d, want 1", r.FixCommits)
	}
}

func TestComponentAggregation(t *testing.T) {
	commits := []source.Commit{
		{Author: "x", When: ts("2025-01-01T00:00:00Z"), Files: []string{"app/svc/a.rb", "app/svc/b.rb"}, Message: "fix: foo"},
		{Author: "x", When: ts("2025-01-02T00:00:00Z"), Files: []string{"app/svc/a.rb"}, Message: "fix: bar"},
		{Author: "x", When: ts("2025-01-03T00:00:00Z"), Files: []string{"app/api/c.rb"}, Message: "feat: x"},
	}
	r := Analyze(commits, Options{ComponentDepth: 2})

	want := map[string]bool{"app/svc": false, "app/api": false}
	for _, c := range r.Components {
		if _, ok := want[c.Path]; ok {
			want[c.Path] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("component %q missing", k)
		}
	}
	// app/svc should rank first: a.rb has churn=2,bugs=2 → score=4; b.rb has churn=1,bugs=1 → score=1; sum=5.
	if r.Components[0].Path != "app/svc" {
		t.Errorf("top component: got %q, want app/svc", r.Components[0].Path)
	}
	if r.Components[0].ScoreSum != 5 {
		t.Errorf("app/svc score sum: got %d, want 5", r.Components[0].ScoreSum)
	}
}
