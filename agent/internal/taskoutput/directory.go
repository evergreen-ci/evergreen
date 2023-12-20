package taskoutput

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// Directory is the application representation of a task's reserved output
// directory. It coordinates asynchronous task output handling, from ingestion
// to persistence, while the task runs.
type Directory struct {
	rootDir  string
	handlers map[string]directoryHandler
}

// NewDirectory returns a new task output directory with the specifed root for
// the given task.
func NewDirectory(rootDir string, tsk *task.Task, logger client.LoggerProducer) *Directory {
	output := tsk.TaskOutputInfo
	taskOpts := taskoutput.TaskOptions{
		ProjectID: tsk.Project,
		TaskID:    tsk.Id,
		Execution: tsk.Execution,
	}
	d := &Directory{
		rootDir:  rootDir,
		handlers: map[string]directoryHandler{},
	}

	testLogDir := filepath.Join(rootDir, output.TestLogs.ID())
	d.handlers[testLogDir] = newTestLogDirectoryHandler(testLogDir, output.TestLogs, taskOpts, logger)

	return d
}

// Start creates the sub-directories and starts all asynchronous directory
// handlers.
func (a *Directory) Start(ctx context.Context) error {
	for dir := range a.handlers {
		if err := os.MkdirAll(dir, 0777); err != nil {
			return errors.Wrapf(err, "creating task output directory '%s'", dir)
		}
	}

	catcher := grip.NewBasicCatcher()
	for dir, handler := range a.handlers {
		catcher.Wrapf(handler.start(ctx), "starting task output directory handler for '%s'", dir)
	}

	return catcher.Resolve()
}

// Close closes all asynchronous directory handlers and removes the task output
// directory.
func (a *Directory) Close(ctx context.Context) error {
	catcher := grip.NewBasicCatcher()

	var wg sync.WaitGroup
	for dir, handler := range a.handlers {
		wg.Add(1)
		go func() {
			defer func() {
				catcher.Add(recovery.HandlePanicWithError(recover(), nil, "task output directory handler closer"))
				wg.Done()
			}()

			catcher.Wrapf(handler.close(ctx), "closing task output handler for '%s'", dir)
		}()
	}
	wg.Wait()

	catcher.Wrap(os.RemoveAll(a.rootDir), "removing task output directory")

	return catcher.Resolve()
}

type directoryHandler interface {
	start(context.Context) error
	close(context.Context) error
}
