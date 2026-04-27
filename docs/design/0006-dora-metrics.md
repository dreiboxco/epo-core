# Design Note 0006 — DORA Phase-1 Metrics

## Scope

Two metrics in this round:

- **Deployment Frequency** — how often code lands on the main branch
- **Change Failure Rate** — what share of those landings end up needing a fix

Both are Git-only and reuse the commit stream we already load. They form the bottom of the DORA pyramid, and together they are the basic "are we shipping fast and is it safe?" pair.

The remaining two DORA metrics are deferred:

- **Lead Time for Changes** — the meaningful version (first commit → production) requires deploy timestamps and ideally PR review timing. Partial Git-only versions exist (first commit on branch → merge commit) but are noisy enough that we'd rather wait for the GitHub adapter to do it properly. Tracked for a future design note.
- **Rework Rate** — line-level analysis via `git blame` on every tracked file. Doable in pure go-git but a meaningfully different shape from our current per-commit aggregation. Deferred to its own design note.

## Why ship Deploy Freq + CFR together

They're orthogonal in narrative but share the same analytical substrate (a stream of commits bucketed by time). Implementing them separately means two reports that always get read together. So we ship them as a single metric — `dora` — with one report that includes both views.

Two analyzers under the hood (`internal/baseline/deploy/` and `internal/baseline/failure/`), so future composition with lead time / rework is clean. One CLI flag, one renderer, one report.

## Deployment Frequency

### Definition

Number of commits that "land" per time bucket. For v0 we use **commits on `HEAD`** as the proxy for landings. The CLI loader already walks `HEAD` and skips merges by default, so what we count is effectively non-merge commits hitting the default branch.

Future refinements (out of scope for v0):

- Tag-based mode (`--deploy-mode tags`) for projects that release on tag push.
- Merge-commit-based mode (`--deploy-mode merges`) for projects that merge feature branches.
- Multiple-branch support.

### Buckets

Default: weekly. Configurable: `daily`, `weekly`, `monthly`.

### Outputs

- Total deploys in window
- Mean / median per bucket
- Last bucket count (a freshness sniff)
- Distribution histogram

## Change Failure Rate

### Definition

Share of commits classified as bug fixes, computed per the same regex used by the hotspots metric. The denominator is the total number of commits considered (after merge filtering, ignore prefixes, etc.).

### Caveats (carried forward from hotspots)

- The regex is a **heuristic**. False positives ("fix typo") and false negatives (a bug fixed in a `chore:` commit) will exist.
- v0 uses `commits classified as fix / total commits`, not the more rigorous DORA definition (`failed deploys / total deploys`). The latter requires distinguishing "failed deploy" from "later commit that fixes it" — not a thing we can recover from commits alone. The proxy we use here is the standard Git-only approximation; teams should treat it as a directional signal, not a contractual SLA.

### Outputs

- Overall fix rate
- Per-period fix rate (so trend is visible)
- Top buckets by failure rate (worst weeks)

## Inputs

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local git working tree |
| `--since` | `12 months` | Time window |
| `--bucket` | `weekly` | `daily`, `weekly`, or `monthly` |
| `--bug-pattern` | (regex) | Override for the bug-fix classifier (same as hotspots) |
| `--include-merges` | false | Include merge commits |
| `--ignore` | (defaults) | Path prefixes to skip |
| `--top` | `10` | Top N "worst" buckets shown |

## Output format (markdown)

```markdown
# DORA Phase-1 Report — <repo>

Window: <range>
Bucket: weekly | daily | monthly

## Deployment Frequency

| Metric | Value |
|---|---|
| Total commits in window | N |
| Buckets analyzed | B |
| Mean per bucket | x |
| Median per bucket | y |
| Last bucket | z |

```
<histogram of commits per bucket, oldest → newest>
```

## Change Failure Rate

| Metric | Value |
|---|---|
| Commits classified as fix | F |
| Total commits | N |
| Overall failure rate | F/N (X%)  |

| Bucket | Total | Fixes | Failure rate |
| ... |

## Top 10 worst buckets by failure rate
```

The two analyzers are kept as separate packages so a future "lead time" or "rework" command can reuse the same per-commit stream substrate without dragging the renderer along.

## Testing strategy

- **Unit**: hand-built `[]source.Commit` slices with deterministic timestamps. Verify bucketing edges (weekly bucket boundaries fall on Mondays UTC), per-bucket counts, fix-rate math.
- **Smoke**: epo-core (very few commits, trivial output) + Embarca repos.

## Future improvements

- Tag-based deployment detection (`--deploy-mode tags`).
- Multiple-branch support for monorepos with parallel release lines.
- Lead-time analyzer (its own design note when GitHub adapter lands).
- Rework analyzer (its own design note; line-level analysis).
- Compute statistical significance of trend changes (is this week's spike normal noise?).

## References

- `knowledge/product/kpis.md` (Camada 1 — DORA+)
- DORA *State of DevOps* reports
