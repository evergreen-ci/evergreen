package model

import (
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
