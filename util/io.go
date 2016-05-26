package util

import (
	"bytes"
	"io"
	"io/ioutil"
)

type SizeTrackingReader struct {
	BytesRead uint64
	io.Reader
}

func (str *SizeTrackingReader) Read(p []byte) (int, error) {
	n, err := str.Reader.Read(p)
	str.BytesRead += uint64(n)
	return n, err
}

// WriteTempFile creates a temp file, writes the data to it, closes it and returns the file name.
func WriteTempFile(prefix string, data []byte) (string, error) {
	file, err := ioutil.TempFile("", prefix)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(file, bytes.NewReader(data))
	if err != nil {
		return file.Name(), err
	}
	return file.Name(), nil
}
