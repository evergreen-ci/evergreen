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
	"strings"
)

// Package is the base unit for this library.
// A *Package contains metadata.
type Package struct {
	controlData map[string]string // This map should contain all keys/values, with keys using the standard camel case
	//controlDataCaseLookup map[string]string // Case sensitive lookups. TODO
}

func NewPackage() *Package {
	para := &Package{controlData: map[string]string{}}
	return para
}

// Set sets a control field by name
func (pkg *Package) Set(key, value string) {
	nkey := NormaliseFieldKey(key)
	pkg.controlData[nkey] = value
}

func NormaliseFieldKey(input string) string {
	isUcase := true
	newString := ""
	for _, ch := range input {
		if isUcase {
			newString += strings.ToUpper(string(ch))
		} else {
			newString += strings.ToLower(string(ch))
		}
		if ch == '-' {
			isUcase = true
		} else {
			isUcase = false
		}
	}
	return newString
}

// GetExtended gets a control field by name, returning key, value & 'exists'
func (pkg *Package) GetExtended(key string) (string, string, bool) {
	nkey := NormaliseFieldKey(key)
	val, exists := pkg.controlData[nkey]
	return nkey, val, exists
}

func (pkg *Package) Get(key string) string {
	nkey := NormaliseFieldKey(key)
	val, _ := pkg.controlData[nkey]
	return val
}

func CopyPara(pkg *Package) *Package {
	npkg := NewPackage()
	for k, v := range pkg.controlData {
		npkg.controlData[k] = v
	}
	return npkg
}

/*
//merge 2 packages for the purposes of generating a binary package
func Merge(inpkg *Package, in2pkg *Package, ignoreKeys []string) *Package {
	npkg := CopyPara(inpkg)
	for k, v := range in2pkg.controlData {
		//check ignore
		isIgnore := false
		for _, ik := range ignoreKeys {
			if ik == k {
				isIgnore = true
			}
		}
		//overwrite ...
		if !isIgnore {
			_, exists := npkg.controlData[k]
			if !exists {
				npkg.controlData[k] = v
			}
		}
	}
	return npkg
}
/*
func Transform(ctrl *Control, id int) {
	debpkg := CopyPara(ctrl.Paragraphs[id])
	debctrl := NewEmptyControl()
	debctrl.Paragraphs = append(debctrl.Paragraphs, debpkg)
	return debpkg
}
*/
