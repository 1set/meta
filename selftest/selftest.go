// Package selftest is a minimal Go module that meta's selftest workflow runs
// through the reusable go-ci workflow (.github/workflows/go-ci.yml) to validate
// it end to end — build, vet, gofmt, go mod tidy, make ci, govulncheck, and the
// revive/tokei analyze step. It is a CI fixture only: not published, not imported.
package selftest

import "strings"

// Greet returns a friendly, trimmed greeting for name; empty name greets "world".
func Greet(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "world"
	}
	return "hello, " + name
}
