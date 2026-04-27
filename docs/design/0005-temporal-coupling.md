# Design Note 0005 — Temporal Coupling

## Goal

Detect pairs of files that consistently change together. If every PR that touches `orders_controller.rb` also touches `payments_service.rb`, they are temporally coupled. The coupling is the empirical signature of an architectural dependency the code may or may not declare.

For v0, the analyzer reports intra-repo coupling only. Cross-repo coupling — same idea, but pairing PRs across repositories by author and timestamp — is Phase 2 work because it requires the catalog graph.

## Why this matters

Per `kpis.md`: *"Acoplamento temporal sugere acoplamento lógico não declarado. Se todo PR na sales-api exige mudança na whitelabel, isso é uma dependência invisível."* The intra-repo version is the cheaper, faster build of the same idea. It surfaces:

- Hidden module dependencies (one struct's lifecycle dragged behind another's)
- Implicit contracts between layers (`controllers/X.rb` ↔ `services/Y.rb`)
- Cohesion debt (files that should arguably be one module)

It also produces noise we have to filter — catch-all files like `CHANGELOG.md`, `db/schema.rb`, or `config/routes.rb` couple with everything, which is uninformative.

## Non-goals (for v0)

- No cross-repo coupling (Phase 2; needs the catalog/graph).
- No PR-level grouping; v0 looks at commits one by one.
- No directional inference ("changing A always precedes changing B" — interesting but expensive).
- No transitive coupling (if A↔B and B↔C, we don't infer A↔C; we just report pairwise).

## Inputs

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `.` | Local git working tree |
| `--since` | `12 months` | Time window |
| `--min-support` | `5` | Minimum co-occurrence count for a pair to appear |
| `--min-confidence` | `0.5` | Minimum average bidirectional confidence |
| `--max-files-per-commit` | `50` | Skip commits touching more files than this (refactors, renames) |
| `--max-commits-per-file` | `200` | Skip files appearing in more commits than this (catch-all files) |
| `--component-depth` | `2` | Path-prefix depth for component-pair aggregation |
| `--ignore` | (defaults) | Path prefixes to skip |
| `--top` | `30` | Top N pairs to show |

## Algorithm

1. Load commits via `git.Load`.
2. **Phase 1 — file frequencies**: count `commitsTouching[f]` for every file.
3. **Phase 2 — pair counts**: for each commit:
   - If `len(files) > maxFilesPerCommit`, skip it.
   - For each pair `(a, b)` with `a < b` lexicographically:
     - Skip if `commitsTouching[a] > maxCommitsPerFile` OR same for `b`.
     - Increment `coOccur[(a, b)]`.
4. For each pair `(a, b)`:
   - `support = coOccur[(a,b)]`
   - `confidence_a_to_b = support / commitsTouching[a]`
   - `confidence_b_to_a = support / commitsTouching[b]`
   - `confidence_avg = (confidence_a_to_b + confidence_b_to_a) / 2`
   - `confidence_min = min(confidence_a_to_b, confidence_b_to_a)`
5. Filter by `support >= minSupport AND confidence_avg >= minConfidence`.
6. Sort by `confidence_avg` desc, then `support` desc.
7. Aggregate per component-pair: sum of pair supports under each `(comp_a, comp_b)` cell.

## Why filter catch-all files

Files like `CHANGELOG.md`, `db/schema.rb`, `config/routes.rb`, `package-lock.json` change in nearly every PR. They form pairs with everything. The signal is genuine — they really do change together — but it's noise for finding architectural coupling.

The `--max-commits-per-file` flag is the cheap pre-filter. A more sophisticated approach (lift normalization, IDF-style weighting) is deferred until we hit a real case where `--max-commits-per-file` is insufficient.

## Output format (markdown)

```markdown
# Temporal Coupling Report — <repo>

Window: <range>
Min support: 5  Min confidence: 0.5
Excluded: 12 files with >200 commits, 8 mass-edit commits

## Summary

| Metric | Value |
|---|---|
| Commits analyzed | M |
| File pairs evaluated | P |
| Coupled pairs surfaced | K |

## Top 30 coupled pairs

| File A | File B | Co-occur | Conf A→B | Conf B→A | Conf avg |

## Component-pair pressure

| Component A | Component B | Pair count | Total co-occur |

## Excluded (catch-all) files

(reference list so reviewers know what was omitted)
```

The "excluded" section is the auditability lever, same as hotspots' classification audit.

## Testing strategy

- **Unit**: pure functions with hand-built commit slices. Verify confidence math, threshold filtering, catch-all exclusion, pair ordering invariants (a < b), component-pair aggregation.
- **Smoke**: epo-core (very few commits, expect almost no surfaced pairs — validates plumbing). Embarca repos for real signal.

## Future improvements (out of scope for v0)

- PR grouping (collapse merge → squashed commit).
- Lift / TF-IDF style normalization to avoid `--max-commits-per-file` heuristic.
- Directional analysis (A always changes before B).
- Cross-repo coupling via shared author + timestamp window.
- Visualization-friendly output (graph format) for downstream tooling.

## Open questions

- Should `min_confidence` apply to `confidence_avg` or `confidence_min`? `_avg` is more permissive and more likely to surface asymmetric pairs ("changing A always touches B, but B is touched standalone often"). Going with `_avg` for v0; revisit if it produces too much noise.
- Should we report all pairs in a giant CSV alongside the markdown summary? Yes eventually; the catalog ingestion will want it. v0 stays markdown-only for the report.

## References

- `knowledge/product/kpis.md` (Temporal Coupling row)
- Adam Tornhill, *Your Code as a Crime Scene*
- [code-maat](https://github.com/adamtornhill/code-maat) — the same idea, in Clojure, decades of prior art
