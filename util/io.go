package util

import "io"

type SizeTrackingReader struct {
	BytesRead uint64
	io.Reader
}

func (str *SizeTrackingReader) Read(p []byte) (int, error) {
	n, err := str.Reader.Read(p)
	str.BytesRead += uint64(n)
	return n, err
}
