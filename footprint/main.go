// Command footprint measures the binary-size cost a Star* module adds.
//
// It builds two tiny programs against the module under test: a BASELINE that
// only spins up a starlet host, and a WITH program that additionally constructs
// the module (NewModule().LoadModule()), forcing the module and its transitive
// SDKs to link. The marginal footprint is (with - baseline), reported for both
// the default and the stripped (-ldflags="-s -w") build.
//
// Usage:
//
//	footprint -modpath github.com/starpkg/<m> [-dir .] [flags]
//
// Flags:
//
//	-modpath <path>   the module's import path (required)
//	-dir <dir>        the local module directory (default ".")
//	-json             emit a shields.io endpoint JSON (badge) on stdout
//	-max-mb <n>       fail if the stripped delta exceeds n MB (0 = no gate)
//
// Build environment (Go toolchain, module cache) is inherited; run it on the
// repo's go floor for comparable numbers. Exit status is non-zero on build
// failure or when -max-mb is exceeded.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	modpath := flag.String("modpath", "", "module import path, e.g. github.com/starpkg/sqlite")
	dir := flag.String("dir", ".", "local module directory")
	jsonOut := flag.Bool("json", false, "emit shields.io endpoint JSON")
	maxMB := flag.Float64("max-mb", 0, "fail if the stripped delta exceeds this many MB (0 = no gate)")
	flag.Parse()

	if *modpath == "" {
		fmt.Fprintln(os.Stderr, "footprint: -modpath is required")
		os.Exit(2)
	}
	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "footprint: "+err.Error())
		os.Exit(1)
	}

	r, err := measure(*modpath, absDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "footprint: "+err.Error())
		os.Exit(1)
	}

	if *jsonOut {
		// shields.io endpoint schema
		fmt.Printf(`{"schemaVersion":1,"label":"binary footprint","message":"+%.1f MB","color":"blue"}`+"\n", mb(r.strippedDelta))
		return
	}

	fmt.Printf("footprint %s\n", *modpath)
	fmt.Printf("  default : baseline %.1f MB  with %.1f MB  delta +%.1f MB (+%.0f%%)\n",
		mb(r.defBase), mb(r.defWith), mb(r.defDelta), pct(r.defDelta, r.defBase))
	fmt.Printf("  stripped: baseline %.1f MB  with %.1f MB  delta +%.1f MB (+%.0f%%)\n",
		mb(r.stripBase), mb(r.stripWith), mb(r.strippedDelta), pct(r.strippedDelta, r.stripBase))

	if *maxMB > 0 && mb(r.strippedDelta) > *maxMB+1e-9 {
		fmt.Fprintf(os.Stderr, "footprint: stripped delta +%.1f MB exceeds the ceiling %.1f MB\n", mb(r.strippedDelta), *maxMB)
		os.Exit(1)
	}
}

type result struct {
	defBase, defWith, defDelta          int64
	stripBase, stripWith, strippedDelta int64
}

func measure(modpath, absDir string) (*result, error) {
	tmp, err := os.MkdirTemp("", "footprint-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	baseline := "package main\n\nimport \"github.com/1set/starlet\"\n\nfunc main() { _ = starlet.NewDefault() }\n"
	with := "package main\n\nimport (\n\t\"github.com/1set/starlet\"\n\tmod \"" + modpath + "\"\n)\n\nfunc main() {\n\t_ = starlet.NewDefault()\n\t_ = mod.NewModule().LoadModule()\n}\n"

	// Require only the module under test (replaced to the local dir); `go mod
	// tidy` then resolves starlet and the rest from the module's own go.mod.
	goDirective := readGoDirective(filepath.Join(absDir, "go.mod"))
	gomod := "module footprintprobe\n\ngo " + goDirective + "\n\nrequire " + modpath + " v0.0.0\n\nreplace " + modpath + " => " + absDir + "\n"

	mustWrite := func(rel, content string) error {
		p := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		return os.WriteFile(p, []byte(content), 0o644)
	}
	if err := mustWrite("go.mod", gomod); err != nil {
		return nil, err
	}
	if err := mustWrite("baseline/main.go", baseline); err != nil {
		return nil, err
	}
	if err := mustWrite("with/main.go", with); err != nil {
		return nil, err
	}

	// Resolve dependencies through the local module's go.mod (the replace).
	if out, err := run(tmp, "go", "mod", "tidy"); err != nil {
		return nil, fmt.Errorf("go mod tidy: %v\n%s", err, out)
	}

	build := func(pkg, out string, strip bool) (int64, error) {
		args := []string{"build", "-trimpath", "-o", out}
		if strip {
			args = append(args, "-ldflags=-s -w")
		}
		args = append(args, "./"+pkg)
		if o, err := run(tmp, "go", args...); err != nil {
			return 0, fmt.Errorf("go build %s: %v\n%s", pkg, err, o)
		}
		fi, err := os.Stat(filepath.Join(tmp, out))
		if err != nil {
			return 0, err
		}
		return fi.Size(), nil
	}

	r := &result{}
	if r.defBase, err = build("baseline", "b_def", false); err != nil {
		return nil, err
	}
	if r.defWith, err = build("with", "w_def", false); err != nil {
		return nil, err
	}
	if r.stripBase, err = build("baseline", "b_str", true); err != nil {
		return nil, err
	}
	if r.stripWith, err = build("with", "w_str", true); err != nil {
		return nil, err
	}
	r.defDelta = r.defWith - r.defBase
	r.strippedDelta = r.stripWith - r.stripBase
	return r, nil
}

// readGoDirective returns the `go` version from a go.mod (default "1.19").
func readGoDirective(gomodPath string) string {
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return "1.19"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "go "))
		}
	}
	return "1.19"
}

func run(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func mb(b int64) float64 { return float64(b) / (1024 * 1024) }
func pct(d, base int64) float64 {
	if base == 0 {
		return 0
	}
	return 100 * float64(d) / float64(base)
}
