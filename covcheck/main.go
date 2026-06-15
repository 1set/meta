// Command covcheck is the test-coverage gate for the Star* ecosystem.
//
// `make ci` writes a Go coverage profile (go test -coverprofile=coverage.txt).
// covcheck parses that profile, computes the total statement coverage, and fails
// when it is below a required minimum — a reliable, self-contained merge gate
// that does not depend on an external coverage service posting a commit status.
//
// Usage:
//
//	covcheck [flags] [coverage.txt]        # file defaults to coverage.txt
//	go run github.com/1set/meta/covcheck@<ref> -min 70 coverage.txt
//
// Flags:
//
//	-min <pct>   minimum total coverage percent required (default 0 = report only)
//
// The total is computed the same way as `go tool cover -func` (covered
// statements / total statements), using only the standard library. Exit status
// is non-zero when coverage is below -min or the profile is missing/empty.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	min := flag.Float64("min", 0, "minimum total coverage percent required")
	flag.Parse()

	path := "coverage.txt"
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}

	pct, err := totalCoverage(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "covcheck: "+err.Error())
		os.Exit(1)
	}

	fmt.Printf("covcheck: total coverage %.1f%% (min %.1f%%)\n", pct, *min)
	if pct+1e-9 < *min {
		fmt.Fprintf(os.Stderr, "covcheck: coverage %.1f%% is below the required minimum %.1f%%\n", pct, *min)
		os.Exit(1)
	}
}

// totalCoverage parses a Go coverage profile and returns the total statement
// coverage percent: covered statements / total statements * 100.
func totalCoverage(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("cannot read coverage profile %s: %v", path, err)
	}
	defer f.Close()

	var total, covered int64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// Format: <path>:<startLine.col>,<endLine.col> <numStatements> <count>
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		numStmt, err1 := strconv.ParseInt(fields[len(fields)-2], 10, 64)
		count, err2 := strconv.ParseInt(fields[len(fields)-1], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		total += numStmt
		if count > 0 {
			covered += numStmt
		}
	}
	if err := sc.Err(); err != nil {
		return 0, fmt.Errorf("reading %s: %v", path, err)
	}
	if total == 0 {
		return 0, fmt.Errorf("no statements found in %s (empty or not a coverage profile)", path)
	}
	return 100 * float64(covered) / float64(total), nil
}
