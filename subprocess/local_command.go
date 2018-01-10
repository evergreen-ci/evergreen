package subprocess

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type localCmd struct {
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

func NewLocalCommand(cmdString, workingDir, shell string, env []string, scriptMode bool) Command {
	return &localCmd{
		CmdString:        cmdString,
		WorkingDirectory: workingDir,
		Shell:            shell,
		Environment:      env,
		ScriptMode:       scriptMode,
	}
}

func (lc *localCmd) Run(ctx context.Context) error {
	err := lc.Start(ctx)
	if err != nil {
		return err
	}

	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	return errors.WithStack(lc.cmd.Wait())
}

func (lc *localCmd) SetOutput(opts OutputOptions) error {
	if err := opts.Validate(); err != nil {
		return errors.WithStack(err)
	}

	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	lc.Stderr = opts.GetError()
	lc.Stdout = opts.GetOutput()

	return nil
}

func (lc *localCmd) Wait() error {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	return lc.cmd.Wait()
}

func (lc *localCmd) GetPid() int {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	if lc.cmd == nil {
		return -1
	}

	return lc.cmd.Process.Pid
}

func (lc *localCmd) Start(ctx context.Context) error {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	if lc.Shell == "" {
		lc.Shell = "sh"
	}

	var cmd *exec.Cmd
	if lc.ScriptMode {
		cmd = exec.CommandContext(ctx, lc.Shell)
		cmd.Stdin = strings.NewReader(lc.CmdString)
	} else {
		cmd = exec.CommandContext(ctx, lc.Shell, "-c", lc.CmdString)
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

func (lc *localCmd) Stop() error {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	if lc.cmd != nil && lc.cmd.Process != nil {
		return lc.cmd.Process.Kill()
	}
	grip.Warning("Trying to stop command but Cmd / Process was nil")
	return nil
}
