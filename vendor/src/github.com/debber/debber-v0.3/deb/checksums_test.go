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
	"path/filepath"
	"testing"
)

func TestChecksums(t *testing.T) {
	checksums := &deb.Checksums{}
	basename := "1.txt"
	p := filepath.Join("testdata", basename)
	err := checksums.Add(p, basename)
	if err != nil {
		t.Fatalf("add failed: %+v", err)
	}
	if len(checksums.ChecksumsMd5) != 1 {
		t.Fatalf("checksums wrong length")
	}
}
