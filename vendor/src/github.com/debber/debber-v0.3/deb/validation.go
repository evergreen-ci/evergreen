/*
   Copyright 2013 Am Laher

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package deb

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidateArchitecture checks the architecture string by parsing it.
// Returns an error on failure.
// Only supported architectures are considered valid. (e.g. armel is not supported, and non-linux OSes are also not supported yet)
//
// See https://www.debian.org/doc/debian-policy/ch-controlfields.html#s-f-Architecture
func ValidateArchitecture(archString string) error {
	if archString == "source" { //OK
		return nil
	}
	_, err := ResolveArches(archString)
	return err
}

// ValidateName validates a package name for both 'Source:' and 'Package:' names
//
// See http://www.debian.org/doc/debian-policy/ch-controlfields.html#s-f-Source
func ValidateName(packageName string) error {
	if packageName == "" {
		return fmt.Errorf("'Package' field is required")
	}
	validName := regexp.MustCompile(`^[a-z0-9][a-z0-9+-.]+$`)
	if !validName.MatchString(packageName) {
		return fmt.Errorf("Invalid package name")
	}
	return nil
}

// ValidateVersion checks a version string against the policy manual definition.
//
// See ParseVersion
func ValidateVersion(packageVersion string) error {
	_, _, _, err := ParseVersion(packageVersion)
	return err
}

// ParseVersion parses a debian version string into its [up to] 3 parts.
// Briefly, the format is: [epoch:]upstream_version[-debian_revision]
//
// See http://www.debian.org/doc/debian-policy/ch-controlfields.html#s-f-Version
func ParseVersion(packageVersion string) (string, string, string, error) {
	if packageVersion == "" {
		return "", "", "", fmt.Errorf("Version property is required")
	}
	var validVersion *regexp.Regexp
	//isEpoch and debian_revision
	var isEpoch bool
	var isDebianRevision bool
	if strings.Index(packageVersion, ":") > -1 && strings.Index(packageVersion, "-") > -1 {
		validVersion = regexp.MustCompile(`^(.*):([0-9][a-zA-Z0-9+-.:~]+)-([a-zA-Z0-9+.~]+)$`)
		isEpoch = true
		isDebianRevision = true
	} else if strings.Index(packageVersion, ":") > -1 { //isEpoch
		validVersion = regexp.MustCompile(`^(.*):([0-9][a-zA-Z0-9+.:~]+)$`)
		isEpoch = true
		isDebianRevision = false
	} else if strings.Index(packageVersion, "-") > -1 { //revision
		validVersion = regexp.MustCompile(`^([0-9][a-zA-Z0-9+-.~]+)-([a-zA-Z0-9+.~]+)$`)
		isEpoch = false
		isDebianRevision = true
	} else { // neither
		validVersion = regexp.MustCompile(`^([0-9][a-zA-Z0-9+.~]+)$`)
		isEpoch = false
		isDebianRevision = false
	}
	submatches := validVersion.FindStringSubmatch(packageVersion)
	//log.Printf("%s: %v %v subs: %v\n", packageVersion, isEpoch, isDebianRevision, submatches)
	if submatches == nil {
		return "", "", "", fmt.Errorf("Invalid version property '%s'", packageVersion)
	}
	epoch := ""
	upstreamRevision := ""
	debianRevision := ""
	if isEpoch {
		epoch = submatches[1]
		upstreamRevision = submatches[2]
		if isDebianRevision {
			debianRevision = submatches[3]
		}
	} else {
		upstreamRevision = submatches[1]
		if isDebianRevision {
			debianRevision = submatches[2]
		}
	}
	return epoch, upstreamRevision, debianRevision, nil

}

// ValidatePackage checks required fields and certain restricted values.
//
// This can be considered a work-in-progress.
func ValidatePackage(pkg *Package) error {
	sourceName := pkg.Get(SourceFName)
	if sourceName == "" {
		err := ValidateName(pkg.Get(PackageFName))
		if err != nil {
			return err
		}

	} else {
		err := ValidateName(pkg.Get(SourceFName))
		if err != nil {
			return err
		}
	}
	/* Version is not required in a package (the source debian/control doesn't contain one)
	err = ValidateVersion(pkg.Get(VersionFName))
	if err != nil {
		return err
	}

	*/
	return nil
}

func ValidateControl(ctrl *Control) error {

	if ctrl.Get(MaintainerFName) == "" {
		return fmt.Errorf("Maintainer property is required")
	}
	for _, pkg := range *ctrl {
		err := ValidatePackage(pkg)
		if err != nil {
			return err
		}
	}
	return nil
}
