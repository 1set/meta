# doccov

The documentation-consistency gate for the Star\* ecosystem.

A Star\* module exposes its script-facing API as Starlark builtins, each built
with `starlark.NewBuiltin("<name>", fn)`. `doccov` statically scans a module's Go
source for those builtins and **fails if any of them is not documented in the
module's README** — so the docs can never silently drift behind the code.

## Usage

```bash
# from a module directory
go run github.com/1set/meta/doccov@master .

# elsewhere / a specific path
go run github.com/1set/meta/doccov@master path/to/module
```

Flags:

| Flag | Default | Meaning |
|------|---------|---------|
| `-readme` | `README.md` | documentation file to check (e.g. `docs/API.md` for the split layout) |
| `-ignore` | (empty) | comma-separated names to exclude (deprecated/internal-but-registered) |
| `-config` | off | also require the `base`-generated config accessors (`get_`/`set_<key>`) to be documented |

## How it decides

- **Surface** = the first string argument of every `starlark.NewBuiltin(...)` call
  in the non-test (`*.go`, excluding `*_test.go`) files at the top of the directory.
  A qualified name `"module.fn"` (and the `ModuleName + ".fn"` form) is reduced to `fn`.
- **Documented** = the name appears as a word inside a backtick span of the README.
- **Config accessors** (`-config`) = a separate group. `base` auto-generates a
  `get_<key>`/`set_<key>` pair for every config option (secret options get only
  `set_`). doccov derives them from the `configKey<X> = "<name>"` constants and
  the `gen[Secret]ConfigOption(configKey<X>, …)` declarations (a declaration line
  containing "Secret" — `genSecretConfigOption` or a chained `.SetSecret(true)` —
  marks the option secret). Reported and gated as its own group, so the docs can
  keep a separate *Configuration* section.
- It checks for **omission**, not accuracy — a wrong description is a review concern.
- Exit status is non-zero on an undocumented builtin or a missing README; it is
  **zero when no `starlark.NewBuiltin` calls are found** (the repo does not opt in).

## In CI

The reusable workflow `1set/meta/.github/workflows/go-ci.yml` runs this as an
opt-in gate. A starpkg domain module enables it in its caller:

```yaml
jobs:
  ci:
    uses: 1set/meta/.github/workflows/go-ci.yml@<pin>
    with:
      go-floor: "1.20"
      doc-coverage: true              # turn on the doccov gate
      # doc-coverage-file: docs/API.md  # split layout: check the API reference
      # doc-coverage-config: true       # also gate the get_/set_ config accessors
    secrets: inherit
```

GoDoc completeness (a doc comment on every exported symbol) is a separate concern,
covered by `revive`'s `exported` rule in the same workflow's Analyze step.
