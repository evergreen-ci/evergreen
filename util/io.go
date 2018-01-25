package util

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
)

// WriteToFile takes in a body and filepath and writes out the data in the body
func WriteToFile(body io.ReadCloser, filepath string) error {
	defer body.Close()
	if filepath == "" {
		return errors.New("cannot write output to an unspecified ")
	}

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, body)
	return err
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
