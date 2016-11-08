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
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ControlFileReader reads a control file.
type ControlFileReader struct {
	Reader io.Reader
}

//NewControlFileReader is a factory for reading Dsc files.
func NewControlFileReader(rdr io.Reader) *ControlFileReader {
	return &ControlFileReader{rdr}
}

const (
	beginPGPSignature     = "-----BEGIN PGP SIGNATURE-----"
	endPGPSignature       = "-----END PGP SIGNATURE-----"
	beginPGPSignedMessage = "-----BEGIN PGP SIGNED MESSAGE-----"
)

// Parse parses a stream into a package definition.
func (dscr *ControlFileReader) Parse() (*Control, error) {
	ctrl := NewControlEmpty()
	br := bufio.NewReader(dscr.Reader)
	para := 0
	lastField := ""
	lastVal := ""
	isSig := false
	for {
		line, err := br.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		colonIndex := strings.Index(line, ":")
		spaceIndex := strings.Index(line, " ")
		//Handle signed DSC files (but not the signature itself)
		if strings.HasPrefix(line, "-----BEGIN PGP") || strings.HasPrefix(line, "-----END PGP") {
			switch strings.TrimSpace(line) {
			case beginPGPSignature:
				//TODO: swallow subsequent lines
				isSig = true
			case endPGPSignature:
				//continue
				isSig = false
			case beginPGPSignedMessage:
				//continue
			default:
				return nil, fmt.Errorf("Unrecognised PGP line: %s", line)
			}
			// part of signature.
		} else if isSig {
			// ignore this signature line.
		} else if colonIndex > -1 && (spaceIndex == -1 || colonIndex < spaceIndex) { //New field:
			res := strings.SplitN(line, ":", 2)
			lastField = res[0]
			lastVal = strings.TrimSpace(res[1])
			(*ctrl)[para].Set(lastField, lastVal)
		} else if len(strings.TrimSpace(line)) == 0 { //Empty line == New paragraph:
			para++
			for len(*ctrl) < para+1 {
				*ctrl = append(*ctrl, NewPackage())
			}
		} else if spaceIndex == 0 { //Additional line for current field:
			lastVal += "\n" + strings.TrimSuffix(line, "\n")
			(*ctrl)[para].Set(lastField, lastVal)
		} else {
			return nil, fmt.Errorf("Unexpected line: '%s' / first colon: %d / first space: %d", line, colonIndex, spaceIndex)
		}
	}
	return ctrl, nil
}
