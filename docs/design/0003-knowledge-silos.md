# Design Note 0003 — Knowledge Silos

## Goal

Surface files that change frequently **and** are understood by few people. The combination is what's dangerous: a file with 50 commits owned by one dev is a silo; a file with 50 commits and broad ownership is just active. The classic Tornhill formulation:

```
score = churn × (1 / unique_contributors)
```

A file that nobody but Alice has touched in 12 months, with 30 commits, scores 30. The same 30 commits split across 5 contributors scores 6. A file with 1 commit (no churn pressure) scores low regardless of ownership.

This is the third leg of the per-file risk triad we now compute:

- `busfactor` — *who* knows the file (concentration of authorship)
- `hotspots` — *how often* the file breaks (churn × bug-fix rate)
- `silos` — *how lonely* the file is when it changes (churn ÷ unique contributors)

These overlap. A file with bus factor 1, high score in hotspots, AND high score in silos is the worst possible combination — frequent breakage, single owner, and low organizational visibility. Phase-2 reporting can fuse them into a "top risk" composite; for now we keep them separate so the audit trail is clear.

## Non-goals (for v0)

- No identity reconciliation (`alice@home.com` vs `alice@work.com` count as two contributors). Reuses the same caveat as bus factor; future feature.
- No weighting by lines changed.
- No coupling with bug data (that's hotspots' job).
- No file-age weighting.

## Inputs

Same flag surface as the other metrics — same `--path`, `--since`, `--component-depth`, `--ignore`, `--top`, `--include-merges`, `--repo-label`. No metric-specific knobs in v0.

## Algorithm

1. Load commits with `git.Load` (no extra metadata needed beyond what we already collect).
2. For each commit, for each file (post-ignore, post-dedup):
   - Increment `churn[file]`.
   - Insert author into `contributors[file]` (a set).
3. For each file: `score = churn / len(contributors)` as a float; record (churn, contributors_count, score, last_touched).
4. Sort by score descending. Files with churn=1 and contributors=1 have score 1.0 — uninteresting; the ranking pushes them toward the bottom regardless.
5. Aggregate by component path-prefix (depth N): sum of churn, sum of unique contributors *across files*, average score, max score.
6. Render markdown with summary, top-N files, per-component table.

## Output format (markdown)

```markdown
# Knowledge Silos Report — <repo>

Window: <range>
Score formula: churn / unique_contributors

## Summary

| Metric | Value |
|---|---|
| Files analyzed | N |
| Files with single contributor | K (X%)  |
| Median contributors per file | M |

## Top 20 silos

| File | Score | Churn | Contributors | Last touched |

## Components by silo pressure

| Component | Files | Avg score | Max score | Files w/ single contributor |
```

## Reuse

The analyzer is structurally identical to bus factor's accounting loop and hotspots' churn loop — same `source.Commit` input, same component aggregation idiom. Worth duplicating for clarity in v0; the right time to extract a shared "per-file walker" helper is when we add a fourth analyzer that wants the same scaffold.

## Testing strategy

- **Unit**: hand-built `[]source.Commit` slices with predictable churn × contributor combinations. Tests verify ordering, score values, single-contributor flagging, and ignore semantics.
- **Smoke**: run against epo-core (most files have 1-2 contributors solo-founder pattern) and against Embarca repos.

## Future improvements

- Weight by lines or files touched per commit.
- Decay older contributors (someone who hasn't touched a file in 9 months may have forgotten it).
- Combine with bus factor: silo files with BF=1 are the highest-priority refactor candidates.
- Composite "top risk" report = silos ∩ hotspots ∩ BF=1 (Fase 2).

## Open questions

- Does `score = churn / contributors` need normalization across files of very different ages? Probably not for the windowed version (12 months bounds it), but worth revisiting if we ever measure all-time.
- Should we exclude bot accounts (`dependabot[bot]`, `renovate[bot]`) from contributor counts? Yes eventually — they inflate contributor counts artificially. Out of scope for v0; doable later with a `--exclude-author` flag.

## References

- `knowledge/product/kpis.md` (Knowledge Silos Score row)
- Adam Tornhill, *Your Code as a Crime Scene* (2015)
