/*
The current vendoring solution supports both new and old style
vendoring, via a trick: We commit all vendored code to the "vendor"
directory, and then, if we're on a version/deployment of go that
doesn't support new style vendoring, we symlink to "build/vendor/src"
and add "build/vendor" to the gopath, which the render-gopath program
generates inside of the makefile.

This script sets up the symlink. This is somewhat fragile for go1.5
and go1.6, if you change the value of the GO15VENDOREXPERIMENT
environment variable, in between runs of "make-vendor" and
builds. Similar switching between go1.4 environments and go1.6
will require manually rerunning this script.
*/
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"./vendoring"
)

func main() {
	if vendoring.NeedsLegacy() {
		pwd, err := os.Getwd()
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		path := filepath.Join(pwd, vendoring.Path)

		if _, err = os.Stat(path); !os.IsNotExist(err) {
			fmt.Println("legacy vendor path configured.")
			return
		}

		err = os.MkdirAll(path, 0755)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)

		}

		err = os.Symlink(filepath.Join(pwd, "vendor"), filepath.Join(pwd, vendoring.Src))
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		fmt.Println("created vendor legacy link")
		return
	}

	fmt.Println("can use existing (new-style) vendoring configuration")
}
