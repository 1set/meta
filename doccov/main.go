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

// configVisibility mirrors base's independent script getter/setter controls.
type configVisibility struct {
	key      string
	secret   bool
	hostOnly bool
}

// scanConfig recognizes the ecosystem's configKey constants and ConfigOption
// factory chains. Parse Go syntax so multiline calls, comments, and explicit
// false overrides have exactly the same visibility semantics as single lines.
func scanConfig(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	keys := map[string]string{}
	options := map[string]configVisibility{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		var scanErr error
		ast.Inspect(file, func(node ast.Node) bool {
			if spec, ok := node.(*ast.ValueSpec); ok {
				for i, ident := range spec.Names {
					if i < len(spec.Values) && strings.HasPrefix(ident.Name, "configKey") {
						if value := stringLit(spec.Values[i]); value != "" {
							keys[ident.Name] = value
						}
					}
				}
			}
			expr, ok := node.(ast.Expr)
			if !ok {
				return true
			}
			option, found, err := configOption(expr)
			if err != nil {
				scanErr = err
				return false
			}
			if found {
				options[option.key] = option
				return false
			}
			return true
		})
		if scanErr != nil {
			return nil, fmt.Errorf("%s: %w", name, scanErr)
		}
	}
	set := map[string]bool{}
	for key, option := range options {
		name, ok := keys[key]
		if !ok {
			continue
		}
		if !option.hostOnly {
			set["set_"+name] = true
		}
		if !option.secret {
			set["get_"+name] = true
		}
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func configOption(expr ast.Expr) (configVisibility, bool, error) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return configVisibility{}, false, nil
	}
	if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
		option, found, err := configOption(selector.X)
		if err != nil || found {
			if err == nil && (selector.Sel.Name == "SetHostOnly" || selector.Sel.Name == "SetSecret") {
				if len(call.Args) != 1 {
					return option, true, fmt.Errorf("%s needs a literal boolean", selector.Sel.Name)
				}
				flag, ok := call.Args[0].(*ast.Ident)
				if !ok || (flag.Name != "true" && flag.Name != "false") {
					return option, true, fmt.Errorf("%s needs a literal boolean", selector.Sel.Name)
				}
				if selector.Sel.Name == "SetHostOnly" {
					option.hostOnly = flag.Name == "true"
				} else {
					option.secret = flag.Name == "true"
				}
			}
			return option, found, err
		}
	}
	name := configFactoryName(call.Fun)
	if !strings.HasSuffix(name, "ConfigOption") {
		return configVisibility{}, false, nil
	}
	for _, arg := range call.Args {
		if key, ok := arg.(*ast.Ident); ok && strings.HasPrefix(key.Name, "configKey") {
			return configVisibility{key: key.Name, secret: strings.Contains(name, "Secret")}, true, nil
		}
	}
	return configVisibility{}, false, nil
}

func configFactoryName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.SelectorExpr:
		return expr.Sel.Name
	case *ast.IndexExpr:
		return configFactoryName(expr.X)
	case *ast.IndexListExpr:
		return configFactoryName(expr.X)
	}
	return ""
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
	fencedBlock  = regexp.MustCompile("(?s)```.*?```")
	backtickSpan = regexp.MustCompile("`[^`]+`")
	wordToken    = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
)

// backtickWords collects every identifier word that appears inside code in the
// document — both ```fenced``` blocks and `inline` spans. Fenced blocks are
// processed and removed first so their backticks don't throw off the
// single-backtick span matcher (which otherwise mis-pairs across a fence, so a
// `code` reference in a table *after* a fenced example could be missed).
func backtickWords(doc string) map[string]bool {
	out := map[string]bool{}
	add := func(s string) {
		for _, w := range wordToken.FindAllString(s, -1) {
			out[w] = true
		}
	}
	rest := fencedBlock.ReplaceAllStringFunc(doc, func(block string) string {
		add(block)
		return "\n"
	})
	for _, span := range backtickSpan.FindAllString(rest, -1) {
		add(span)
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
