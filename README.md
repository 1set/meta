# meta 🤿

The single source of truth for the **Star\*** ecosystem's shared CI/CD and
quality-gate tooling. Every `1set/*` and `starpkg/*` Go module consumes this
repo so the whole ecosystem stays uniform — one place to fix the pipeline, one
place that defines the standards.

## What's here

| Path | What it is |
|------|------------|
| [`.github/workflows/go-ci.yml`](.github/workflows/go-ci.yml) | the **reusable CI workflow** every module calls |
| [`doccov/`](doccov/) | **doc-coverage** gate — every script builtin is documented in the README |
| [`covcheck/`](covcheck/) | **test-coverage** gate — total coverage may not drop below a floor (ratchet) |
| [`footprint/`](footprint/) | **binary-footprint** gate + badge — the marginal binary size a module adds |
| [`revive.toml`](revive.toml) | the shared `revive` lint config (incl. the `exported` GoDoc rule) |
| `selftest/`, `.github/workflows/selftest.yml` | proves `go-ci.yml` + the tools end-to-end inside meta |

`doccov`, `covcheck`, and `footprint` are three `main` command packages under the
**single module `github.com/1set/meta`** (one root `go.mod`, go 1.19,
**standard-library only — no go.sum**). Each builds and runs independently:
`go run github.com/1set/meta/<tool>@<ref>` compiles just that command. (The
`selftest/` fixture is a separate nested module on purpose — it has its own
deps.)

## Reusable CI workflow

A consuming repo's `.github/workflows/build.yml` is a thin caller:

```yaml
name: Build
on:
  push: { branches: ["master"] }
  pull_request: { branches: ["master"] }
permissions: read-all
jobs:
  ci:
    uses: 1set/meta/.github/workflows/go-ci.yml@<pinned-sha>
    with:
      go-floor: "1.19"          # this repo's go.mod floor minor version
      # doc-coverage: true      # opt in to the doccov gate
      # cov-min: 78             # test-coverage ratchet floor (0 = no gate)
      # footprint: true         # measure & badge the binary footprint
      # footprint-max-mb: 7     # fail if the stripped footprint delta exceeds this
    secrets: inherit
```

It runs the matrix `[<go-floor>.x, 1.25.x] × {ubuntu-22.04, macos-14,
windows-2022}`. Static checks (vet, gofmt, `go mod tidy`, govulncheck, doccov)
run once on the latest-Go Linux leg; coverage upload + the covcheck/footprint
gates run on the floor Linux leg.

### Inputs

| Input | Type | Default | Meaning |
|-------|------|---------|---------|
| `go-floor` | string | (required) | the repo's go.mod floor minor, e.g. `"1.19"` / `"1.22"` |
| `working-directory` | string | `"."` | path to the Go module within the repo |
| `doc-coverage` | bool | `false` | run the **doccov** README↔builtin gate |
| `cov-min` | number | `0` | **covcheck** test-coverage floor (`0` = no gate) |
| `footprint` | bool | `false` | run **footprint** (badge + bloat gate) |
| `footprint-max-mb` | number | `0` | fail if the stripped footprint delta exceeds this many MB |
| `doc-coverage-file` | string | `README.md` | doc file doccov checks (e.g. `docs/API.md` for the split layout) |
| `doc-coverage-config` | bool | `false` | also gate the `base`-generated `get_`/`set_` config accessors |

Pin the `@<sha>` for supply-chain safety; bump it when this workflow changes.

## Gate tools

Each tool also runs standalone for local checks (see its README):

```bash
go run github.com/1set/meta/doccov@master .                                            # docs ↔ builtins
go run github.com/1set/meta/covcheck@master -min 70 coverage.txt                        # test coverage floor
go run github.com/1set/meta/footprint@master -modpath github.com/starpkg/sqlite -dir .  # binary size
```

- **doccov** ([README](doccov/README.md)) — AST-scans every `starlark.NewBuiltin(...)` and fails if it isn't documented (backtick) in the README.
- **covcheck** ([README](covcheck/README.md)) — parses the `make ci` coverage profile and fails below `-min`. Policy is a **ratchet**: each repo's floor sits just below its current coverage, so it can only hold or improve.
- **footprint** ([README](footprint/README.md)) — builds a bare starlet host vs the host + module and reports the marginal binary size (default + stripped); emits a shields.io badge and gates against silent bloat. Numbers are platform-specific — use the CI (linux/amd64) value.

GoDoc completeness (a doc comment on every exported symbol) is enforced by
`revive`'s `exported` rule from `revive.toml`, in the workflow's Analyze step.

## Self-test

`selftest.yml` calls `go-ci.yml` against the `selftest/` fixture (so any change
to the workflow is proven before consumers re-pin) and runs `go test ./...` over
the gate tools. Note the **chicken-and-egg**: a new tool isn't on `master` until
its PR merges, so don't enable its `go-ci.yml` input in the fixture in the same
PR — keep new tool inputs opt-in (default off) and let consumers turn them on
after the merge.
