# Design Note 0001 — Bus Factor

> Per-feature design note. Not an ADR (those are strategic and live in the private repo).
> This is the implementation spec for the first baseline metric.

## Goal

Given a local git repository, compute the **bus factor** of each tracked file and aggregate the result by directory and by component (a directory rooted at a configurable depth or matching a pattern).

The bus factor of a file/module is the smallest number of unique authors whose combined contributions account for more than 50% of the changes to that file in the last `N` months (default: 12).

## Non-goals (for v0)

- No GitHub API integration. Local git history only.
- No multi-repository aggregation. One repo per invocation.
- No identity reconciliation (alice@home.com and alice@work.com are two authors). Identity merging is a future feature.
- No exclusion of merge commits beyond what `--no-merges` provides natively.
- No weighting by lines-changed initially — every commit counts as 1. (See "Future improvements".)

## Inputs

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Path to the local git working tree |
| `--since` | `12 months` | Time window for commit history |
| `--threshold` | `0.5` | Coverage threshold (50%) |
| `--component-depth` | `2` | How many path segments to take when aggregating ("internal/baseline/busfactor" with depth 2 → "internal/baseline") |
| `--ignore` | (none) | Comma-separated globs to skip (vendor, node_modules, generated/) |
| `--top` | `20` | Show only the top N riskiest entries in the report |

## Algorithm

1. Open repo at `--path` using go-git.
2. Walk commits on `HEAD` since `--since`, excluding merges.
3. For each commit:
   - For each file changed, increment `commits[file][author]`.
4. For each file:
   - Sort authors by commit count desc.
   - Take the smallest prefix whose cumulative share >= `--threshold`. The size of that prefix is the file's bus factor.
5. Aggregate by component (path-prefix to depth `--component-depth`):
   - A component's bus factor is the **min** across its files (worst case in the component).
   - Also track median and distribution.
6. Render markdown:
   - Top-N files with bus factor 1 (single point of failure).
   - Per-component summary table.
   - Histogram of bus factors across the repo.

## Choice of git access

Use `github.com/go-git/go-git/v6` (pure Go). Reasons:
- No dependency on the `git` binary on the host.
- Easier to test with synthetic in-memory repositories.
- Slower on huge repos than shell-out — acceptable for v0.

If performance becomes a real problem on the Embarca repos, we add a `--use-git-binary` fallback that shells out to `git log --name-only --pretty='%ae'` and parses the stream. Decision deferred.

## Output format (markdown)

A self-contained markdown document, ready to be appended to or referenced by `knowledge/customers/<client>/baseline.md`:

```markdown
# Bus Factor Report — <repo name>

Generated: <ISO date>
Window: last 12 months
Threshold: 50%

## Summary

| Metric | Value |
|---|---|
| Files analyzed | 1,243 |
| Files with bus factor 1 | 287 (23%) |
| Median file bus factor | 2 |
| Components analyzed | 41 |
| Components with bus factor 1 | 11 |

## Top 20 single-point-of-failure files

| File | Author | Commits | Last touched |
|---|---|---|---|

## Components by risk

| Component | Files | Min bus factor | Median bus factor |
|---|---|---|---|

## Bus factor distribution

(ASCII histogram or compact table)
```

The renderer should also accept a `--format json` flag in v1, but v0 is markdown-only.

## Testing strategy

- **Unit**: pure functions over a `[]commit` slice (synthesizable in tests). No git dependency.
- **Integration**: a small synthetic repo built in `testdata/` with a predictable history (3 authors, ~10 commits) where the expected bus factor of each file is hand-computed. Run go-git over it and assert.
- **Smoke**: against `epo-core` itself once the repo has enough history. Fast feedback loop.

## Future improvements (out of scope for v0)

- Weighted by lines changed (`--weight=loc`).
- Identity reconciliation via `.mailmap` and a configurable email-to-person map.
- Time-decay weighting (recent commits count more).
- `--exclude-renames` to avoid bumping bus factor due to a `git mv`.
- JSON output and feed into the catalog graph (Fase 2 territory).

## Open questions

- Should the report include a "blast radius preview" if a bus-factor-1 author leaves? (Probably yes when the catalog exists; for v0, the bare bus factor is enough.)
- How to surface "the dev who knows this is no longer active in the company"? Requires a directory of current employees. Out of scope for v0.

## References

- `knowledge/product/kpis.md` — strategic KPI definitions
- Adam Tornhill, *Your Code as a Crime Scene* (2015) — original framing of code-as-knowledge metrics
- [code-maat](https://github.com/adamtornhill/code-maat) — reference implementation in Clojure
