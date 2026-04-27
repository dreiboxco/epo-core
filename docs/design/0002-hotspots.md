# Design Note 0002 — Code Hotspots

## Goal

Surface the files most likely to harbor bugs, by combining how often a file changes (churn) with how often those changes are bug fixes. The classic Tornhill heuristic: **score = churn × bug_commits**.

A file with high churn and many fix commits is an active source of pain. A file with high churn and few fixes is just under heavy normal development. A file with low churn is irrelevant regardless of fix density. Multiplication captures all three cases naturally.

## Non-goals (for v0)

- No bug-tracker integration (Linear, Jira). Bug detection is purely heuristic on commit messages.
- No weighting by lines changed.
- No author dimension (the bus factor metric covers ownership; hotspots is orthogonal).
- No comparison across repos. One repo per scan; aggregation is the renderer's job, not the analyzer's.

## Inputs

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local git working tree |
| `--since` | `12 months` | Time window |
| `--bug-pattern` | (default regex) | Override the regex used to classify bug-fix commits |
| `--component-depth` | `2` | Path-prefix depth for component aggregation |
| `--ignore` | (defaults) | Path prefixes to skip |
| `--top` | `20` | Top N files in the report |

Reuses every flag the bus factor command already understands. Same loader, same windowing semantics.

## Bug detection

A commit is classified as a "fix" if its message matches the **default regex** (case-insensitive):

```
^\s*(fix|bug|hotfix|revert|patch|regression)(\(|:|!|\s|$)
|\bfixes?\s+#\d+
|\bcloses?\s+#\d+
|\bfix\b
|\bbug\b
|\bhotfix\b
|\bregression\b
```

The first alternative covers Conventional Commits (`fix:`, `fix(scope):`, `fix!:`). The `Fixes #123` / `Closes #45` alternatives cover the GitHub linking convention. The remaining word-boundary checks catch loose conventions ("Fix typo in payments_controller", "Bug fix for stripe webhook", "Hotfix: revert refund") without being so loose that "prefixes" or "fixed-width" leak in.

This is intentionally a **simple heuristic**, not a classifier. False positives and negatives are expected. Two safeguards:

1. The pattern is overrideable per-team via `--bug-pattern`.
2. The report shows the **classification share** so reviewers can sanity-check ("18% of commits in this window were classified as fixes — does that match your perception?"). If it's wildly off, the team tunes the regex.

## Algorithm

1. Load commits via the existing loader (already provides Message).
2. For each commit:
   - Classify as fix or non-fix.
   - For each touched file (post-ignore, post-dedup), increment `churn[file]` by 1, and `bugs[file]` by 1 if the commit was a fix.
3. Score each file: `score = churn[file] * bugs[file]`. Files that never appeared in a fix get score 0 and drop off the ranking.
4. Aggregate per component: sum of churn, sum of bugs, sum of scores; also show file-count and median.
5. Render markdown with: summary, top-N files, per-component table, fix-classification breakdown (so reviewers can audit the heuristic).

## Output format (markdown)

```markdown
# Hotspots Report — <repo>

Generated: <date>
Window: <range>
Bug pattern: default | custom

## Summary

| Metric | Value |
|---|---|
| Files analyzed | N |
| Commits analyzed | M |
| Commits classified as fix | F (X%)  |
| Files touched by ≥1 fix | K |

## Top 20 hotspots

| File | Score | Churn | Fix commits | Fix rate |

## Components by hotspot pressure

| Component | Files | Total churn | Total fixes | Score sum |

## Bug classification audit

Sample of classified fix commits (first 10): … | non-fix (first 10): …
```

The audit section is the key novelty. We *expect* the regex to be wrong sometimes, and surfacing examples is how the team fixes it without us having to be perfect.

## Reuse from busfactor

- Same `source.Commit` input type.
- Same `git.Load` invocation.
- Same `--since`, `--component-depth`, `--ignore` semantics — extracted to a shared helper if needed.
- Same markdown style — same renderer package.

## Testing strategy

- **Unit**: pure functions over `[]source.Commit`. Verify classification on a curated table of commit messages (Conventional, GitHub linking, freeform, plus negatives that must not match like "fix typo" misspellings, "prefix changes", "affixed text"). Verify scoring with a tiny commit set where the answer is hand-computed.
- **Integration**: against epo-core itself (small, predictable history). Sanity-check that bootstrap commits aren't false-positives.
- **Smoke**: against the Embarca repos.

## Future improvements (out of scope for v0)

- Lines-of-change weighting (replace churn count with delta lines).
- Issue-tracker integration to ground-truth the fix label.
- Time-decay (recent fixes count more than year-old fixes).
- Cross-repo hotspots: when a fix in repo A consistently follows a change in repo B, flag the coupling. (This is Fase 2 territory — the catalog graph makes it possible.)
- Confidence score: how reliable is the classification given message volume?

## Open questions

- Should the score normalize by file age (a 3-year-old file with 30 fixes vs. a 6-month-old file with 30 fixes)? Probably yes eventually, but the v0 window-based approach already constrains comparison to the same period.
- Should we deduplicate revert pairs ("fix X" + "Revert fix X") to avoid double-counting? Probably yes, but only after we have a real example where it skews the ranking.

## References

- `knowledge/product/kpis.md` (Code Hotspots row)
- Adam Tornhill, *Your Code as a Crime Scene* (2015)
- [code-maat](https://github.com/adamtornhill/code-maat)
