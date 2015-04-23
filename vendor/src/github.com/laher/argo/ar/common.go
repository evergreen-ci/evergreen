// Copyright 2013 Am Laher.
// This code is adapted from code within the Go tree.
// See Go's licence information below:
//
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ar implements access to ar archives.
// argo only implements the 'common' format as used for .deb files, by GNU ar, and by BSD ar. AIX and Coherent variants are not supported.
// Note that argo is not currently supporting the workarounds for long filenames as defined by GNU ar or BSD ar. Please get in touch if you require this feature.
//
// References:
//   http://en.wikipedia.org/wiki/Ar_(Unix)
//   http://linux.die.net/man/1/ar
//   http://www.freebsd.org/cgi/man.cgi?query=ar&sektion=5&apropos=0&manpath=FreeBSD+9.3-RELEASE
package ar

import (
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	// the size of a file header
	headerSize = 60
	// the length of an 'ar' archive header
	arHeaderSize = 8
	// the length of the filename header field
	fileNameSize = 16
	// the length of the modTime header field
	modTimeSize = 12
	// the length of the UID header field
	uidSize = 6
	// the length of the GID header field
	gidSize = 6
	// the length of the Mode header field
	modeSize = 8
	// the length of the Size header field
	sizeSize = 10
	// the length of the 'magic' number
	magicSize = 2
)

var (
	zeroBlock = make([]byte, headerSize)
	// ErrHeader describes an invalid ar file header
	ErrHeader = errors.New("ar: invalid ar header")
	// ArFileHeader is the string used to identify an .ar file
	ArFileHeader = "!<arch>\n"
)

/*
Sample ar data showing file entries:
!<arch>
debian-binary   1282478016  0     0     100644  4         `
2.0
control.tar.gz  1282478016  0     0     100644  444       `
.....binary-data.....
*/

// A Header represents a single header in an ar archive.
// Some fields may not be populated.
type Header struct {
	// Name is the name of the file.
	// It must be a relative path: it must not start with a drive
	// letter (e.g. C:) or leading slash, and only forward slashes
	// are allowed.
	Name    string    // name of header file entry
	ModTime time.Time // modified time
	Uid     int       // user id of owner
	Gid     int       // group id of owner
	Mode    int64     // permission and mode bits
	Size    int64     // length in bytes
}

type slicer []byte

func (sp *slicer) next(n int) (b []byte) {
	s := *sp
	b, *sp = s[0:n], s[n:]
	return
}

// FileInfo returns an os.FileInfo for the Header.
func (h *Header) FileInfo() os.FileInfo {
	return headerFileInfo{h}
}

// headerFileInfo implements os.FileInfo.
type headerFileInfo struct {
	h *Header
}

func (fi headerFileInfo) Size() int64        { return fi.h.Size }
func (fi headerFileInfo) IsDir() bool        { return fi.Mode().IsDir() }
func (fi headerFileInfo) ModTime() time.Time { return fi.h.ModTime }
func (fi headerFileInfo) Sys() interface{}   { return fi.h }
func (fi headerFileInfo) Name() string       { return fi.h.Name }

// Mode returns the permission and mode bits for the headerFileInfo.
func (fi headerFileInfo) Mode() (mode os.FileMode) {
	// Set file permission bits.
	mode = os.FileMode(fi.h.Mode).Perm()
	return mode
}

// FileInfoHeader creates a partially-populated Header from an
// os.FileInfo.
// Because os.FileInfo's Name method returns only the base name of
// the file it describes, it may be necessary to modify the Name field
// of the returned header to provide the full path name of the file.
func FileInfoHeader(fi os.FileInfo) (*Header, error) {
	if fi == nil {
		return nil, fmt.Errorf("File header is nil")
	}
	size := fi.Size()
	h := &Header{
		Name: fi.Name(),
		Size: int64(size),
	}
	fm := fi.Mode()
	h.ModTime = fi.ModTime()
	h.Mode = int64(fm.Perm())

	return h, nil
}
