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

package deb_test

import (
	"github.com/debber/debber-v0.3/deb"
	"testing"
)

func TestCopy(t *testing.T) {
	ctrl := deb.NewControlDefault("testpkg", "me", "me@a", "Dummy package for doing nothing", "testpkg is package ", true)
	nctrl := deb.Copy(ctrl)
	if ctrl == nctrl {
		t.Errorf("Copy returned the same reference - not a copy")
	}
	if ctrl.Get(deb.PackageFName) != nctrl.Get(deb.PackageFName) {
		t.Errorf("Copy didn't copy the same Name value")
	}
	t.Logf("Original: %+v", ctrl)
	t.Logf("Copy:     %+v", nctrl)
}
