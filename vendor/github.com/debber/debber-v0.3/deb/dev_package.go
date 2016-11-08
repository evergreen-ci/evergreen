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

/*
// NewDevPackage is a factory for creating '-dev' packages from packages.
// It just does a copy, appends "-dev" to the name, and sets the
func NewDevPackage(pkg *Control) *Control {
	//TODO *complete* copy of properties, using reflection?
	devpkg := Copy(pkg)
	devpkg.Paragraphs[0].Set(PackageFName, pkg.Get(PackageFName)+"-dev")
	devpkg.Paragraphs[0].Set(ArchitectureFName, "all")
	return devpkg

}
*/
