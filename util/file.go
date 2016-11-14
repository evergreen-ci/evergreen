package util

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	ignore "github.com/sabhiram/go-git-ignore"
)

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
	dir := filepath.Join(os.TempDir(), "evergreen")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	file, err := ioutil.TempFile(dir, "temp_file_")
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
