package subprocess

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type LocalCommand struct {
	CmdString        string    `json:"command"`
	WorkingDirectory string    `json:"directory"`
	Shell            string    `json:"shell"`
	Environment      []string  `json:"environment"`
	ScriptMode       bool      `json:"script"`
	Stdout           io.Writer `json:"-"`
	Stderr           io.Writer `json:"-"`
	cmd              *exec.Cmd
	mutex            sync.RWMutex
}

func (lc *LocalCommand) Run(ctx context.Context) error {
	err := lc.Start()
	if err != nil {
		return err
	}

	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	errChan := make(chan error)
	go func() {
		select {
		case errChan <- lc.cmd.Wait():
		case <-ctx.Done():
		}
	}()

	select {
	case <-ctx.Done():
		err = lc.cmd.Process.Kill()
		return errors.Wrapf(err,
			"operation '%s' was canceled and terminated.",
			lc.CmdString)
	case err = <-errChan:
		return errors.WithStack(err)
	}
}

func (lc *LocalCommand) Wait() error {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	return lc.cmd.Wait()
}

func (lc *LocalCommand) GetPid() int {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	if lc.cmd == nil {
		return -1
	}

	return lc.cmd.Process.Pid
}

func (lc *LocalCommand) Start() error {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	if lc.Shell == "" {
		lc.Shell = "sh"
	}

	var cmd *exec.Cmd
	if lc.ScriptMode {
		cmd = exec.Command(lc.Shell)
		cmd.Stdin = strings.NewReader(lc.CmdString)
	} else {
		cmd = exec.Command(lc.Shell, "-c", lc.CmdString)
	}

	// create the command, set the options
	if lc.WorkingDirectory != "" {
		cmd.Dir = lc.WorkingDirectory
	}
	cmd.Env = lc.Environment
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Stdout = lc.Stdout
	cmd.Stderr = lc.Stderr

	// cache the command running
	lc.cmd = cmd

	// start the command
	return cmd.Start()
}

func (lc *LocalCommand) Stop() error {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	if lc.cmd != nil && lc.cmd.Process != nil {
		return lc.cmd.Process.Kill()
	}
	grip.Warning("Trying to stop command but Cmd / Process was nil")
	return nil
}

func (lc *LocalCommand) PrepToRun(expansions *util.Expansions) error {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	var err error

	lc.CmdString, err = expansions.ExpandString(lc.CmdString)
	if err != nil {
		return errors.WithStack(err)
	}

	lc.WorkingDirectory, err = expansions.ExpandString(lc.WorkingDirectory)
	return errors.WithStack(err)
}
