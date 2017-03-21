// Simple script to print the current GOPATH with additional vendoring
// component for legacy vendoring, as needed. Use in conjunction with
// makefile configuration and the "make-vendor" script.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"./vendoring"
)

func main() {
	currentGoPath := os.Getenv("GOPATH")
	pwd, err := os.Getwd()

	// print error and exit if there's an error
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// if this version of go does not support new-style vendoring, then we need to mangle the gopath.
	if vendoring.NeedsLegacy() {
		fmt.Printf("GOPATH=%s:%s", currentGoPath, filepath.Join(pwd, vendoring.Path))
		return
	}

	// in all other cases, we can just echo the gopath here.
	fmt.Printf("GOPATH=%s", currentGoPath)
}
