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
)

// Directory is the application representation of a task's reserved output
// directory. It coordinates asynchronous task output handling, from ingestion
// to persistence, while the task runs.
type Directory struct {
	root     string
	handlers map[string]directoryHandler
}

// NewDirectory returns a new task output directory with the specifed root for
// the given task.
func NewDirectory(root string, tsk *task.Task, logger client.LoggerProducer) *Directory {
	output := tsk.TaskOutputInfo
	taskOpts := taskoutput.TaskOptions{
		ProjectID: tsk.Project,
		TaskID:    tsk.Id,
		Execution: tsk.Execution,
	}

	return &Directory{
		root: root,
		handlers: map[string]directoryHandler{
			output.TestLogs.ID(): newTestLogDirectoryHandler(output.TestLogs, taskOpts, logger),
		},
	}
}

// Start creates the sub-directories and starts all asynchronous directory
// handlers.
func (a *Directory) Start(ctx context.Context) error {
	catcher := grip.NewBasicCatcher()
	for id, handler := range a.handlers {
		subDir := filepath.Join(a.root, id)
		if err := os.MkdirAll(subDir, 0777); err != nil {
			catcher.Wrapf(err, "creating task output directory '%s'", subDir)
		} else {
			catcher.Wrapf(handler.start(ctx, subDir), "starting task output directory handler for '%s'", subDir)
		}
	}

	return catcher.Resolve()
}

// Close closes all asynchronous directory handlers and removes the task output
// directory.
func (a *Directory) Close(ctx context.Context) error {
	catcher := grip.NewBasicCatcher()

	var wg sync.WaitGroup
	for id, handler := range a.handlers {
		wg.Add(1)
		go func(id string, h directoryHandler) {
			defer func() {
				catcher.Add(recovery.HandlePanicWithError(recover(), nil, "task output directory handler closer"))
			}()
			defer wg.Done()

			catcher.Wrapf(h.close(ctx), "closing task output handler for '%s'", id)
		}(id, handler)
	}
	wg.Wait()

	catcher.Wrap(os.RemoveAll(a.root), "removing task output directory")

	return catcher.Resolve()
}

type directoryHandler interface {
	start(context.Context, string) error
	close(context.Context) error
}
