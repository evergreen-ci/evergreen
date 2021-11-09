// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ar

import (
	"bytes"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

/*8
const (
	c_ISUID  = 04000   // Set uid
	c_ISGID  = 02000   // Set gid
	c_ISVTX  = 01000   // Save text (sticky bit)
	c_ISDIR  = 040000  // Directory
	c_ISFIFO = 010000  // FIFO
	c_ISREG  = 0100000 // Regular file
	c_ISLNK  = 0120000 // Symbolic link
	c_ISBLK  = 060000  // Block special file
	c_ISCHR  = 020000  // Character special file
	c_ISSOCK = 0140000 // Socket
)
*/
func TestFileInfoHeader(t *testing.T) {
	fi, err := os.Stat("testdata/small.txt")
	if err != nil {
		t.Fatal(err)
	}
	h, err := FileInfoHeader(fi)
	if err != nil {
		t.Fatalf("FileInfoHeader: %v", err)
	}
	if g, e := h.Name, "small.txt"; g != e {
		t.Errorf("Name = %q; want %q", g, e)
	}
	if g, e := h.Mode, int64(fi.Mode().Perm()); g != e {
		t.Errorf("Mode = %#o; want %#o", g, e)
	}
	if g, e := h.Size, int64(5); g != e {
		t.Errorf("Size = %v; want %v", g, e)
	}
	if g, e := h.ModTime, fi.ModTime(); !g.Equal(e) {
		t.Errorf("ModTime = %v; want %v", g, e)
	}
	// FileInfoHeader should error when passing nil FileInfo
	if _, err := FileInfoHeader(nil); err == nil {
		t.Fatalf("Expected error when passing nil to FileInfoHeader")
	}
}

func TestFileInfoHeaderSymlink(t *testing.T) {
	h, err := FileInfoHeader(symlink{})
	if err != nil {
		t.Fatal(err)
	}
	if g, e := h.Name, "some-symlink"; g != e {
		t.Errorf("Name = %q; want %q", g, e)
	}
}

type symlink struct{}

func (symlink) Name() string       { return "some-symlink" }
func (symlink) Size() int64        { return 0 }
func (symlink) Mode() os.FileMode  { return os.ModeSymlink }
func (symlink) ModTime() time.Time { return time.Time{} }
func (symlink) IsDir() bool        { return false }
func (symlink) Sys() interface{}   { return nil }

func TestRoundTrip(t *testing.T) {
	data := []byte("some file contents")

	var b bytes.Buffer
	tw := NewWriter(&b)
	hdr := &Header{
		Name:    "file.txt",
		Uid:     1 << 21, // too big for 6 octal digits
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	// ar only supports second precision.
	hdr.ModTime = hdr.ModTime.Add(-time.Duration(hdr.ModTime.Nanosecond()) * time.Nanosecond)
	err := tw.WriteHeader(hdr)
	if err == nil {
		t.Fatalf("tw.WriteHeader should return an error here")
	}
}

type headerRoundTripTest struct {
	h  *Header
	fm os.FileMode
}

func TestHeaderRoundTrip(t *testing.T) {
	golden := []headerRoundTripTest{
		// regular file.
		{
			h: &Header{
				Name:    "test.txt",
				Mode:    0644,
				Size:    12,
				ModTime: time.Unix(1360600916, 0),
				//	Typeflag: TypeReg,
			},
			fm: 0644,
		},
	}

	for i, g := range golden {
		fi := g.h.FileInfo()
		h2, err := FileInfoHeader(fi)
		if err != nil {
			t.Error(err)
			continue
		}
		if strings.Contains(fi.Name(), "/") {
			t.Errorf("FileInfo of %q contains slash: %q", g.h.Name, fi.Name())
		}
		name := path.Base(g.h.Name)
		if fi.IsDir() {
			name += "/"
		}
		if got, want := h2.Name, name; got != want {
			t.Errorf("i=%d: Name: got %v, want %v", i, got, want)
		}
		if got, want := h2.Size, g.h.Size; got != want {
			t.Errorf("i=%d: Size: got %v, want %v", i, got, want)
		}
		if got, want := h2.Mode, g.h.Mode; got != want {
			t.Errorf("i=%d: Name:%s Mode: got %o, want %o", i, h2.Name, got, want)
		}
		if got, want := fi.Mode(), g.fm; got != want {
			t.Errorf("i=%d: fi.Mode: got %o, want %o", i, got, want)
		}
		if got, want := h2.ModTime, g.h.ModTime; got != want {
			t.Errorf("i=%d: ModTime: got %v, want %v", i, got, want)
		}
		if sysh, ok := fi.Sys().(*Header); !ok || sysh != g.h {
			t.Errorf("i=%d: Sys didn't return original *Header", i)
		}
	}
}
