package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/pkg/errors"
)

const (
	goModFile = "go.mod"
	goSumFile = "go.sum"
)

// verify-mod-tidy verifies that `go mod tidy` has been run to clean up the
// go.mod and go.sum files.
func main() {
	var (
		goBin   string
		timeout time.Duration
	)

	flag.DurationVar(&timeout, "timeout", 0, "timeout for verifying modules are tidy")
	flag.StringVar(&goBin, "goBin", "go", "path to go binary to use for mod tidy check")
	flag.Parse()

	ctx := context.Background()
	if timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	oldGoMod, oldGoSum, err := readModuleFiles()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := runModTidy(ctx, goBin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	newGoMod, newGoSum, err := readModuleFiles()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if !bytes.Equal(oldGoMod, newGoMod) || !bytes.Equal(oldGoSum, newGoSum) {
		fmt.Fprintf(os.Stderr, "%s and/or %s are not tidy - please run `go mod tidy`.\n", goModFile, goSumFile)
		writeModuleFiles(oldGoMod, oldGoSum)
		os.Exit(1)
	}
}

// readModuleFiles reads the contents of the go module files.
func readModuleFiles() (goMod []byte, goSum []byte, err error) {
	goMod, err = os.ReadFile(goModFile)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "reading file '%s'", goModFile)
	}
	goSum, err = os.ReadFile(goSumFile)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "reading file '%s'", goSumFile)
	}
	return goMod, goSum, nil
}

// writeModuleFiles writes the contents of the go module files.
func writeModuleFiles(goMod, goSum []byte) {
	if err := os.WriteFile(goModFile, goMod, 0600); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if err := os.WriteFile(goSumFile, goSum, 0600); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// runModTidy runs the `go mod tidy` command with the given go binary.
func runModTidy(ctx context.Context, goBin string) error {
	cmd := exec.CommandContext(ctx, goBin, "mod", "tidy")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return errors.Wrap(cmd.Run(), "mod tidy")
}
