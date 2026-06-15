// Tests for the doccov gate, grouped by goal:
//   - surface extraction (scanSurface, stringLit, shortName): which builtins are found
//   - README coverage (backtickWords): which names count as documented
//   - end-to-end (run): pass / fail / no-builtins / ignore behaviour
package main

import (
	"go/parser"
	"os"
	"path/filepath"
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

func TestBacktickWordsAcrossFences(t *testing.T) {
	// Regression: a `code` reference in a table AFTER a ```fenced``` block must
	// still be detected (the fenced block must not corrupt inline-span pairing).
	doc := "Intro `inline_one`.\n\n```go\ncode := example()\n```\n\n| opt | accessors |\n|---|---|\n| width | `get_width` / `set_width` |\n"
	words := backtickWords(doc)
	for _, w := range []string{"inline_one", "example", "get_width", "set_width"} {
		if !words[w] {
			t.Errorf("expected %q documented across the fence, got %v", w, words)
		}
	}
}

func TestRunGood(t *testing.T) {
	if err := run("testdata/good", "README.md", nil, false); err != nil {
		t.Fatalf("good fixture should pass, got: %v", err)
	}
}

func TestRunBad(t *testing.T) {
	err := run("testdata/bad", "README.md", nil, false)
	if err == nil {
		t.Fatal("bad fixture should fail: gamma is undocumented")
	}
	if !strings.Contains(err.Error(), "gamma") {
		t.Fatalf("error should name the undocumented builtin, got: %v", err)
	}
}

func TestRunBadIgnored(t *testing.T) {
	if err := run("testdata/bad", "README.md", map[string]bool{"gamma": true}, false); err != nil {
		t.Fatalf("ignoring gamma should pass, got: %v", err)
	}
}

func TestRunNoBuiltins(t *testing.T) {
	if err := run("testdata/empty", "README.md", nil, false); err != nil {
		t.Fatalf("a repo with no builtins must not fail, got: %v", err)
	}
}

func TestScanConfig(t *testing.T) {
	got, err := scanConfig("testdata/config")
	if err != nil {
		t.Fatalf("scanConfig: %v", err)
	}
	// width non-secret -> get_+set_ ; token secret -> set_ only
	want := "get_width,set_token,set_width"
	if strings.Join(got, ",") != want {
		t.Fatalf("accessors = %v, want %s", got, want)
	}
}

func TestRunConfigGate(t *testing.T) {
	dir := t.TempDir()
	// copy the config fixture's go file + a README documenting only some accessors
	src, _ := os.ReadFile("testdata/config/mod.go")
	if err := os.WriteFile(filepath.Join(dir, "mod.go"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	// README documents get_width/set_width but NOT set_token
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# c\n`get_width` `set_width`\n"), 0o644)
	if err := run(dir, "README.md", nil, false); err != nil {
		t.Fatalf("without -config, missing accessors must not fail: %v", err)
	}
	err := run(dir, "README.md", nil, true)
	if err == nil || !strings.Contains(err.Error(), "set_token") {
		t.Fatalf("with -config, undocumented set_token must fail, got: %v", err)
	}
}

func TestRunMissingReadme(t *testing.T) {
	if err := run("testdata/good", "NOPE.md", nil, false); err == nil {
		t.Fatal("a missing documentation file should fail")
	}
}
