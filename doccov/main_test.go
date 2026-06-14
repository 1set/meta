// Tests for the doccov gate, grouped by goal:
//   - surface extraction (scanSurface, stringLit, shortName): which builtins are found
//   - README coverage (backtickWords): which names count as documented
//   - end-to-end (run): pass / fail / no-builtins / ignore behaviour
package main

import (
	"go/parser"
	"strings"
	"testing"
)

func TestScanSurface(t *testing.T) {
	got, err := scanSurface("testdata/good")
	if err != nil {
		t.Fatalf("scanSurface: %v", err)
	}
	want := []string{"alpha", "beta"} // "good.alpha" -> "alpha"; "test_only" excluded (_test.go)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("surface = %v, want %v", got, want)
	}
}

func TestStringLitFromExpr(t *testing.T) {
	// stringLit must resolve a plain literal and the "ModuleName + \".fn\"" form.
	cases := map[string]string{
		`"connect"`:               "connect",
		`"sqlite.connect"`:        "sqlite.connect",
		`ModuleName + ".connect"`: ".connect",
	}
	for src, want := range cases {
		expr, err := parser.ParseExpr(src)
		if err != nil {
			t.Fatalf("ParseExpr(%q): %v", src, err)
		}
		if got := stringLit(expr); got != want {
			t.Errorf("stringLit(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestShortName(t *testing.T) {
	for in, want := range map[string]string{
		"connect":        "connect",
		"sqlite.connect": "connect",
		".connect":       "connect",
	} {
		if got := shortName(in); got != want {
			t.Errorf("shortName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBacktickWords(t *testing.T) {
	doc := "Use `alpha` and `db.beta(x)`; plain gamma is not in backticks."
	words := backtickWords(doc)
	if !words["alpha"] || !words["beta"] || !words["db"] {
		t.Errorf("expected alpha/beta/db in backticks, got %v", words)
	}
	if words["gamma"] {
		t.Errorf("gamma is outside backticks and must not count as documented")
	}
}

func TestRunGood(t *testing.T) {
	if err := run("testdata/good", "README.md", nil); err != nil {
		t.Fatalf("good fixture should pass, got: %v", err)
	}
}

func TestRunBad(t *testing.T) {
	err := run("testdata/bad", "README.md", nil)
	if err == nil {
		t.Fatal("bad fixture should fail: gamma is undocumented")
	}
	if !strings.Contains(err.Error(), "gamma") {
		t.Fatalf("error should name the undocumented builtin, got: %v", err)
	}
}

func TestRunBadIgnored(t *testing.T) {
	if err := run("testdata/bad", "README.md", map[string]bool{"gamma": true}); err != nil {
		t.Fatalf("ignoring gamma should pass, got: %v", err)
	}
}

func TestRunNoBuiltins(t *testing.T) {
	if err := run("testdata/empty", "README.md", nil); err != nil {
		t.Fatalf("a repo with no builtins must not fail, got: %v", err)
	}
}

func TestRunMissingReadme(t *testing.T) {
	if err := run("testdata/good", "NOPE.md", nil); err == nil {
		t.Fatal("a missing documentation file should fail")
	}
}
