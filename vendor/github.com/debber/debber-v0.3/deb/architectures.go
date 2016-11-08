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
	"strings"
)

// Architecture - processor architecture (ARM/x86/AMD64) - as named by Debian.
// At this stage: i386, armhf, amd64 and 'all'.
// Note that 'any' is not valid for a binary package, and resolves to [i386, armhf, amd64]
// TODO: armel
// (Note that armhf = ARMv7 and armel = ARMv5. In Go terms, this is is is governed by the environment variable GOARM, and 7 is the default)
type Architecture string

const (
	//ArchI386 represents x86 machines
	ArchI386 Architecture = "i386"
	//ArchArmhf represents ARMv7 (TODO: armel)
	ArchArmhf Architecture = "armhf"
	//ArchAmd64 represents 64-bit machines.
	ArchAmd64 Architecture = "amd64"
	//ArchAll is for binary packages.
	ArchAll Architecture = "all"
)

//ResolveArches currently parses 'Binary' arches only
func ResolveArches(arches string) ([]Architecture, error) {
	arches = strings.TrimSpace(arches)

	//recurse ...
	if strings.Contains(arches, ",") {
		archesArr := strings.Split(arches, ",")
		architectures := []Architecture{}
		for _, arch := range archesArr {
			theseArchitectures, err := ResolveArches(arch)
			if err != nil {
				return nil, err
			}
			architectures = append(architectures, theseArchitectures...)
		}
		return architectures, nil
	}

	//strip linux- prefix
	if strings.HasPrefix(arches, "linux-") {
		return ResolveArches(strings.TrimPrefix(arches, "linux-"))
	}
	//reject other OSes (not supported yet)
	if strings.Contains(arches, "-") {
		return nil, fmt.Errorf("Linux is the only OS supported. Sorry")
	}

	if arches == "any" || arches == "" {
		return []Architecture{ArchI386, ArchArmhf, ArchAmd64}, nil
	} else if arches == string(ArchI386) {
		return []Architecture{ArchI386}, nil
	} else if arches == string(ArchArmhf) {
		return []Architecture{ArchArmhf}, nil
	} else if arches == string(ArchAmd64) {
		return []Architecture{ArchAmd64}, nil
	} else if arches == string(ArchAll) {
		return []Architecture{ArchAll}, nil
	}
	return nil, fmt.Errorf("Architecture %s not supported", arches)
}
