// Tests for the covcheck gate: parsing a coverage profile into a total percent,
// and the missing/empty-profile error paths.
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProfile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "coverage.txt")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTotalCoverage(t *testing.T) {
	// 3 + 2 = 5 statements; 3 covered (count>0) => 60.0%
	p := writeProfile(t, "mode: atomic\n"+
		"pkg/a.go:1.1,2.10 3 1\n"+
		"pkg/a.go:3.1,4.10 2 0\n")
	got, err := totalCoverage(p)
	if err != nil {
		t.Fatalf("totalCoverage: %v", err)
	}
	if got < 59.9 || got > 60.1 {
		t.Fatalf("coverage = %.2f, want 60.0", got)
	}
}

func TestTotalCoverageAllCovered(t *testing.T) {
	p := writeProfile(t, "mode: set\npkg/a.go:1.1,2.10 4 1\n")
	got, err := totalCoverage(p)
	if err != nil || got < 99.9 {
		t.Fatalf("coverage = %.2f, err %v, want 100.0", got, err)
	}
}

func TestTotalCoverageMissingFile(t *testing.T) {
	if _, err := totalCoverage(filepath.Join(t.TempDir(), "nope.txt")); err == nil {
		t.Fatal("missing profile should error")
	}
}

func TestTotalCoverageEmpty(t *testing.T) {
	p := writeProfile(t, "mode: atomic\n")
	if _, err := totalCoverage(p); err == nil {
		t.Fatal("a profile with no statements should error")
	}
}
