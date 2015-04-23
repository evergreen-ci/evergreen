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
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// A Reader provides sequential access to the contents of an ar archive.
// An ar archive consists of a sequence of files.
// The Next method advances to the next file in the archive (including the first),
// and then it can be treated as an io.Reader to access the file's data.
type Reader struct {
	r   io.Reader
	err error
	nb  int64 // number of unread bytes for current file entry
	pad bool  // whether the file will be padded an extra byte (i.e. if ther's an odd number of bytes in the file)
}

// NewReader creates a new Reader reading from r.
// NewReader automatically reads in the ar file header, and checks it is valid.
func NewReader(r io.Reader) (*Reader, error) {
	ar := &Reader{r: r}
	arHeader := make([]byte, arHeaderSize)
	_, err := io.ReadFull(ar.r, arHeader)
	if err != nil {
		return nil, err
	}
	if string(arHeader) != ArFileHeader {
		return nil, errors.New("ar: Invalid ar file")
	}
	return ar, nil
}

// skipUnread skips any unread bytes in the existing file entry, as well as any alignment padding.
func (ar *Reader) skipUnread() {
	nr := ar.nb // number of bytes to skip
	if ar.pad {
		nr += int64(1)
		ar.pad = false
	}
	ar.nb = 0
	if sr, ok := ar.r.(io.Seeker); ok {
		if _, err := sr.Seek(nr, os.SEEK_CUR); err == nil {
			return
		}
	}

	_, ar.err = io.CopyN(ioutil.Discard, ar.r, nr)
}

// Next advances to the next entry in the ar archive.
func (ar *Reader) Next() (*Header, error) {
	var hdr *Header
	if ar.err == nil {
		ar.skipUnread()
	}
	if ar.err != nil {
		return hdr, ar.err
	}
	hdr = ar.readHeader()
	if hdr == nil {
		return hdr, ar.err
	}
	return hdr, ar.err
}

// NextString reads a string up to a given max length.
// This is useful for reading the first part of .a files.
func (ar *Reader) NextString(max int) (string, error) {
	firstLine := make([]byte, max)
	n, err := io.ReadFull(ar.r, firstLine)
	ar.nb -= int64(n)
	if err != nil {
		ar.err = err
		return "", err
	}
	return string(firstLine), nil
}

func (ar *Reader) readHeader() *Header {
	header := make([]byte, headerSize)
	if _, ar.err = io.ReadFull(ar.r, header); ar.err != nil {
		return nil
	}

	//TODO check end of archive

	// Unpack
	hdr := new(Header)
	s := slicer(header)

	hdr.Name = strings.TrimSpace(string(s.next(fileNameSize)))
	if strings.HasSuffix(hdr.Name, "/") {
		hdr.Name = hdr.Name[:len(hdr.Name)-1]
	}
	modTime, err := strconv.Atoi(strings.TrimSpace(string(s.next(modTimeSize))))
	if err != nil {
		log.Printf("Error: (%+v)", ar.err)
		log.Printf(" (Header: %+v)", hdr)
		return nil
	}
	hdr.ModTime = time.Unix(int64(modTime), int64(0))
	hdr.Uid, ar.err = strconv.Atoi(strings.TrimSpace(string(s.next(uidSize))))
	if ar.err != nil {
		log.Printf("Error: (%+v)", ar.err)
		log.Printf(" (Header: %+v)", hdr)
		return nil
	}
	hdr.Gid, err = strconv.Atoi(strings.TrimSpace(string(s.next(gidSize))))
	if ar.err != nil {
		log.Printf("Error: (%+v)", ar.err)
		log.Printf(" (Header: %+v)", hdr)
		return nil
	}
	modeStr := strings.TrimSpace(string(s.next(modeSize)))
	hdr.Mode, ar.err = strconv.ParseInt(modeStr, 10, 64)
	sizeStr := strings.TrimSpace(string(s.next(sizeSize)))
	hdr.Size, ar.err = strconv.ParseInt(sizeStr, 10, 64)
	if ar.err != nil {
		log.Printf("Error: (%+v)", ar.err)
		log.Printf(" (Header: %+v)", hdr)
		return nil
	}
	magic := s.next(2) // magic
	if magic[0] != 0x60 || magic[1] != 0x0a {
		log.Printf("Invalid magic Header (%x,%x)", int(magic[0]), int(magic[1]))
		log.Printf(" (Header: %+v)", hdr)
		ar.err = ErrHeader
		return nil
	}
	if ar.err != nil {
		log.Printf("Error: (%+v)", ar.err)
		log.Printf(" (Header: %+v)", hdr)
		return nil
	}

	ar.nb = hdr.Size
	if math.Mod(float64(hdr.Size), float64(2)) == float64(1) {
		ar.pad = true
	} else {
		ar.pad = false
	}
	return hdr
}

// Read reads from the current entry in the ar archive.
// It returns 0, io.EOF when it reaches the end of that entry,
// until Next is called to advance to the next entry.
func (ar *Reader) Read(b []byte) (n int, err error) {
	if ar.nb == 0 {
		// file consumed
		return 0, io.EOF
	}

	if int64(len(b)) > ar.nb {
		b = b[0:ar.nb]
	}
	n, err = ar.r.Read(b)
	ar.nb -= int64(n)

	if err == io.EOF && ar.nb > 0 {
		err = io.ErrUnexpectedEOF
	}
	ar.err = err
	return
}
