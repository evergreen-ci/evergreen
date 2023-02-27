package util

import (
	"io"
	"os"
	"path/filepath"
)

// WriteToTempFile writes the given string to a temporary file and returns the
// path to the file.
func WriteToTempFile(data string) (string, error) {
	dir := filepath.Join(os.TempDir(), "evergreen")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	file, err := os.CreateTemp(dir, "temp_file_")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err = io.WriteString(file, data); err != nil {
		return "", err
	}
	return file.Name(), nil
}
