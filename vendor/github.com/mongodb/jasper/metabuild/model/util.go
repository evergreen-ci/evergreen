package model

import (
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
)

// withMatchingFiles performs the given operation op on all files that match the
// gitignore-style globbing patterns within the working directory workDir.
func withMatchingFiles(workDir string, patterns []string, op func(file string) error) error {
	include, err := utility.NewGitignoreFileMatcher(workDir, patterns...)
	if err != nil {
		return errors.Wrap(err, "building gitignore file-matching patterns")
	}
	b := utility.FileListBuilder{
		Include:    include,
		WorkingDir: workDir,
	}
	files, err := b.Build()
	if err != nil {
		return errors.Wrap(err, "evaluating file patterns")
	}
	for _, file := range files {
		if err := op(util.ConsistentFilepath(workDir, file)); err != nil {
			return errors.Wrapf(err, "file '%s'", file)
		}
	}

	return nil
}

var errNotRelativeToWorkingDir = errors.New("converting to path relative to working directory")

// relPathToWorkingDir attempts to make the given path relative to the working
// directory if possible. If the path is already non-absolute, it is returned
// unchanged and assumed to be relative to the working directory. If it is
// absolute and can be successfully converted to a relative path, it returns the
// relative path and nil error. Otherwise, if path is absolute and cannot be
// made relative to the working directory, it returns the path and
// errNotRelativeToWorkingDir.
func relToPath(path, workingDir string) (string, error) {
	workingDir = util.ConsistentFilepath(workingDir)
	path = util.ConsistentFilepath(path)

	if !filepath.IsAbs(path) {
		return path, nil
	}

	if workingDir == "" || !strings.HasPrefix(path, workingDir) {
		return path, errNotRelativeToWorkingDir
	}

	relPath, err := filepath.Rel(workingDir, path)
	if err != nil {
		return path, errors.Wrap(err, errNotRelativeToWorkingDir.Error())
	}

	return util.ConsistentFilepath(relPath), nil
}
