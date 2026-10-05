package command

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/pkg/errors"
)

// containToWorkdir errors if path resolves outside the task work directory on
// a container-isolated task.
func containToWorkdir(conf *internal.TaskConfig, desc, path string) error {
	if !conf.ContainerIsolationEnabled() {
		return nil
	}

	rel, err := filepath.Rel(filepath.Clean(conf.WorkDir), filepath.Clean(path))
	if err != nil {
		return errors.Wrapf(err, "%s '%s' is not comparable to the work directory", desc, path)
	}
	if pathEscapesRoot(rel) {
		return errors.Errorf("%s '%s' must stay inside the work directory for container-isolated tasks", desc, path)
	}
	return nil
}

// rootMkdirAll creates dir and any missing parents inside root.
func rootMkdirAll(root *os.Root, dir string) error {
	if dir == "." || dir == "" {
		return nil
	}
	accumulated := ""
	for component := range strings.SplitSeq(filepath.ToSlash(dir), "/") {
		if component == "" || component == "." {
			continue
		}
		accumulated = path.Join(accumulated, component)
		if err := root.Mkdir(accumulated, 0755); err != nil && !errors.Is(err, os.ErrExist) {
			return errors.Wrapf(err, "creating directory '%s'", accumulated)
		}
	}
	return nil
}

// verifyBoundedAncestors ensures every directory component of rel resolves
// inside root. Callers creating symlinks and hard links with the os package
// (which os.Root does not support) verify the destination parents first.
func verifyBoundedAncestors(root *os.Root, rel string) error {
	dir := filepath.Dir(rel)
	if dir == "." || dir == "" {
		return nil
	}
	accumulated := ""
	for component := range strings.SplitSeq(filepath.ToSlash(dir), "/") {
		if component == "" || component == "." {
			continue
		}
		accumulated = path.Join(accumulated, component)
		if _, err := root.Stat(accumulated); err != nil {
			return errors.Wrapf(err, "path component '%s' does not resolve inside the root", accumulated)
		}
	}
	return nil
}
