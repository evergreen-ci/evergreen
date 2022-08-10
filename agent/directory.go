package agent

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// createTaskDirectory makes a directory for the agent to execute
// the current task within. It changes the necessary variables
// so that all of the agent's operations will use this folder.
func (a *Agent) createTaskDirectory(tc *taskContext) (string, error) {
	h := md5.New()

	_, err := h.Write([]byte(
		fmt.Sprintf("%s_%d_%d", tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution, os.Getpid())))
	if err != nil {
		tc.logger.Execution().Errorf("Error creating task directory name: %v", err)
		return "", err
	}

	dirName := hex.EncodeToString(h.Sum(nil))
	newDir := filepath.Join(a.opts.WorkingDirectory, dirName)

	tc.logger.Execution().Infof("Making new folder for task execution: %v", newDir)

	if err = os.MkdirAll(newDir, 0777); err != nil {
		tc.logger.Execution().Errorf("Error creating task directory: %v", err)
		return "", err
	}

	if err = os.MkdirAll(filepath.Join(newDir, "tmp"), 0777); err != nil {
		tc.logger.Execution().Warning(message.WrapError(err, "problem creating task temporary, continuing"))
	}

	return newDir, nil
}

// removeTaskDirectory removes the folder the agent created for the task it
// was executing. It does not return an error because it is executed at the end of
// a task run, and the agent loop will start another task regardless of how this
// exits.
func (a *Agent) removeTaskDirectory(tc *taskContext) {
	if tc.taskDirectory == "" {
		grip.Info("Task directory is not set, not removing")
		return
	}
	grip.Infof("Deleting directory for completed task: %s", tc.taskDirectory)

	// Removing long relative paths hangs on Windows https://github.com/golang/go/issues/36375,
	// so we have to convert to an absolute path before removing.
	abs, err := filepath.Abs(tc.taskDirectory)
	if err != nil {
		grip.Critical(errors.Wrap(err, "Problem getting absolute directory for task directory"))
		return
	}
	err = a.removeAll(abs)
	grip.Critical(errors.Wrapf(err, "Error removing working directory for the task: %s", tc.taskDirectory))
	grip.InfoWhen(err == nil, message.Fields{
		"message":   "Successfully deleted directory for completed task",
		"directory": tc.taskDirectory,
	})
}

// removeAll is the same as os.RemoveAll, but recursively changes permissions for subdirectories and contents before removing
func (a *Agent) removeAll(dir string) error {
	grip.Error(filepath.Walk(dir, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		grip.Error(os.Chmod(path, 0777))
		return nil
	}))
	return os.RemoveAll(dir)
}

// tryCleanupDirectory is a very conservative function that attempts
// to cleanup the working directory when the agent starts. Without
// this function, if an agent attempts to start on a system where a
// previous agent has run but crashed for some reason, the new agent
// could easily run out of disk space and will have no way to clean up
// for itself, which leads to an increased maintenance burden for
// static, and other long running hosts.
//
// By conservative, the operation does not return an error or attempt
// to retry in the case of an error, so running this function does not
// ensure that any files are necessarily removed, but the hope is that
// its better than not doing anything.
//
// Additionally the function does *not* handle log rotation or
// management, and only attempts to clean up the agent's working
// directory, so files not located in a directory may still cause
// issues.
func tryCleanupDirectory(dir string) {
	defer recovery.LogStackTraceAndContinue("clean up directories")

	if dir == "" {
		return
	}

	stat, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return
	}

	if !stat.IsDir() {
		return
	}

	// Don't run in a development environment
	if _, err = os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		grip.Notice("refusing to clean a directory that contains '.git'")
		return
	}

	usr, err := user.Current()
	if err != nil {
		grip.Warning(err)
		return
	}

	if strings.HasPrefix(dir, usr.HomeDir) || strings.Contains(dir, "cygwin") {
		grip.Notice("not cleaning up directory, because it is in the home directory.")
		return
	}

	paths := []string{}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == dir {
			return nil
		}

		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		if info.IsDir() {
			paths = append(paths, path)
		}

		return nil
	})

	if err != nil {
		return
	}

	grip.Infof("attempting to clean up directory '%s'", dir)
	for _, p := range paths {
		if err = os.RemoveAll(p); err != nil {
			grip.Notice(err)
		}
	}
}
