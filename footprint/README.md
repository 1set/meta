# footprint

The binary-size gate & badge for the Star\* ecosystem.

footprint measures the **marginal binary size a module adds**. It builds two
tiny programs against the module: a BASELINE that only spins up a starlet host,
and a WITH program that additionally constructs the module
(`NewModule().LoadModule()`), forcing the module and its transitive SDKs to
link. The footprint is `with − baseline`, for both the default and the stripped
(`-ldflags="-s -w"`) build.

## Usage

```bash
go run github.com/1set/meta/footprint@master -modpath github.com/starpkg/sqlite -dir .
# footprint github.com/starpkg/sqlite
#   default : baseline 13.0 MB  with 20.2 MB  delta +7.2 MB (+55%)
#   stripped: baseline  9.0 MB  with 13.7 MB  delta +4.8 MB (+53%)
```

| Flag | Default | Meaning |
|------|---------|---------|
| `-modpath` | (required) | the module's import path, e.g. `github.com/starpkg/sqlite` |
| `-dir` | `.` | the local module directory |
| `-json` | off | emit a shields.io endpoint JSON (for the README badge) |
| `-max-mb` | `0` | fail if the stripped delta exceeds this many MB (`0` = report only) |

Run it on the repo's go floor for comparable numbers. Exit status is non-zero on
a build failure or when `-max-mb` is exceeded.

## In CI

The reusable workflow runs this on the floor leg when a repo opts in:

```yaml
with:
  footprint: true
  footprint-max-mb: 7   # current stripped delta + a small headroom
```

This both surfaces the footprint and **fails if a dependency silently bloats the
binary past the ceiling**. The `-json` output feeds a shields.io endpoint badge
in the README so the size cost is visible on the front page.
