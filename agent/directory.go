package agent

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
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
	newDir := filepath.Join(tc.taskConfig.Distro.WorkDir, dirName)

	tc.logger.Execution().Infof("Making new folder for task execution: %v", newDir)
	err = os.MkdirAll(newDir, 0777)
	if err != nil {
		tc.logger.Execution().Errorf("Error creating task directory: %v", err)
		return "", err
	}

	tc.taskConfig.WorkDir = newDir
	return newDir, nil
}

// removeTaskDirectory removes the folder the agent created for the task it
// was executing. It does not return an error because it is executed at the end of
// a task run, and the agent loop will start another task regardless of how this
// exits.
func (a *Agent) removeTaskDirectory(tc *taskContext) {
	if tc.taskDirectory == "" {
		grip.Critical("Task directory is not set")
		return
	}
	grip.Infof("Deleting directory for completed task: %s", tc.taskDirectory)

	if err := os.RemoveAll(tc.taskDirectory); err != nil {
		grip.Criticalf("Error removing working directory for the task: %v", err)
	}
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

		if strings.HasSuffix(path, ".git") {
			grip.Warning("don't run the agent in the development environment")
			return errors.New("skip cleanup in development environments")
		}

		if info.IsDir() {
			paths = append(paths, path)
		}

		return nil
	})

	if err != nil {
		return
	}

	for _, p := range paths {
		if err = os.RemoveAll(p); err != nil {
			grip.Notice(err)
		}
	}
}
