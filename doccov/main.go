// Command doccov is the documentation-consistency gate for the Star* ecosystem.
//
// A Star* module exposes its script-facing API as Starlark builtins, each
// constructed with starlark.NewBuiltin("<name>", fn). doccov statically scans a
// module's Go source for those builtins and fails if any of them is not
// documented in the module's README — so the docs can never silently drift
// behind the code.
//
// Usage:
//
//	doccov [flags] [dir]            # dir defaults to the current directory
//	go run github.com/1set/meta/doccov@<ref> .
//
// Flags:
//
//	-readme <file>   documentation file to check (default "README.md")
//	-ignore a,b,c    builtin names to exclude (deprecated/internal-but-registered)
//
// It scans only non-test *.go files in dir (top level), so test-only builtins do
// not count as public surface. A builtin name of the form "module.fn" is reduced
// to "fn" before the README is checked. A symbol counts as documented when it
// appears as a word inside any backtick span in the README; doccov guards against
// omission, not against an inaccurate description (that is a review concern).
//
// Exit status is non-zero when a builtin is undocumented or the README is
// missing; it is zero when no starlark.NewBuiltin calls are found (the repo does
// not opt into this convention).
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func main() {
	readme := flag.String("readme", "README.md", "documentation file to check")
	ignore := flag.String("ignore", "", "comma-separated builtin names to exclude")
	checkConfig := flag.Bool("config", false, "also require the base-generated config accessors (get_/set_<key>) to be documented")
	flag.Parse()

	dir := "."
	if flag.NArg() > 0 {
		dir = flag.Arg(0)
	}

	if err := run(dir, *readme, splitCSV(*ignore), *checkConfig); err != nil {
		fmt.Fprintln(os.Stderr, "doccov: "+err.Error())
		os.Exit(1)
	}
}

func run(dir, readmeName string, ignore map[string]bool, checkConfig bool) error {
	surface, err := scanSurface(dir)
	if err != nil {
		return err
	}
	var accessors []string
	if checkConfig {
		if accessors, err = scanConfig(dir); err != nil {
			return err
		}
	}
	if len(surface) == 0 && len(accessors) == 0 {
		fmt.Println("doccov: no starlark.NewBuiltin calls or config options found; nothing to check")
		return nil
	}

	readmePath := filepath.Join(dir, readmeName)
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %v", readmePath, err)
	}
	documented := backtickWords(string(data))

	missBuiltins := undocumented(surface, documented, ignore)
	missConfig := undocumented(accessors, documented, ignore)

	fmt.Printf("doccov: %s — builtins %d/%d documented, config accessors %d/%d documented\n",
		readmeName,
		len(surface)-len(missBuiltins), len(surface),
		len(accessors)-len(missConfig), len(accessors))

	var errs []string
	if len(missBuiltins) > 0 {
		errs = append(errs, "undocumented builtins: "+strings.Join(missBuiltins, ", "))
	}
	if len(missConfig) > 0 {
		errs = append(errs, "undocumented config accessors: "+strings.Join(missConfig, ", "))
	}
	if len(errs) > 0 {
		return fmt.Errorf("in %s: %s", readmeName, strings.Join(errs, "; "))
	}
	return nil
}

// undocumented returns the sorted names from want that are neither ignored nor
// present (as a backtick word) in documented.
func undocumented(want []string, documented, ignore map[string]bool) []string {
	var missing []string
	for _, name := range want {
		if ignore[name] || documented[name] {
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return missing
}

// scanSurface returns the sorted, de-duplicated set of script-facing builtin
// names declared in the non-test Go files at the top level of dir.
func scanSurface(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	set := map[string]bool{}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "NewBuiltin" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "starlark" {
				return true
			}
			if lit := stringLit(call.Args[0]); lit != "" {
				set[shortName(lit)] = true
			}
			return true
		})
	}

	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

var (
	configKeyConst = regexp.MustCompile(`(configKey\w+)\s*=\s*"([^"]+)"`)
	configDeclLine = regexp.MustCompile(`ConfigOption\(\s*(configKey\w+)`)
)

// scanConfig returns the sorted config-accessor builtin names that `base`
// auto-generates for a module's config options: set_<name> for every option,
// plus get_<name> for non-secret options. It is convention-based (best-effort):
// it reads the `configKey<X> = "<name>"` constants and the
// gen[Secret]ConfigOption(configKey<X>, …) declarations; a declaration line
// containing "Secret" (genSecretConfigOption or a chained .SetSecret(true))
// marks that option secret, so it gets no get_ accessor. Returns nil if the
// module follows neither convention.
func scanConfig(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	keyValue := map[string]string{} // configKey ident -> option name
	declared := map[string]bool{}   // configKey ident -> registered as an option
	secret := map[string]bool{}     // configKey ident -> secret (set_ only)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		text := string(data)
		for _, m := range configKeyConst.FindAllStringSubmatch(text, -1) {
			keyValue[m[1]] = m[2]
		}
		for _, line := range strings.Split(text, "\n") {
			m := configDeclLine.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			declared[m[1]] = true
			if strings.Contains(line, "Secret") {
				secret[m[1]] = true
			}
		}
	}
	set := map[string]bool{}
	for ident := range declared {
		val, ok := keyValue[ident]
		if !ok {
			continue
		}
		set["set_"+val] = true
		if !secret[ident] {
			set["get_"+val] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// stringLit extracts a string constant from a builtin's first argument. It
// resolves a plain literal ("module.fn") and the common "ModuleName + \".fn\""
// concatenation, returning the literal portion.
func stringLit(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			if s, err := strconv.Unquote(v.Value); err == nil {
				return s
			}
		}
	case *ast.BinaryExpr:
		if l := stringLit(v.X); l != "" {
			return l
		}
		return stringLit(v.Y)
	}
	return ""
}

// shortName reduces a qualified builtin name ("module.fn") to its final segment.
func shortName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

var (
	backtickSpan = regexp.MustCompile("`[^`]+`")
	wordToken    = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
)

// backtickWords collects every identifier word appearing inside a backtick span
// of the document.
func backtickWords(doc string) map[string]bool {
	out := map[string]bool{}
	for _, span := range backtickSpan.FindAllString(doc, -1) {
		for _, w := range wordToken.FindAllString(span, -1) {
			out[w] = true
		}
	}
	return out
}

func splitCSV(s string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out[p] = true
		}
	}
	return out
}
