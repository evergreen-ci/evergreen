package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type result struct {
	name     string
	cmd      string
	passed   bool
	duration time.Duration
	output   []string
}

// String prints the results of a linter run in gotest format.
func (r *result) String() string {
	buf := &bytes.Buffer{}

	fmt.Fprintln(buf, "=== RUN", r.name)
	if r.passed {
		fmt.Fprintf(buf, "--- PASS: %s (%s)", r.name, r.duration)
	} else {
		fmt.Fprintf(buf, strings.Join(r.output, "\n"))
		fmt.Fprintf(buf, "--- FAIL: %s (%s)", r.name, r.duration)
	}

	return buf.String()
}

// fixup goes through the output and improves the output generated by
// specific linters so that all output includes the relative path to the
// error, instead of mixing relative and absolute paths.
func (r *result) fixup(dirname string) {
	for idx, ln := range r.output {
		if strings.HasPrefix(ln, dirname) {
			r.output[idx] = ln[len(dirname)+1:]
		}
	}
}

// runs the gometalinter on a list of packages; integrating with the "make lint" target.
func main() {
	var (
		lintArgs       string
		lintBin        string
		packageList    string
		output         string
		packages       []string
		results        []*result
		hasFailingTest bool
	)
	gopath := os.Getenv("GOPATH")

	flag.StringVar(&lintArgs, "lintArgs", "", "additional args to pass to the linter")
	flag.StringVar(&lintBin, "lintBin", filepath.Join(gopath, "bin", "golangci-lint"), "path to linter")
	flag.StringVar(&packageList, "packages", "", "list of space-separated packages")
	flag.StringVar(&output, "output", "", "output file to write results")
	flag.Parse()

	packages = strings.Split(strings.Replace(packageList, "-", "/", -1), " ")
	dirname, _ := os.Getwd()

	for _, pkg := range packages {
		args := []string{lintBin, "run", lintArgs}
		if pkg == filepath.Base(dirname) {
			args = append(args, "./")
		} else {
			args = append(args, "./"+pkg)
		}

		startAt := time.Now()
		cmd := strings.Join(args, " ")
		out, err := exec.Command("sh", "-c", cmd).CombinedOutput()

		r := &result{
			cmd:      strings.Join(args, " "),
			name:     "lint-" + strings.Replace(pkg, "/", "-", -1),
			passed:   err == nil,
			duration: time.Since(startAt),
			output:   strings.Split(string(out), "\n"),
		}
		r.fixup(dirname)

		if !r.passed {
			hasFailingTest = true
		}

		results = append(results, r)
		fmt.Println(r)
	}

	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			os.Exit(1)
		}
		defer func() {
			if err != f.Close() {
				panic(err)
			}
		}()

		for _, r := range results {
			f.WriteString(r.String() + "\n")
		}
	}

	if hasFailingTest {
		os.Exit(1)
	}
}
