package command

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func dirExists(path string) bool {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}

	if !stat.IsDir() {
		return false
	}

	return true
}

func createEnclosingDirectoryIfNeeded(path string) error {
	localDir := filepath.Dir(path)

	if dirExists(path) {
		return nil
	}

	if err := os.MkdirAll(localDir, 0755); err != nil {
		return errors.Wrapf(err, "problem creating directory %s", localDir)
	}

	return nil
}
