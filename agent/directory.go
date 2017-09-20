package agent

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
)

// createTaskDirectory makes a directory for the agent to execute
// the current task within. It changes the necessary variables
// so that all of the agent's operations will use this folder.
func (a *Agent) createTaskDirectory(tc *taskContext, taskConfig *model.TaskConfig) (string, error) {
	h := md5.New()

	_, err := h.Write([]byte(
		fmt.Sprintf("%s_%d_%d", taskConfig.Task.Id, taskConfig.Task.Execution, os.Getpid())))
	if err != nil {
		tc.logger.Execution().Errorf("Error creating task directory name: %v", err)
		return "", err
	}

	dirName := hex.EncodeToString(h.Sum(nil))
	newDir := filepath.Join(taskConfig.Distro.WorkDir, dirName)

	tc.logger.Execution().Infof("Making new folder for task execution: %v", newDir)
	err = os.Mkdir(newDir, 0777)
	if err != nil {
		tc.logger.Execution().Errorf("Error creating task directory: %v", err)
		return "", err
	}

	taskConfig.WorkDir = newDir
	return newDir, nil
}

// removeTaskDirectory removes the folder the agent created for the task it
// was executing. It does not return an error because it is executed at the end of
// a task run, and the agent loop will start another task regardless of how this
// exits.
func (a *Agent) removeTaskDirectory(tc *taskContext) {
	if tc.taskDirectory == "" {
		grip.Critical("Task directory is not set")
	}
	if tc.taskConfig == nil {
		grip.Critical("No taskConfig in taskContext")
		return
	}
	grip.Info("Deleting directory for completed task.")

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
	defer func() {
		m := recover()
		grip.Warning(m)
	}()

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

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == dir {
			return nil
		}

		if strings.HasSuffix(path, ".git") {
			grip.Warning("don't run the agent in the development environment")
			return errors.New("skip cleanup in development environments")
		}

		if info.IsDir() {
			if err = os.RemoveAll(path); err != nil {
				grip.Notice(err)
				return nil
			}
		}

		return nil
	})
}
