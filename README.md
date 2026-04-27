# epo-core

Open-source CLI and core libraries of **EPO** — a platform that orchestrates the engineering toolchain.

> EPO is an integration platform, not yet another linter or reviewer. It builds a living graph of your environment and makes the tools you already have (CodeRabbit, SonarQube, Backstage, Cursor, Claude Code, etc.) work better together.

This repository (`epo-core`) is the public, Apache 2.0 portion of the project: the CLI binary, the catalog schema, the adapters, and the SDK. Cloud-specific features live in a separate, private repository.

## Status

**Pre-alpha.** First feature in development: `epo baseline scan` — an analysis tool that produces a baseline report (bus factor, knowledge silos, hotspots, basic DORA) of one or more repositories. It is the first dogfooding target against EPO's cliente-zero.

Expect rough edges, breaking changes between minor versions, and an evolving public API until v1.0.0.

## Install

Not yet published. Build from source:

```bash
git clone https://github.com/dreiboxco/epo-core
cd epo-core
make build
./bin/epo --help
```

Requires Go 1.24+.

## Quick start

Once the first command lands:

```bash
epo baseline scan --path /path/to/repo --metric busfactor --out baseline.md
```

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

The project is in its earliest phase. Issues and discussions are welcome; PRs may be deferred until the architecture stabilizes (target: v0.1.0).
