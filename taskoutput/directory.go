package taskoutput

import (
	"context"
	"os"
	"sync"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
)

type DirectoryApp struct {
	rootDir  string
	handlers map[string]directoryHandler
}

func (o TaskOutput) NewDirectoryApp(rootDir string, taskOpts TaskOptions, logger grip.Journaler) *DirectoryApp {
	return &DirectoryApp{
		rootDir: rootDir,
		handlers: map[string]directoryHandler{
			o.TestLogs.ID(): o.TestLogs.newDirectoryHandler(rootDir, taskOpts, logger),
		},
	}
}

func (a *DirectoryApp) Start(ctx context.Context) error {
	catcher := grip.NewBasicCatcher()

	for id, handler := range a.handlers {
		catcher.Wrapf(handler.start(ctx), "starting task output directory handler for '%s'", id)
	}

	return nil
}

func (a *DirectoryApp) Close(ctx context.Context) error {
	catcher := grip.NewBasicCatcher()

	var wg sync.WaitGroup
	for id, handler := range a.handlers {
		wg.Add(1)
		go func() {
			defer func() {
				catcher.Add(recovery.HandlePanicWithError(recover(), nil, "task output directory handler closer"))
				wg.Done()
			}()

			catcher.Wrapf(handler.close(ctx), "closing task output handler for '%s'", id)
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
