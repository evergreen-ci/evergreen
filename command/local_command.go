package command

import (
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
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
}

func (lc *LocalCommand) Run() error {
	err := lc.Start()
	if err != nil {
		return err
	}
	return lc.Cmd.Wait()
}

func (lc *LocalCommand) Start() error {
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
	if lc.Cmd != nil && lc.Cmd.Process != nil {
		return lc.Cmd.Process.Kill()
	}
	evergreen.Logger.Logf(slogger.WARN, "Trying to stop command but Cmd / Process was nil")
	return nil
}

func (lc *LocalCommand) PrepToRun(expansions *Expansions) error {
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
