// Package git wraps go-git to produce a stream of busfactor.Commit records
// from a local repository.
package git

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/dreiboxco/epo-core/internal/baseline/busfactor"
)

// LoadOptions configures the commit walk.
type LoadOptions struct {
	// Path is the local path to a git working tree (or a bare repo).
	Path string
	// Since restricts commits to those authored at or after this instant.
	// Zero value disables the filter.
	Since time.Time
	// IncludeMerges keeps merge commits in the output. Default is to skip
	// them — merges typically duplicate authorship signal.
	IncludeMerges bool
}

// Load opens the repo at opts.Path and walks the HEAD branch, returning a
// slice of busfactor.Commit ready for analysis.
//
// The implementation diffs each commit against its first parent (or against
// an empty tree for the root commit) to recover the list of changed files.
// Renames are reported as a deletion of the old path and an addition of the
// new path; merging into a single "rename" is left to a future iteration.
func Load(opts LoadOptions) ([]busfactor.Commit, error) {
	if opts.Path == "" {
		return nil, errors.New("git.Load: Path is required")
	}

	repo, err := git.PlainOpen(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("open repo at %s: %w", opts.Path, err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}

	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return nil, fmt.Errorf("walk log: %w", err)
	}
	defer iter.Close()

	var out []busfactor.Commit
	walkErr := iter.ForEach(func(c *object.Commit) error {
		if !opts.Since.IsZero() && c.Author.When.Before(opts.Since) {
			return nil
		}
		if !opts.IncludeMerges && c.NumParents() > 1 {
			return nil
		}

		files, err := changedFiles(c)
		if err != nil {
			return fmt.Errorf("changed files for %s: %w", c.Hash, err)
		}
		if len(files) == 0 {
			return nil
		}

		out = append(out, busfactor.Commit{
			SHA:    c.Hash.String(),
			Author: identityKey(c.Author),
			When:   c.Author.When,
			Files:  files,
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

// identityKey returns the author key the analyzer uses. Email is preferred
// because names can drift (e.g. "Aguinelo K." vs "Aguinelo Koczkodai") while
// emails usually stay stable per identity.
func identityKey(sig object.Signature) string {
	if sig.Email != "" {
		return sig.Email
	}
	return sig.Name
}
