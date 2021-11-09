package utility

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
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

// FileMatcher represents a type that can match against files and file
// information to determine if it should be included.
type FileMatcher interface {
	Match(file string, info os.FileInfo) bool
}

// AlwaysMatch provides a FileMatcher implementation that will always match a
// file or directory.
type AlwaysMatch struct{}

// Match always returns true.
func (m AlwaysMatch) Match(string, os.FileInfo) bool { return true }

// FileFileAlwaysMatch provides a FileMatcher implementation that will always
// match a file, but not a directory.
type FileAlwaysMatch struct{}

func (m FileAlwaysMatch) Match(_ string, info os.FileInfo) bool {
	return !info.IsDir()
}

// NeverMatch provides a FileMatcher implementation that will never match a
// file or directory.
type NeverMatch struct{}

// Match always returns false.
func (m NeverMatch) Match(string, os.FileInfo) bool { return false }

// FileListBuilder provides options to find files within a directory.
type FileListBuilder struct {
	// Include determines which files should be included. This has lower
	// precedence than the Exclude filter.
	Include FileMatcher
	// Exclude determines which files should be excluded from the file list.
	// This has higher precedence than the Include filter.
	Exclude FileMatcher
	// WorkingDir is the base working directory from which to start searching
	// for files.
	WorkingDir string
	files      []string
}

// NewFileListBuilder returns a default FileListBuilder that will match all
// files (but not directory names) within the given directory dir.
func NewFileListBuilder(dir string) *FileListBuilder {
	return &FileListBuilder{
		Include:    FileAlwaysMatch{},
		Exclude:    NeverMatch{},
		WorkingDir: dir,
	}
}

// Build finds all files that pass the include and exclude filters within the
// working directory. It does not follow symlinks. All files returned are
// relative to the working directory.
func (b *FileListBuilder) Build() ([]string, error) {
	if b.Include == nil {
		return nil, errors.New("cannot build file list without an include filter")
	}
	if err := filepath.Walk(b.WorkingDir, b.visitPath); err != nil {
		return nil, errors.Wrap(err, "building file list")
	}

	return b.files, nil
}

func (b *FileListBuilder) visitPath(path string, info os.FileInfo, err error) error {
	if err != nil {
		return errors.Wrapf(err, "path '%s'", path)
	}

	if b.Exclude != nil && b.Exclude.Match(path, info) {
		return nil
	}

	if b.Include == nil || !b.Include.Match(path, info) {
		return nil
	}

	// All accumulated paths are relative to the working directory.
	toAdd := strings.TrimPrefix(strings.TrimPrefix(path, b.WorkingDir), string(os.PathSeparator))
	b.files = append(b.files, toAdd)

	return nil
}
