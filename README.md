# epo-core

Open-source CLI and core libraries of **EPO** — a platform that orchestrates the engineering toolchain.

> EPO is an integration platform, not yet another linter or reviewer. It builds a living graph of your environment and makes the tools you already have (CodeRabbit, SonarQube, Backstage, Cursor, Claude Code, etc.) work better together.

This repository (`epo-core`) is the public, Apache 2.0 portion of the project: the CLI binary, the catalog schema, the adapters, and the SDK. Cloud-specific features live in a separate, private repository.

## Status

**v0.1.0 — first public release** (2026-04-29). Surface: a single command — `epo baseline scan` — with six metrics validated in a 132-scan dogfooding round on the cliente-zero (22 repositories, 0 failures). See [CHANGELOG.md](./CHANGELOG.md) for details.

Expect breaking changes between `0.x` minor releases until the public API stabilizes at `1.0.0`.

## Install

Not yet published to package managers. Build from source:

```bash
git clone https://github.com/dreiboxco/epo-core
cd epo-core
make build
./bin/epo --help
```

Requires Go 1.24+.

## Quick start

Scan a repository's bus factor against its integration branch (auto-detected):

```bash
epo baseline scan \
  --path /path/to/repo \
  --metric busfactor \
  --ref auto \
  --out baseline.md
```

Run all six metrics in a loop:

```bash
for m in busfactor hotspots silos age coupling dora; do
  epo baseline scan --path . --metric $m --ref auto \
    --out reports/$m.md --repo-label my-repo
done
```

### Available metrics

| Metric | What it measures |
|--------|------------------|
| `busfactor` | Per-file authorship concentration. Surfaces single-points-of-failure (BF=1). |
| `hotspots` | `churn × bug-fix density`. Files where bugs nest. |
| `silos` | `churn / unique contributors`. "Active and lonely" files. |
| `age` | Distribution of last-touched timestamps. Pair with `--since 0` for full history. |
| `coupling` | Files that change together in the same commit (intra-repo). |
| `dora` | Phase-1 DORA: deploy frequency + change failure rate (heuristic). |

### Useful flags

| Flag | Purpose |
|------|---------|
| `--ref auto` | Resolve integration branch from `origin/HEAD` (fallback `main → master → trunk → develop → development`). |
| `--ref <name>` | Scan a specific branch / tag / SHA explicitly. |
| `--exclude-author <regex>` | Drop bot commits. Default `\[bot\]` catches dependabot, renovate, github-actions. |
| `--ignore <prefix>` | Skip path prefixes. Default ignores `vendor/`, `node_modules/`, `third_party/`, `CHANGELOG.md`. |
| `--since <expr>` | Time window. Accepts `12 months`, `6 weeks`, `90 days`, `YYYY-MM-DD`, or `0` (full history). |
| `--out <file>` | Write markdown report to a file (otherwise stdout). |
| `--top <N>` | Limit to top-N riskiest files in detail tables. `0` = all. |
| `--bucket daily|weekly|monthly` | Time-binning resolution for `dora` (default weekly). |
| `--bug-pattern <regex>` | Override the heuristic that classifies bug-fix commits (`hotspots`, `dora`). |

Run `epo baseline scan --help` for the complete reference.

## Project layout

```
cmd/epo/          CLI entrypoint
internal/         Implementation (not part of the stable API)
  baseline/         Baseline measurement commands
  cli/              Cobra command wiring
pkg/              Public Go API (empty for now)
docs/design/      Per-feature design notes and metric specs
testdata/         Synthetic repositories used in tests
```

## License

Apache License 2.0. See [LICENSE](./LICENSE).

## Contributing

The project is still early. Issues and discussions are welcome; PRs may be deferred until the architecture stabilizes (target: v1.0.0). For substantial changes, open an issue first to align on direction.
