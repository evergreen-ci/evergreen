package util

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// CurrentGitHash returns the current git hash of the git repository
// located at the specified directory.
func CurrentGitHash(repoDir string) (string, error) {

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	defer stdout.Close()
	cmd.Start()

	var data []byte = make([]byte, 1024)
	_, err = stdout.Read(data)
	if err != nil {
		return "", err
	}

	if err := cmd.Wait(); err != nil {
		return "", err
	}

	line := strings.SplitN(string(data), "\n", 2)[0]

	if len(line) != 40 { // a SHA1 in hex is 40 bytes
		return "", fmt.Errorf("bad rev-parse output: %v (%d != 40)", line, len(line))
	}
	return line, nil
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
