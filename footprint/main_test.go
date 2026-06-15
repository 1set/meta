// Tests for footprint's pure helpers. The build-based measurement itself needs
// a module + toolchain + network and is exercised in CI, not here.
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadGoDirective(t *testing.T) {
	dir := t.TempDir()
	gomod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(gomod, []byte("module example.com/x\n\ngo 1.23\n\nrequire foo v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readGoDirective(gomod); got != "1.23" {
		t.Fatalf("readGoDirective = %q, want 1.23", got)
	}
	// Missing file falls back to the floor.
	if got := readGoDirective(filepath.Join(dir, "nope.mod")); got != "1.19" {
		t.Fatalf("readGoDirective(missing) = %q, want 1.19", got)
	}
}

func TestMBAndPct(t *testing.T) {
	if mb(1024*1024) != 1.0 {
		t.Fatalf("mb(1MiB) = %v, want 1.0", mb(1024*1024))
	}
	if got := pct(50, 100); got != 50.0 {
		t.Fatalf("pct(50,100) = %v, want 50.0", got)
	}
	if got := pct(5, 0); got != 0 {
		t.Fatalf("pct(_,0) = %v, want 0 (no divide-by-zero)", got)
	}
}
