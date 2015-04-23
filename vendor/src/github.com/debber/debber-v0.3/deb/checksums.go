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
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// Checksum stores a checksum for a file
type Checksum struct {
	Checksum string
	Size     int64
	File     string
}

// Checksums stores the 3 required checksums for a list of files
type Checksums struct {
	ChecksumsMd5    []Checksum
	ChecksumsSha1   []Checksum
	ChecksumsSha256 []Checksum
}

// Add adds checksum entries for each checksum algorithm
func (cs *Checksums) Add(filepath, basename string) error {
	checksumMd5, checksumSha1, checksumSha256, err := checksums(filepath, basename)
	if err != nil {
		return err
	}
	cs.ChecksumsMd5 = append(cs.ChecksumsMd5, *checksumMd5)
	cs.ChecksumsSha1 = append(cs.ChecksumsSha1, *checksumSha1)
	cs.ChecksumsSha256 = append(cs.ChecksumsSha256, *checksumSha256)
	return nil
}

func checksums(path, name string) (*Checksum, *Checksum, *Checksum, error) {
	//checksums
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}

	hashMd5 := md5.New()
	size, err := io.Copy(hashMd5, f)
	if err != nil {
		return nil, nil, nil, err
	}
	checksumMd5 := Checksum{hex.EncodeToString(hashMd5.Sum(nil)), size, name}

	_, err = f.Seek(int64(0), 0)
	if err != nil {
		return nil, nil, nil, err
	}
	hash256 := sha256.New()
	size, err = io.Copy(hash256, f)
	if err != nil {
		return nil, nil, nil, err
	}
	checksumSha256 := Checksum{hex.EncodeToString(hash256.Sum(nil)), size, name}

	_, err = f.Seek(int64(0), 0)
	if err != nil {
		return nil, nil, nil, err
	}
	hash1 := sha1.New()
	size, err = io.Copy(hash1, f)
	if err != nil {
		return nil, nil, nil, err
	}
	checksumSha1 := Checksum{hex.EncodeToString(hash1.Sum(nil)), size, name}

	err = f.Close()
	if err != nil {
		return nil, nil, nil, err
	}

	return &checksumMd5, &checksumSha1, &checksumSha256, nil

}
