// Simple script to print the current GOPATH with additional vendoring
// component for legacy vendoring, as needed. Use in conjunction with
// makefile configuration and the "make-vendor" script.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// initialize the gopath components.
	goPathParts := []string{currentGoPath}

	// if this version of go does not support new-style vendoring,
	// then we need to mangle the gopath so that the build can use
	// vendored dependencies.
	if vendoring.NeedsLegacy() {
		goPathParts = append(goPathParts, filepath.Join(pwd, vendoring.Path))

		// add any additional paths to nested vendored gopaths.
		for _, path := range os.Args[1:] {
			absPath, err := filepath.Abs(path)

			if err == nil {
				goPathParts = append(goPathParts, absPath)
			} else {
				goPathParts = append(goPathParts, path)
			}
		}
	}

	fmt.Printf("GOPATH=%s", strings.Join(goPathParts, ":"))
}
