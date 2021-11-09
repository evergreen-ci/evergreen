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

package debgen

import (
	"github.com/debber/debber-v0.3/deb"
)

// Applies go-specific information to packages.
// i.e. dependencies
func ApplyGoDefaults(ctrl *deb.Control) {
	for _, pkg := range ctrl.SourceParas() {
		pkg.Set(deb.BuildDependsFName, deb.BuildDependsGoDefault)
	}
	for _, pkg := range ctrl.BinaryParas() {
		pkg.Set(deb.DependsFName, deb.DependsDefault)
	}
}

// Applies non-go-specific information to packages.
// i.e. dependencies
func ApplyBasicDefaults(ctrl *deb.Control) {
	for _, pkg := range ctrl.SourceParas() {
		pkg.Set(deb.BuildDependsFName, deb.BuildDependsDefault)
	}
	for _, pkg := range ctrl.BinaryParas() {
		pkg.Set(deb.DependsFName, deb.DependsDefault)
	}
}
