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
	"archive/tar"
	"fmt"
	"github.com/debber/debber-v0.3/targz"
	"github.com/laher/argo/ar"
	"io"
	"io/ioutil"
	"log"
	"strings"
)

//Reader is a wrapper around an io.Reader and an ar.Reader.
type Reader struct {
	Reader           io.Reader
	ArReader         *ar.Reader
	HasDebianVersion bool
}

//NewReader is a factory for deb.Reader
func NewReader(rdr io.Reader) (*Reader, error) {
	drdr := &Reader{Reader: rdr, HasDebianVersion: false}
	arr, err := ar.NewReader(rdr)
	if err != nil {
		return nil, err
	}
	drdr.ArReader = arr
	return drdr, err
}

//NextTar gets next tar header (for supported tar archive types - initially just tar.gz)
func (drdr *Reader) NextTar() (string, *tar.Reader, error) {
	for {
		hdr, err := drdr.ArReader.Next()
		if err != nil {
			return "", nil, err
		}
		if hdr.Name == "debian-binary" {
			drdr.HasDebianVersion = true
			continue
		}
		if strings.HasSuffix(hdr.Name, ".tar.gz") {
			tgzr, err := targz.NewReader(drdr.ArReader)
			return hdr.Name, tgzr.Reader, err
		}
		// else return error
		return hdr.Name, nil, fmt.Errorf("Unsuported file type: %s", hdr.Name)
	}
}

// ParseDebMetadata reads an artifact's contents.
func ParseDebMetadata(rdr io.Reader) (*Control, error) {

	arr, err := ar.NewReader(rdr)
	if err != nil {
		return nil, err
	}

	var pkg *Control
	pkg = nil
	hasDataArchive := false
	hasControlArchive := false
	hasControlFile := false
	hasDebianBinaryFile := false

	// Iterate through the files in the archive.
	for {
		hdr, err := arr.Next()
		if err == io.EOF {
			// end of ar archive
			break
		}
		if err != nil {
			return nil, err
		}
		//		t.Logf("File %s:\n", hdr.Name)
		if hdr.Name == BinaryDataArchiveNameDefault {
			// SKIP!
			hasDataArchive = true
		} else if hdr.Name == BinaryControlArchiveNameDefault {
			// Find control file
			hasControlArchive = true
			tgzr, err := targz.NewReader(arr)
			if err != nil {
				return nil, err
			}
			for {
				thdr, err := tgzr.Next()
				if err == io.EOF {
					// end of tar.gz archive
					break
				}
				if err != nil {
					return nil, err
				}
				if thdr.Name == "control" {
					hasControlFile = true
					dscr := NewControlFileReader(tgzr)
					pkg, err = dscr.Parse()
					if err != nil {
						return nil, err
					}
				} else {
					//SKIP
					log.Printf("File %s", thdr.Name)
				}
			}

		} else if hdr.Name == "debian-binary" {
			b, err := ioutil.ReadAll(arr)
			if err != nil {
				return nil, err
			}
			hasDebianBinaryFile = true
			if string(b) != "2.0\n" {
				return nil, fmt.Errorf("Binary version not valid: %s", string(b))
			}
		} else {
			return nil, fmt.Errorf("Unsupported file %s", hdr.Name)
		}
	}

	if !hasDebianBinaryFile {
		return nil, fmt.Errorf("No debian-binary file in .deb archive")
	}
	if !hasDataArchive {
		return nil, fmt.Errorf("No data.tar.gz file in .deb archive")
	}
	if !hasControlArchive {
		return nil, fmt.Errorf("No control.tar.gz file in .deb archive")
	}
	if !hasControlFile {
		return nil, fmt.Errorf("No debian/control file in control.tar.gz")
	}
	return pkg, nil
}
