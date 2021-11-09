// Copyright 2013 Am Laher.
// This code is adapted from code within the Go tree.
// See Go's licence information below:
//
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ar

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	//ErrWriteAfterClose shows that a write was attempted after the archive has been closed (and the footer is written)
	ErrWriteAfterClose = errors.New("ar: write after close")
	errNameTooLong     = errors.New("ar: name too long")
	errInvalidHeader   = errors.New("ar: header field too long or contains invalid values")
)

// A Writer provides sequential writing of an ar archive.
// An ar archive consists of a sequence of files.
// Call WriteHeader to begin a new file, and then call Write to supply that file's data,
// writing at most hdr.Size bytes in total.
type Writer struct {
	w                       io.Writer
	arFileHeaderWritten     bool
	err                     error
	nb                      int64 // number of unwritten bytes for current file entry
	pad                     bool  // whether the file will be padded an extra byte (i.e. if ther's an odd number of bytes in the file)
	closed                  bool
	TerminateFilenamesSlash bool // This flag determines whether to terminate filenames with a slash '/' or not. GNU ar uses slashes, whereas .deb files tend not to use them.
}

// NewWriter creates a new Writer writing to w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Flush finishes writing the current file (optional. This is called by writeHeader anyway.)
func (aw *Writer) Flush() error {
	if aw.nb > 0 {
		aw.err = fmt.Errorf("ar: missed writing %d bytes", aw.nb)
		return aw.err
	}
	if !aw.arFileHeaderWritten {
		_, aw.err = aw.w.Write([]byte(ArFileHeader))
		if aw.err != nil {
			return aw.err
		}
		aw.arFileHeaderWritten = true
	}
	if aw.pad {
		//pad with a newline
		if _, aw.err = aw.w.Write([]byte("\n")); aw.err != nil {
			return aw.err
		}
	}
	aw.nb = 0
	aw.pad = false
	return aw.err
}

// WriteHeader writes hdr and prepares to accept the file's contents.
// WriteHeader calls Flush if it is not the first header.
// Calling after a Close will return ErrWriteAfterClose.
func (aw *Writer) WriteHeader(hdr *Header) error {
	return aw.writeHeader(hdr)
}

// WriteHeader writes hdr and prepares to accept the file's contents.
// WriteHeader calls Flush if it is not the first header.
// Calling after a Close will return ErrWriteAfterClose.
func (aw *Writer) writeHeader(hdr *Header) error {
	if aw.closed {
		return ErrWriteAfterClose
	}
	if aw.err == nil {
		aw.Flush()
	}
	if aw.err != nil {
		return aw.err
	}
	fmodTimestamp := fmt.Sprintf("%d", hdr.ModTime.Unix())
	//use root by default (this is particularly useful for debs).
	uid := fmt.Sprintf("%d", hdr.Uid)
	if len(uid) > 6 {
		return fmt.Errorf("UID too long")
	}
	gid := fmt.Sprintf("%d", hdr.Gid)
	if len(gid) > 6 {
		return fmt.Errorf("GID too long")
	}
	//Files only atm (not dirs)
	mode := fmt.Sprintf("100%d", hdr.Mode)
	size := fmt.Sprintf("%d", hdr.Size)
	name := hdr.Name
	if aw.TerminateFilenamesSlash {
		name += "/"
	}
	line := fmt.Sprintf("%s%s%s%s%s%s`\n", pad(name, 16), pad(fmodTimestamp, 12), pad(gid, 6), pad(uid, 6), pad(mode, 8), pad(size, 10))
	if _, err := aw.Write([]byte(line)); err != nil {
		return err
	}
	// data section is 2-byte aligned.
	if hdr.Size%2 == 1 {
		aw.pad = true
	}
	aw.nb = hdr.Size
	return nil
}

// Write some data to the ar file.
func (aw *Writer) Write(b []byte) (int, error) {
	if aw.closed {
		aw.err = ErrWriteAfterClose
		return 0, aw.err
	}
	var n int
	n, aw.err = aw.w.Write(b)
	if aw.err != nil {
		return n, aw.err
	}
	aw.nb -= int64(n)
	return n, aw.err
}

// Close closes the ar archive, flushing any unwritten
// data to the underlying writer.
func (aw *Writer) Close() error {
	if aw.err != nil || aw.closed {
		return aw.err
	}
	aw.Flush()
	aw.closed = true
	if aw.err != nil {
		return aw.err
	}
	return aw.err
}

// pads a value with spaces up to a given length
func pad(value string, length int) string {
	plen := length - len(value)
	if plen > 0 {
		return value + strings.Repeat(" ", plen)
	}
	return value
}
