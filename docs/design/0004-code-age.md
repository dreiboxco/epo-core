# Design Note 0004 — Code Age Distribution

## Goal

For each file, find the timestamp of its most recent commit and bucket the ages so we can answer:

- How much of the codebase is fresh (touched this month)?
- How much is stable (touched in the last year but not recently)?
- How much is **stale** — not touched in over a year, possibly forgotten?

The metric is honest about what it does: it's a distribution of "last touched" times, nothing more. The interpretation depends on the codebase. Old code that's stable and well-tested is fine. Old code combined with `busfactor=1` and high complexity is a different story — but composition of metrics is for later. v0 just produces the distribution.

## Why this matters

`kpis.md` puts it well: *"Código velho estável = bom. Código velho + ninguém entende + alta complexidade = risco. Combinado com bus factor, mostra o que está 'congelado por medo'."* For v0 we compute the raw age; the combination with bus factor is a follow-up that the renderer or a later "composite" command can do.

## Non-goals (for v0)

- No first-touch timestamp (oldest commit per file). We report only the most recent.
- No combination with bus factor or complexity in the report itself. Composite views are deferred.
- No working-tree filtering: a file deleted from HEAD will not appear at all (no commits in window) but if it was renamed, both old and new paths are tracked. v0 is silent about this; future work can detect renames properly.
- No date arithmetic for irregular calendars (months counted as 30 days, years as 365). Off by a few days for reporting purposes; nobody cares for this use case.

## Inputs

Same flag surface as the other metrics. Two notes:

- `--since` still works but means "only consider commits in this window when computing the most recent timestamp." For an honest all-time age distribution, **omit `--since` or pass `--since 0`**. The CLI default of 12 months is fine for "freshness within the year" but truncates anything older.
- A new flag, `--age-buckets`, is introduced so teams can override the boundaries. Default boundaries (in days): `30, 90, 365`. That gives 4 buckets: `<30d`, `30-90d`, `90-365d`, `>365d` — matching the kpis.md spec.

## Algorithm

1. Load commits.
2. For each commit, for each file (post-ignore, post-dedup), update `latestCommit[file] = max(existing, commit.When)`.
3. For each file: `ageDays = now - latestCommit[file]` (in days, integer).
4. Bucket each file into the smallest boundary `b` such that `ageDays < b`. The last bucket is "older than the highest boundary".
5. Aggregate per component: median age (file ages), oldest age, count per bucket.
6. Sort files by age desc (oldest first) for the top-N "stale" list.

## Output format (markdown)

```markdown
# Code Age Report — <repo>

Generated: <date>
Window: <range or "all time">
Buckets: <30d, 30-90d, 90-365d, >365d (default)

## Summary

| Metric | Value |
|---|---|
| Files analyzed | N |
| Median age | X days |
| Oldest file age | Y days |
| Newest file age | Z days |

## Age distribution

```
<30d    │ ██ N₁
30-90d  │ ███ N₂
90-365d │ █████ N₃
>365d   │ ███████ N₄
```

## Top 20 stalest files

| File | Age (days) | Last touched |

## Components by staleness

| Component | Files | Median age | Oldest age | Stale (>365d) files |
```

## Reuse

Same `source.Commit` input, same loader, same component aggregation idiom. The bucketing helper (`bucketize(days, []int)`) is local to the package and small enough that extracting it is premature.

## Testing strategy

- **Unit**: pure functions over `[]source.Commit` with deterministic timestamps. Verify bucketing edges (a file aged exactly 30 days lands in `30-90d`, not `<30d` — boundary is `<`), median computation, oldest-first sorting.
- **Smoke**: epo-core (everything will be 0 days old, all in `<30d` bucket — boring but validates the plumbing) and Embarca repos (where we'll see real distribution).

## Future improvements

- First-touch timestamp (file age proper, not last-touched).
- Composite "frozen by fear" view: stale × BF=1 × high complexity.
- Trend over time: same file's age distribution at different snapshots.
- Detect renames so a `git mv` doesn't reset perceived age.

## References

- `knowledge/product/kpis.md` (Code Age Distribution row)
