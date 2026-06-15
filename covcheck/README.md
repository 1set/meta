# covcheck

The test-coverage gate for the Star\* ecosystem.

`make ci` writes a Go coverage profile (`go test -coverprofile=coverage.txt`).
covcheck parses it, computes the **total statement coverage**, and **fails when
it is below a required minimum** — a reliable, self-contained merge gate that
does not depend on an external coverage service posting a commit status.

## Usage

```bash
go run github.com/1set/meta/covcheck@master -min 70 coverage.txt
```

| Flag | Default | Meaning |
|------|---------|---------|
| `-min` | `0` | minimum total coverage percent required (`0` = report only) |

The total is computed the same way as `go tool cover -func` (covered
statements ÷ total statements), using only the standard library. Exit status is
non-zero when coverage is below `-min` or the profile is missing/empty.

## In CI

The reusable workflow `1set/meta/.github/workflows/go-ci.yml` runs this on the
coverage (floor) leg when a repo sets a **ratchet floor**:

```yaml
with:
  cov-min: 78   # this repo's current coverage minus a small margin
```

The ecosystem policy is a **ratchet**: each repo's `cov-min` is set just below
its current coverage, so coverage can only hold or improve, never regress.
Codecov upload stays on for the dashboard and the README coverage badge.
