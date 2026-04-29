// Package git wraps go-git to produce a stream of busfactor.Commit records
// from a local repository.
package git

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/dreiboxco/epo-core/internal/baseline/source"
)

// LoadOptions configures the commit walk.
type LoadOptions struct {
	// Path is the local path to a git working tree (or a bare repo).
	Path string
	// Ref is the branch, tag, or commit SHA to start the walk from. Empty
	// falls back to HEAD. Useful for measuring against a specific branch
	// (e.g. origin/main) without affecting the working tree.
	Ref string
	// Since restricts commits to those authored at or after this instant.
	// Zero value disables the filter.
	Since time.Time
	// IncludeMerges keeps merge commits in the output. Default is to skip
	// them — merges typically duplicate authorship signal.
	IncludeMerges bool
	// ExcludeAuthors drops commits whose author name OR email matches any
	// of the patterns. Useful to silence bots (dependabot, renovate,
	// github-actions[bot]) that inflate author counts and skew bus factor,
	// silos, and DORA metrics. Patterns are applied case-insensitively when
	// callers pass `(?i)` themselves; this loader does not normalize.
	ExcludeAuthors []*regexp.Regexp
}

// Load opens the repo at opts.Path and walks the HEAD branch, returning a
// slice of source.Commit ready for analysis.
//
// The implementation diffs each commit against its first parent (or against
// an empty tree for the root commit) to recover the list of changed files.
// Renames are reported as a deletion of the old path and an addition of the
// new path; merging into a single "rename" is left to a future iteration.
func Load(opts LoadOptions) ([]source.Commit, error) {
	if opts.Path == "" {
		return nil, errors.New("git.Load: Path is required")
	}

	repo, err := git.PlainOpen(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("open repo at %s: %w", opts.Path, err)
	}

	startHash, err := resolveRef(repo, opts.Ref)
	if err != nil {
		return nil, err
	}

	iter, err := repo.Log(&git.LogOptions{From: startHash})
	if err != nil {
		return nil, fmt.Errorf("walk log: %w", err)
	}
	defer iter.Close()

	var out []source.Commit
	walkErr := iter.ForEach(func(c *object.Commit) error {
		if !opts.Since.IsZero() && c.Author.When.Before(opts.Since) {
			return nil
		}
		if !opts.IncludeMerges && c.NumParents() > 1 {
			return nil
		}
		if authorExcluded(c.Author, opts.ExcludeAuthors) {
			return nil
		}

		files, err := changedFiles(c)
		if err != nil {
			return fmt.Errorf("changed files for %s: %w", c.Hash, err)
		}
		if len(files) == 0 {
			return nil
		}

		out = append(out, source.Commit{
			SHA:     c.Hash.String(),
			Author:  identityKey(c.Author),
			When:    c.Author.When,
			Files:   files,
			Message: c.Message,
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	// Stable order: oldest first. The analyzer doesn't care, but a
	// deterministic order helps debugging.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].When.Before(out[j].When)
	})

	return out, nil
}

func changedFiles(c *object.Commit) ([]string, error) {
	commitTree, err := c.Tree()
	if err != nil {
		return nil, err
	}

	var parentTree *object.Tree
	if c.NumParents() > 0 {
		parent, err := c.Parents().Next()
		if err != nil {
			return nil, err
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return nil, err
		}
	}

	changes, err := commitTree.Diff(parentTree)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	for _, ch := range changes {
		from, to := ch.From.Name, ch.To.Name
		if from != "" {
			seen[from] = struct{}{}
		}
		if to != "" && to != from {
			seen[to] = struct{}{}
		}
	}
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, nil
}

// authorExcluded reports whether the signature matches any of the exclusion
// patterns. The patterns are tested against name and email independently;
// either match drops the commit.
func authorExcluded(sig object.Signature, patterns []*regexp.Regexp) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, p := range patterns {
		if p == nil {
			continue
		}
		if sig.Name != "" && p.MatchString(sig.Name) {
			return true
		}
		if sig.Email != "" && p.MatchString(sig.Email) {
			return true
		}
	}
	return false
}

// identityKey returns the author key the analyzer uses. Email is preferred
// because names can drift (e.g. "Aguinelo K." vs "Aguinelo Koczkodai") while
// emails usually stay stable per identity.
func identityKey(sig object.Signature) string {
	if sig.Email != "" {
		return sig.Email
	}
	return sig.Name
}

// resolveRef turns a ref name (branch, tag, remote-tracking branch, or full
// SHA) into a commit hash. Empty ref falls back to HEAD.
//
// Resolution order: HEAD → revision (handles "origin/main", tags, full SHAs,
// short SHAs that go-git can disambiguate) → local branch → tag.
func resolveRef(repo *git.Repository, ref string) (plumbing.Hash, error) {
	if ref == "" {
		head, err := repo.Head()
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("resolve HEAD: %w", err)
		}
		return head.Hash(), nil
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err == nil && hash != nil {
		return *hash, nil
	}
	return plumbing.ZeroHash, fmt.Errorf("resolve ref %q: %w", ref, err)
}

// integrationBranchHeuristic lists the branch names to probe when
// `refs/remotes/origin/HEAD` is not set. Order is intentional: more common
// integration branches first, less common (or release-only on Git Flow) last.
var integrationBranchHeuristic = []string{"main", "master", "trunk", "develop", "development"}

// ResolveIntegrationBranch returns the name of the branch that should be
// treated as the integration target for repo at path. Tries, in order:
//
//  1. Symbolic ref `refs/remotes/origin/HEAD` (set by `git remote set-head`).
//  2. The first remote branch in integrationBranchHeuristic that exists.
//
// Returns the bare branch name (e.g. "main"), not the remote-prefixed form.
// Callers typically pass this back as LoadOptions.Ref qualified as
// "origin/<name>" if they want to walk the remote tracking branch.
func ResolveIntegrationBranch(path string) (string, error) {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return "", fmt.Errorf("open repo at %s: %w", path, err)
	}

	if name, ok := readSymbolicOriginHead(repo); ok {
		return name, nil
	}

	for _, candidate := range integrationBranchHeuristic {
		if _, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", candidate), true); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no integration branch found: origin/HEAD unset and none of %v exist on origin", integrationBranchHeuristic)
}

// readSymbolicOriginHead reads `refs/remotes/origin/HEAD` and returns the
// branch name it points to, if the symbolic ref is set. go-git stores
// symbolic refs in the packed-refs/loose-refs storage; we read raw to
// preserve the symbolic target without resolving through it.
func readSymbolicOriginHead(repo *git.Repository) (string, bool) {
	refs, err := repo.Storer.IterReferences()
	if err != nil {
		return "", false
	}
	defer refs.Close()

	target := plumbing.NewRemoteHEADReferenceName("origin")
	var found bool
	var branchName string
	_ = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name() != target {
			return nil
		}
		if ref.Type() != plumbing.SymbolicReference {
			return nil
		}
		// Symbolic target looks like "refs/remotes/origin/main"; strip prefix.
		const prefix = "refs/remotes/origin/"
		name := ref.Target().String()
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			branchName = name[len(prefix):]
			found = true
		}
		return nil
	})
	return branchName, found
}
