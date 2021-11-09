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

package targz

import (
	"archive/tar"
	"compress/gzip"
	"io"
)

// A Reader provides sequential access to the contents of a tar.gz archive.
// A tar.gz archive consists of a sequence of files.
// The Next method advances to the next file in the archive (including the first),
// and then it can be treated as an io.Reader to access the file's data.
type Reader struct {
	*tar.Reader
	WrappedReader io.Reader
	GzipReader    *gzip.Reader
}

// NewReader creates a new Reader reading from r.
func NewReader(r io.Reader) (*Reader, error) {
	tgzr := &Reader{WrappedReader: r}
	var err error
	tgzr.GzipReader, err = gzip.NewReader(tgzr.WrappedReader)
	if err != nil {
		return nil, err
	}
	tgzr.Reader = tar.NewReader(tgzr.GzipReader)
	return tgzr, err
}

/*
func (r *Reader) Next() (*tar.Header, error) {
	return r.Tr.Next()
}

func (r *Reader) Read(b []byte) (int, error) {
	return r.Tr.Read(b)
}
*/

// Close closes the gzip reader
func (tgzr *Reader) Close() error {
	err := tgzr.GzipReader.Close()
	return err
}
