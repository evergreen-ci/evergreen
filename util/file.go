package util

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// GetAppendingFile opens a file for appending. The file will be created
// if it does not already exist.
func GetAppendingFile(path string) (*os.File, error) {
	exists, err := FileExists(path)
	if err != nil {
		return nil, err
	}
	if !exists {
		if _, err := os.Create(path); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
}

// FileExists returns true if 'path' exists.
func FileExists(elem ...string) (bool, error) {
	path := filepath.Join(elem...)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// WriteToTempFile writes the given string to a temporary file and returns the
// path to the file.
func WriteToTempFile(data string) (string, error) {
	file, err := ioutil.TempFile("", "temp_file_")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err = io.WriteString(file, data); err != nil {
		return "", err
	}
	return file.Name(), nil
}
