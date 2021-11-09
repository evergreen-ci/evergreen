// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ar

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type unarTest struct {
	file    string
	headers []*Header
	cksums  []string
}

var simpleArTest = &unarTest{
	file: "testdata/common.ar",
	headers: []*Header{
		{
			Name:    "small.txt",
			Mode:    100664,
			Uid:     1000,
			Gid:     1000,
			Size:    5,
			ModTime: time.Unix(1405990895, 0),
		},
		{
			Name:    "small2.txt",
			Mode:    100664,
			Uid:     1000,
			Gid:     1000,
			Size:    11,
			ModTime: time.Unix(1405990895, 0),
		},
	},
	cksums: []string{
		"e38b27eaccb4391bdec553a7f3ae6b2f",
		"c65bd2e50a56a2138bf1716f2fd56fe9",
	},
}
var unarTests = []*unarTest{
	simpleArTest,
}

func TestNextString(t *testing.T) {
testLoop:
	for i, test := range unarTests {
		f, err := os.Open(test.file)
		if err != nil {
			t.Errorf("test %d: Unexpected error: %v", i, err)
			continue
		}
		defer f.Close()
		tr, err := NewReader(f)
		if err != nil {
			t.Errorf("test %d: Error checking file header: %v", i, err)
			continue
		}
		for j := range test.headers {
			hdr, err := tr.Next()
			if err != nil || hdr == nil {
				t.Errorf("test %d, entry %d: Didn't get entry: %v", i, j, err)
				f.Close()
				continue testLoop
			}
			if hdr.Size >= 6 {
				str, err := tr.NextString(6)
				if err != nil || len(str) != 6 {
					t.Errorf("test %d, entry %d: Didn't get string: %s, %v", i, j, str, err)
					f.Close()
					continue testLoop
				}
				t.Logf("read first 6 bytes: %s", str)
			}
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			continue testLoop
		}
		if hdr != nil || err != nil {
			t.Errorf("test %d: Unexpected entry or error: hdr=%v err=%v", i, hdr, err)
		}
	}
}

func TestReader(t *testing.T) {
testLoop:
	for i, test := range unarTests {
		f, err := os.Open(test.file)
		if err != nil {
			t.Errorf("test %d: Unexpected error: %v", i, err)
			continue
		}
		defer f.Close()
		tr, err := NewReader(f)
		if err != nil {
			t.Errorf("test %d: Error checking file header: %v", i, err)
			continue
		}
		for j, header := range test.headers {
			hdr, err := tr.Next()
			if err != nil || hdr == nil {
				t.Errorf("test %d, entry %d: Didn't get entry: %v", i, j, err)
				f.Close()
				continue testLoop
			}
			if !reflect.DeepEqual(*hdr, *header) {
				t.Errorf("test %d, entry %d: Incorrect header:\nhave %+v\nwant %+v",
					i, j, *hdr, *header)
			}
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			continue testLoop
		}
		if hdr != nil || err != nil {
			t.Errorf("test %d: Unexpected entry or error: hdr=%v err=%v", i, hdr, err)
		}
	}
}

func TestPartialRead(t *testing.T) {
	f, err := os.Open("testdata/common.ar")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer f.Close()

	tr, err := NewReader(f)
	if err != nil || tr == nil {
		t.Fatalf("Didn't get ar header: %v", err)
	}

	// Read the first four bytes; Next() should skip the last byte.
	hdr, err := tr.Next()
	if err != nil || hdr == nil {
		t.Fatalf("Didn't get first file: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(tr, buf); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if expected := []byte("Kilt"); !bytes.Equal(buf, expected) {
		t.Errorf("Contents = %v, want %v", buf, expected)
	}

	// Second file
	hdr, err = tr.Next()
	if err != nil || hdr == nil {
		t.Fatalf("Didn't get second file: %v", err)
	}
	buf = make([]byte, 6)
	if _, err := io.ReadFull(tr, buf); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if expected := []byte("Google"); !bytes.Equal(buf, expected) {
		t.Errorf("Contents = %v, want %v", buf, expected)
	}
}

func TestIncrementalRead(t *testing.T) {
	test := simpleArTest
	f, err := os.Open(test.file)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer f.Close()

	tr, err := NewReader(f)
	if err != nil {
		t.Fatalf("Unexpected error reading ar header: %v", err)
	}

	headers := test.headers
	cksums := test.cksums
	nread := 0

	// loop over all files
	for ; ; nread++ {
		hdr, err := tr.Next()
		if hdr == nil || err == io.EOF {
			break
		}

		// check the header
		if !reflect.DeepEqual(*hdr, *headers[nread]) {
			t.Errorf("Incorrect header:\nhave %+v\nwant %+v",
				hdr, headers[nread])
		}

		// read file contents in little chunks EOF,
		// checksumming all the way
		h := md5.New()
		rdbuf := make([]uint8, 8)
		for {
			nr, err := tr.Read(rdbuf)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("Read: unexpected error %v\n", err)
				break
			}
			h.Write(rdbuf[0:nr])
		}
		// verify checksum
		have := fmt.Sprintf("%x", h.Sum(nil))
		want := cksums[nread]
		if want != have {
			t.Errorf("Bad checksum on file %s:\nhave %+v\nwant %+v", hdr.Name, have, want)
		}
	}
	if nread != len(headers) {
		t.Errorf("Didn't process all files\nexpected: %d\nprocessed %d\n", len(headers), nread)
	}
}

func TestNonSeekable(t *testing.T) {
	test := simpleArTest
	f, err := os.Open(test.file)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer f.Close()

	type readerOnly struct {
		io.Reader
	}
	tr, err := NewReader(readerOnly{f})
	if err != nil {
		t.Fatalf("NewReader error: %v", err)
	}
	nread := 0

	for ; ; nread++ {
		_, err = tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}

	if nread != len(test.headers) {
		t.Errorf("Didn't process all files\nexpected: %d\nprocessed %d\n", len(test.headers), nread)
	}
}

func TestUninitializedRead(t *testing.T) {
	test := simpleArTest
	f, err := os.Open(test.file)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer f.Close()

	tr, err := NewReader(f)
	if err != nil {
		t.Fatalf("NewReader error: %v", err)
	}
	_, err = tr.Read([]byte{})
	if err == nil || err != io.EOF {
		t.Errorf("Unexpected error: %v, wanted %v", err, io.EOF)
	}

}

type myCloser struct {
	Reader io.Reader
	closed bool
}

func (t myCloser) Read(p []byte) (int, error) {
	if t.closed {
		return -1, io.EOF
	}
	return t.Reader.Read(p)
}

func (t myCloser) Close() error {
	t.closed = true
	return nil
}

func TestNoFooter(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "nofooter.ar"))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer f.Close()

	tr, err := NewReader(f)
	if err != nil {
		t.Fatalf("NewReader error: %v", err)
	}
	_, err = tr.Read([]byte{})
	if err == nil || err != io.EOF {
		t.Errorf("Unexpected error: %v, wanted %v", err, io.EOF)
	}
	nread := 0

	// loop over all files
	for ; ; nread++ {
		hdr, err := tr.Next()
		if hdr == nil || err == io.EOF {
			break
		}
		_, err = io.Copy(ioutil.Discard, tr)
		if nread == 1 {
			if err == nil {
				t.Fatalf("Read should have produced an error: %v", err)
			} else {
				t.Logf("Correctly produced an error: %v", err)
			}
		} else {
			if err != nil {
				t.Fatalf("Unexpected read error: %v", err)
			}
		}
	}

}
func TestClosedReaderNextString(t *testing.T) {
	sr := strings.NewReader("!<arch>\nblah")
	r := myCloser{sr, false}
	tr, err := NewReader(r)
	if err != nil {
		t.Errorf("Unexpected error returned by NewReader: %v", err)
	}
	r.Close()
	str, err := tr.NextString(8)
	if err == nil {
		t.Errorf("No error returned by NextString: %s / %v", str, err)
	}
}

func TestClosedReader(t *testing.T) {
	r, _ := io.Pipe()
	r.Close()
	_, err := NewReader(r)
	if err == nil {
		t.Errorf("No error returned by NewReader: %v", err)
	}
}

func TestInvalidArHeader(t *testing.T) {
	r := strings.NewReader("not an ar file")
	_, err := NewReader(r)
	if err == nil {
		t.Errorf("No error returned by NewReader: %v", err)
	}
}
