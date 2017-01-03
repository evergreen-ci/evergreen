package command

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/tychoish/grip/slogger"
	"github.com/evergreen-ci/evergreen"
)

type RemoteCommand struct {
	Id        string
	CmdString string

	Stdout io.Writer
	Stderr io.Writer

	// info necessary for sshing into the remote host
	RemoteHostName string
	User           string
	Options        []string
	Background     bool

	// optional flag for hiding sensitive commands from log output
	LoggingDisabled bool

	// set after the command is started
	Cmd *exec.Cmd
}

func (rc *RemoteCommand) Run() error {
	evergreen.Logger.Logf(slogger.DEBUG, "RemoteCommand(%v) beginning Run()", rc.Id)
	err := rc.Start()
	if err != nil {
		return err
	}
	if rc.Cmd != nil && rc.Cmd.Process != nil {
		evergreen.Logger.Logf(slogger.DEBUG, "RemoteCommand(%v) started process %v", rc.Id, rc.Cmd.Process.Pid)
	} else {
		evergreen.Logger.Logf(slogger.DEBUG, "RemoteCommand(%v) has nil Cmd or Cmd.Process in Run()", rc.Id)
	}
	return rc.Cmd.Wait()
}

func (rc *RemoteCommand) Wait() error {
	return rc.Cmd.Wait()
}

func (rc *RemoteCommand) Start() error {

	// build the remote connection, in user@host format
	remote := rc.RemoteHostName
	if rc.User != "" {
		remote = fmt.Sprintf("%v@%v", rc.User, remote)
	}

	// build the command
	cmdArray := append(rc.Options, remote)

	// set to the background, if necessary
	cmdString := rc.CmdString
	if rc.Background {
		cmdString = fmt.Sprintf("nohup %v > /tmp/start 2>&1 &", cmdString)
	}
	cmdArray = append(cmdArray, cmdString)

	if !rc.LoggingDisabled {
		evergreen.Logger.Logf(slogger.WARN, "Remote command executing: '%#v'",
			strings.Join(cmdArray, " "))
	}

	// set up execution
	cmd := exec.Command("ssh", cmdArray...)
	cmd.Stdout = rc.Stdout
	cmd.Stderr = rc.Stderr

	// cache the command running
	rc.Cmd = cmd
	return cmd.Start()
}

func (rc *RemoteCommand) Stop() error {
	if rc.Cmd != nil && rc.Cmd.Process != nil {
		evergreen.Logger.Logf(slogger.DEBUG, "RemoteCommand(%v) killing process %v", rc.Id, rc.Cmd.Process.Pid)
		return rc.Cmd.Process.Kill()
	}
	evergreen.Logger.Logf(slogger.WARN, "RemoteCommand(%v) Trying to stop command but Cmd / Process was nil", rc.Id)
	return nil
}
