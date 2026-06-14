package selftest

import (
	"testing"

	"github.com/pmezard/go-difflib/difflib"
)

func TestGreet(t *testing.T) {
	if got := Greet(" Star "); got != "hello, Star" {
		t.Fatalf("Greet(%q) = %q, want %q", " Star ", got, "hello, Star")
	}
	if got := Greet(""); got != "hello, world" {
		t.Fatalf("Greet(empty) = %q, want %q", got, "hello, world")
	}
	// Exercise an external dependency so the fixture has a real, stable go.sum
	// (a no-dependency module produces no go.sum, which breaks setup-go caching).
	if lines := difflib.SplitLines("a\nb"); len(lines) != 2 {
		t.Fatalf("difflib.SplitLines = %d lines, want 2", len(lines))
	}
}
