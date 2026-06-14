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
	flag.Parse()

	dir := "."
	if flag.NArg() > 0 {
		dir = flag.Arg(0)
	}

	if err := run(dir, *readme, splitCSV(*ignore)); err != nil {
		fmt.Fprintln(os.Stderr, "doccov: "+err.Error())
		os.Exit(1)
	}
}

func run(dir, readmeName string, ignore map[string]bool) error {
	surface, err := scanSurface(dir)
	if err != nil {
		return err
	}
	if len(surface) == 0 {
		fmt.Println("doccov: no starlark.NewBuiltin calls found; nothing to check")
		return nil
	}

	readmePath := filepath.Join(dir, readmeName)
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %v", readmePath, err)
	}
	documented := backtickWords(string(data))

	var missing []string
	for _, name := range surface {
		if ignore[name] || documented[name] {
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)

	fmt.Printf("doccov: %d script-facing builtins, %d documented, %d missing\n",
		len(surface), len(surface)-len(missing), len(missing))
	if len(missing) > 0 {
		return fmt.Errorf("undocumented in %s: %s", readmeName, strings.Join(missing, ", "))
	}
	return nil
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
