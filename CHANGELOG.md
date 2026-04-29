# Changelog

All notable changes to `epo-core` are documented here.
Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While the project is on the `0.x` line, **breaking changes can happen in any minor release** (per SemVer 4.) until the public API stabilizes at `1.0.0`.

## [Unreleased]

## [0.1.0] — 2026-04-29

First public release. Locks down the surface of `epo baseline scan` after a full
dogfooding round on 22 repositories of the cliente-zero (Embarca / ArcaSolucoes):
22 × 6 metrics = 132 successful scans, 0 failures.

### Added

- `epo baseline scan` command with six metrics, all rendered as markdown to `--out` or stdout:
  - **`busfactor`** — per-file authorship concentration. Surfaces files where ≥ 50% of changes come from a single author (BF=1) and aggregates by component path.
  - **`hotspots`** — `churn × bug-fix density` per file. Bug commits identified by a configurable regex (`--bug-pattern`); the report includes a sample audit so the heuristic stays honest.
  - **`silos`** — `churn / unique contributors`. Highlights "active and lonely" files that complement bus factor and hotspots.
  - **`age`** — distribution of last-touched timestamps with configurable buckets (default `<30d / 30d-90d / 90d-365d / >365d`).
  - **`coupling`** — pairs of files that change together within a single commit, with min-support / min-confidence / catch-all filters.
  - **`dora`** — Phase-1 DORA: deployment frequency proxy (commits in HEAD per bucket) + change failure rate (fixes / total). Bucket configurable: daily, weekly, monthly.
- `--ref <branch|tag|sha>` to scan an arbitrary revision instead of the working `HEAD`.
- `--ref auto` to resolve the integration branch automatically. Tries the symbolic ref `refs/remotes/origin/HEAD` first; falls back to a heuristic in the order `main → master → trunk → develop → development`.
- `--exclude-author <regex>` (repeatable) to drop commits whose author name **or** email matches a pattern. Default: `\[bot\]` — catches GitHub bot accounts (`dependabot[bot]`, `renovate[bot]`, `github-actions[bot]`). Pass `--exclude-author=''` to disable.
- `--ignore <prefix>` (repeatable) to skip path prefixes during analysis. Default list: `vendor/`, `node_modules/`, `third_party/`, and `CHANGELOG.md` (catch-all without signal).
- `--since` accepts both relative offsets (`12 months`, `6 weeks`, `90 days`) and absolute dates (`YYYY-MM-DD`); `0` disables the time filter.
- Per-feature design notes under `docs/design/`:
  `0001-bus-factor.md`, `0002-hotspots.md`, `0003-knowledge-silos.md`,
  `0004-code-age.md`, `0005-temporal-coupling.md`, `0006-dora-metrics.md`.

### Notes

- All metrics read history from a local clone via go-git — no GitHub API dependency.
- Author identity uses email when present (more stable than display name); identity reconciliation across multiple emails is left to a future release.
- Renames are reported as a deletion plus an addition; merging into a single rename is left to a future release.
- DORA `change failure rate` is a heuristic over commit messages, **not** the rigorous DORA definition (which requires deploy outcome data). The report makes this explicit and includes a classification audit sample.
- Coupling is intra-repo only in this release; cross-repo coupling depends on a catalog graph that lives in a later phase.

[Unreleased]: https://github.com/dreiboxco/epo-core/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/dreiboxco/epo-core/releases/tag/v0.1.0
