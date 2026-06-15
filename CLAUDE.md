# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`1set/meta` is the **shared CI/CD and quality-gate hub** for the entire Star\*
ecosystem. It is not a library anyone imports for runtime behavior — it is the
single source of truth every `1set/*` and `starpkg/*` module *consumes* so the
whole ecosystem stays uniform: one reusable CI workflow, one lint config, and a
trio of gate tools that enforce the three coverage standards.

## Repository layout

- **`.github/workflows/go-ci.yml`** — the reusable workflow (`on: workflow_call`).
  Every consuming repo's `build.yml` is a thin caller pinned to a SHA here.
- **`doccov/`**, **`covcheck/`**, **`footprint/`** — three `main` command
  packages, the gate tools.
- **`revive.toml`** — the shared `revive` lint config (fetched at CI time by the
  Analyze step; includes the `exported` GoDoc rule and the Lizard nloc limit).
- **`selftest/`** — a fixture *module* used to exercise `go-ci.yml`.
- **`.github/workflows/selftest.yml`** — runs `go-ci.yml` on the fixture + the
  tools' `go test`.

### Module layout (important — why there's one go.mod)

The root holds **one** module `github.com/1set/meta` (`go.mod`, go 1.19). The
three tools are command packages *under* it (`github.com/1set/meta/doccov`,
`/covcheck`, `/footprint`), each `package main`. This is deliberate:

- **All three are standard-library only** — zero third-party deps, so the root
  go.mod needs no `go.sum`, and there is nothing to isolate per tool.
- **They still build/run independently**: `go run github.com/1set/meta/<tool>@<ref>`
  (and `go build ./<tool>/`) compile only that command and its imports — the
  other two are not touched. One module ≠ one binary.
- **Per-tool go.mod would be pure overhead** — three go.mod/go.sum to keep in
  sync, and no `go test ./...` across them. The idiomatic Go layout for a
  multi-command repo is many command packages under one module.
- **`selftest/` is a separate nested module on purpose** — it is the *thing being
  tested* and carries its own deps (e.g. go-difflib for a non-empty go.sum); it
  must not pollute the tools' module.

## The three gate tools

All are tiny, stdlib-only, and run from CI as `go run …@master` (like
`govulncheck`), gated behind opt-in workflow inputs.

- **`doccov`** — AST-scans non-test `.go` for every `starlark.NewBuiltin(<arg>)`
  (resolving `"mod.fn"` and `ModuleName + ".fn"`), and fails if a builtin's name
  isn't a backtick word in the README. Checks **omission, not accuracy**.
- **`covcheck`** — parses a `go test` coverage profile (`coverage.txt`), computes
  total statement coverage (same math as `go tool cover -func`), and fails below
  `-min`. The ecosystem uses it as a **ratchet**: each repo's `cov-min` sits just
  below its current coverage, so coverage can only hold or rise.
- **`footprint`** — builds a baseline starlet host vs the host +
  `mod.NewModule().LoadModule()`, so the module + its SDKs link, and reports the
  marginal binary size (default + stripped). `-json` emits a shields.io badge;
  `-max-mb` gates against bloat.

GoDoc completeness is *not* a tool here — it is `revive`'s `exported` rule
(`revive.toml`), run in the Analyze step. Keep that division: doccov = README
surface, revive = GoDoc.

## go-ci.yml design (the part that matters when editing)

- Matrix: `[<go-floor>.x, 1.25.x] × {ubuntu-22.04, macos-14, windows-2022}`.
- Two env-flag legs: **`IS_CHECKS`** (latest Go + Linux) runs the once-only static
  checks (vet/gofmt/tidy/govulncheck/doccov); **`IS_REPORT`** (floor + Linux) runs
  `make ci` coverage upload + the covcheck/footprint gates (floor for comparable
  footprint numbers).
- New gates are added as **opt-in inputs** (default off) so existing callers and
  the core libraries are unaffected until they opt in.

## Conventions (non-obvious, easy to get wrong)

- **Self-test before consumers re-pin.** Any change to `go-ci.yml` or a tool must
  keep `selftest.yml` green. Consumers pin a SHA; they only get a change after
  re-pinning.
- **The `@master` chicken-and-egg.** The workflow runs the tools via
  `go run …@master`. A *new* tool isn't on `master` until its PR merges, so its
  go-ci.yml step must be gated on an input the `selftest/` fixture does **not**
  set — otherwise the self-test PR runs `@master` for a tool that doesn't exist
  yet and fails. Add the tool + the (default-off) input together; let real
  consumers enable it after merge.
- **Footprint is platform-specific.** linux/amd64 (CI) differs from local
  mac/arm64 — gum was 16.5 MB locally, 23.8 MB in CI. Ceilings and badges must use
  the **CI value** (read from the PR's footprint step log), or the gate misfires.
- **Coverage is measured without the private suite.** CI has no `starpkg/test`
  integration scripts, so `make ci` coverage = unit tests only; set ratchet
  floors from that (locally, measure with `starpkg/test` moved aside).
- **Third-party GitHub Actions are pinned to a full commit SHA** (Codacy's
  0-new-issue gate flags unpinned actions); tools like revive/tokei are pinned to
  a release tag.

## Dev commands

```bash
go test -race ./...                 # the three tools (selftest fixture is a separate module)
go vet ./... && gofmt -l .          # must be clean
go run ./doccov <module-dir>        # try a tool locally
```

CI/commit style follows the 1set repos (`[ci]`/`[feat]`/`[chore]`/`[doc]`
prefixes). Changes here ripple to every consumer — prefer additive, opt-in
changes and keep the self-test green.
