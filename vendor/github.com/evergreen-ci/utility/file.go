package utility

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	ignore "github.com/sabhiram/go-git-ignore"
)

// FileExists provides a clearer interface for checking if a file
// exists.
func FileExists(path string) bool {
	if path == "" {
		return false
	}

	_, err := os.Stat(path)

	return !os.IsNotExist(err)
}

// WriteRawFile writes a sequence of byes to a new file created at the
// specified path.
func WriteRawFile(path string, data []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "problem creating file '%s'", path)
	}

	n, err := file.Write(data)
	if err != nil {
		grip.Warning(message.WrapError(errors.WithStack(file.Close()),
			message.Fields{
				"message":       "problem closing file after error",
				"path":          path,
				"bytes_written": n,
				"input_len":     len(data),
			}))

		return errors.Wrapf(err, "problem writing data to file '%s'", path)
	}

	if err = file.Close(); err != nil {
		return errors.Wrapf(err, "problem closing file '%s' after successfully writing data", path)
	}

	return nil
}

// WriteFile provides a clearer interface for writing string data to a
// file.
func WriteFile(path string, data string) error {
	return errors.WithStack(WriteRawFile(path, []byte(data)))
}

// fileListBuilder contains the information for building a list of files in the given directory.
// It adds the files to include in the fileNames array and uses the ignorer to determine if a given
// file matches and should be added.
type fileListBuilder struct {
	fileNames []string
	ignorer   *ignore.GitIgnore
	prefix    string
}

func (fb *fileListBuilder) walkFunc(path string, info os.FileInfo, err error) error {
	if err != nil {
		return errors.Wrapf(err, "Error received by walkFunc for path %s", path)
	}
	path = strings.TrimPrefix(path, fb.prefix)
	path = strings.TrimLeft(path, string(os.PathSeparator))
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
		prefix:    startPath,
	}
	err = filepath.Walk(startPath, fb.walkFunc)
	if err != nil {
		return nil, err
	}
	return fb.fileNames, nil
}
