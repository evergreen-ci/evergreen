// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ar

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"testing"
	"testing/iotest"
	"time"
)

type writerTestEntry struct {
	header   *Header
	contents string
}

type writerTest struct {
	file    string // filename of expected output
	entries []*writerTestEntry
}

var writerTests = []*writerTest{
	// The writer test file was produced with this command:
	// GNU ar (GNU Binutils for Ubuntu) 2.24
	// ar q writer.ar small.txt small2.txt
	{
		file: "testdata/writer.ar",
		entries: []*writerTestEntry{
			{
				header: &Header{
					Name:    "small.txt",
					Mode:    664,
					Uid:     1000,
					Gid:     1000,
					Size:    5,
					ModTime: time.Unix(1405990895, 0),
				},
				contents: "Kilts",
			},
			{
				header: &Header{
					Name:    "small2.txt",
					Mode:    664,
					Uid:     1000,
					Gid:     1000,
					Size:    11,
					ModTime: time.Unix(1405990895, 0),
				},
				contents: "Google.com\n",
			},
		},
	},
}

// Render byte array in a two-character hexadecimal string, spaced for easy visual inspection.
func bytestr(offset int, b []byte) string {
	const rowLen = 32
	s := fmt.Sprintf("%04x ", offset)
	for _, ch := range b {
		switch {
		case '0' <= ch && ch <= '9', 'A' <= ch && ch <= 'Z', 'a' <= ch && ch <= 'z':
			s += fmt.Sprintf("  %c", ch)
		default:
			s += fmt.Sprintf(" %02x", ch)
		}
	}
	return s
}

// Render a pseudo-diff between two blocks of bytes.
func bytediff(a []byte, b []byte) string {
	const rowLen = 32
	s := fmt.Sprintf("(%d bytes vs. %d bytes)\n", len(a), len(b))
	for offset := 0; len(a)+len(b) > 0; offset += rowLen {
		na, nb := rowLen, rowLen
		if na > len(a) {
			na = len(a)
		}
		if nb > len(b) {
			nb = len(b)
		}
		sa := bytestr(offset, a[0:na])
		sb := bytestr(offset, b[0:nb])
		if sa != sb {
			s += fmt.Sprintf("-%v\n+%v\n", sa, sb)
		}
		a = a[na:]
		b = b[nb:]
	}
	return s
}

func TestWriter(t *testing.T) {
testLoop:
	for i, test := range writerTests {
		expected, err := ioutil.ReadFile(test.file)
		if err != nil {
			t.Errorf("test %d: Unexpected error: %v", i, err)
			continue
		}

		buf := new(bytes.Buffer)
		tw := NewWriter(iotest.TruncateWriter(buf, 4<<10)) // only catch the first 4 KB
		tw.TerminateFilenamesSlash = true
		big := false
		for j, entry := range test.entries {
			big = big || entry.header.Size > 1<<10
			if err := tw.WriteHeader(entry.header); err != nil {
				t.Errorf("test %d, entry %d: Failed writing header: %v", i, j, err)
				continue testLoop
			}
			if _, err := io.WriteString(tw, entry.contents); err != nil {
				t.Errorf("test %d, entry %d: Failed writing contents: %v", i, j, err)
				continue testLoop
			}
		}
		// Only interested in Close failures for the small tests.
		if err := tw.Close(); err != nil && !big {
			t.Errorf("test %d: Failed closing archive: %v", i, err)
			continue testLoop
		}

		_, err = tw.Write([]byte{})
		if err != ErrWriteAfterClose {
			t.Errorf("test %d: ErrWriteAfterClosed not triggered: %v", i, err)
			continue testLoop
		}

		actual := buf.Bytes()
		if !bytes.Equal(expected, actual) {
			t.Errorf("test %d: Incorrect result: (-=expected, +=actual)\n%v",
				i, bytediff(expected, actual))
		}
		if testing.Short() { // The second test is expensive.
			break
		}
	}
}
