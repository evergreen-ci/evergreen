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
	"fmt"
	"io"
	"os"
)

// Writer encapsulates the tar, gz and file operations of a .tar.gz file.
type Writer struct {
	*tar.Writer // Tar writer (wraps the GzipWriter)
	//Filename string       // Filename
	WrappedWriter io.Writer    // File writer
	GzipWriter    *gzip.Writer // Gzip writer (wraps the WrappedWriter)
}

// Close closes all 3 writers.
// Returns the first error
func (tgzw *Writer) Close() error {
	err1 := tgzw.Writer.Close()
	err2 := tgzw.GzipWriter.Close()
	if err1 != nil {
		return fmt.Errorf("Error closing Tar Writer %v", err1)
	}
	if err2 != nil {
		return fmt.Errorf("Error closing Gzip Writer %v", err2)
	}
	return nil
}

// NewWriterFromFile is a factory for Writer
func NewWriterFromFile(archiveFilename string) (*Writer, error) {
	fw, err := os.Create(archiveFilename)
	if err != nil {
		return nil, err
	}
	tgzw := NewWriter(fw)
	return tgzw, err
}

// NewWriter is a factory for Writer.
// It wraps the io.Writer with a Tar writer and Gzip writer
func NewWriter(w io.Writer) *Writer {
	tgzw := &Writer{WrappedWriter: w}
	// gzip writer
	tgzw.GzipWriter = gzip.NewWriter(w)
	// tar writer
	tgzw.Writer = tar.NewWriter(tgzw.GzipWriter)
	return tgzw
}
