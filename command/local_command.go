package command

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/tychoish/grip"
)

type LocalCommand struct {
	CmdString        string
	WorkingDirectory string
	Shell            string
	Environment      []string
	ScriptMode       bool
	Stdout           io.Writer
	Stderr           io.Writer
	Cmd              *exec.Cmd
	mutex            sync.RWMutex
}

func (lc *LocalCommand) Run() error {
	err := lc.Start()
	if err != nil {
		return err
	}

	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	return lc.Cmd.Wait()
}

func (lc *LocalCommand) GetPid() int {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	if lc.Cmd == nil {
		return -1
	}

	return lc.Cmd.Process.Pid
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
	lc.Cmd = cmd

	// start the command
	return cmd.Start()
}

func (lc *LocalCommand) Stop() error {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	if lc.Cmd != nil && lc.Cmd.Process != nil {
		return lc.Cmd.Process.Kill()
	}
	grip.Warning("Trying to stop command but Cmd / Process was nil")
	return nil
}

func (lc *LocalCommand) PrepToRun(expansions *Expansions) error {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	var err error

	lc.CmdString, err = expansions.ExpandString(lc.CmdString)
	if err != nil {
		return err
	}

	lc.WorkingDirectory, err = expansions.ExpandString(lc.WorkingDirectory)
	if err != nil {
		return err
	}

	return nil
}

type LocalCommandGroup struct {
	Commands   []*LocalCommand
	Expansions *Expansions
}

func (lc *LocalCommandGroup) PrepToRun() error {
	for _, cmd := range lc.Commands {
		if err := cmd.PrepToRun(lc.Expansions); err != nil {
			return err
		}
	}
	return nil
}
