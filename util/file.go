package util

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	ignore "github.com/sabhiram/go-git-ignore"
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

// fileListBuilder contains the information for building a list of files in the given directory.
// It adds the files to include in the fileNames array and uses the ignorer to determine if a given
// file matches and should be added.
type fileListBuilder struct {
	fileNames []string
	ignorer   *ignore.GitIgnore
}

func (fb *fileListBuilder) walkFunc(path string, info os.FileInfo, err error) error {
	if !info.IsDir() && fb.ignorer.MatchesPath(path) {
		fb.fileNames = append(fb.fileNames, path)
	}
	return nil
}

// BuildFileList returns a list of files that match the given list of expressions
// rooted at the given startPath. The expressions correspond to gitignore ignore
// expressions: anything that would be matched - and therefore ignored by git - is included
// in the returned list of file paths. BuildFileList does not follow symlinks as
// it uses filpath.Walk, which does not follow symlinks.
func BuildFileList(startPath string, expressions ...string) ([]string, error) {
	ignorer, err := ignore.CompileIgnoreLines(expressions...)
	if err != nil {
		return nil, err
	}
	fb := &fileListBuilder{
		fileNames: []string{},
		ignorer:   ignorer,
	}
	err = filepath.Walk(startPath, fb.walkFunc)
	if err != nil {
		return nil, err
	}
	return fb.fileNames, nil
}
