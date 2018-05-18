// Package vendoring provides a several variables used in vendoring
// buildscripts and function that reports (without any external
// dependencies) if the current environment requires legacy-style
// vendoring, or if its safe to use new-style vendoring.
package vendoring

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var (
	// Path to the vendored gopath component.
	Path = filepath.Join("build", "vendor")

	// Path to the legacy vendor source directory.
	Src = filepath.Join(Path, "src")
)

// NeedsLegacy determines what kind vendoring system to use. Returns
// true when using pre-go1.5 vendoring, and false otherwise.
func NeedsLegacy() bool {
	var version []int

	versionString := strings.TrimPrefix(runtime.Version(), "go")

	// gccgo version strings need a bit more processing.
	if strings.Contains(versionString, "gccgo") {
		versionString = strings.Split(versionString, " ")[0]
	}

	// parse the version number.
	for _, part := range strings.Split(versionString, ".") {
		value, err := strconv.Atoi(part)

		if err != nil {
			// if it's a dev version, and a part of the
			// version string isn't a number, then given
			// that this script is written in the go1.6
			// era, then let's assume that its new-style
			// vendoring.

			return false
		}

		version = append(version, value)
	}

	// determine what kind of vendoring we can use.
	if version[0] > 1 || version[1] >= 7 {
		// go 2.0 and greater, or go 1.7+, do not require
		// legacy verndoring, and can use the top-level vendor
		// directory.
		return false
	} else if version[1] <= 4 {
		// everything less than or equal to go1.4 uses legacy
		// vendoring (e.g. a modified GOPATH.)
		return true
	}

	// for go1.5 and go1.6, use the environment variable to
	// determine the vendoring configuration.
	vendorExperiment := os.Getenv("GO15VENDOREXPERIMENT")
	if version[1] == 6 {
		return vendorExperiment == "0"
	} else if version[1] == 5 {
		return vendorExperiment != "1"
	}

	// last resort, shouldn't ever hit this.
	panic(fmt.Sprintf("cannot determine vendoring requirements for this version of go. (%s)",
		versionString))
}
