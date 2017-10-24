package subprocess

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type RemoteCommand struct {
	Id        string `json:"id"`
	CmdString string `json:"command"`

	Stdout io.Writer `json:"-"`
	Stderr io.Writer `json:"-"`

	// info necessary for sshing into the remote host
	RemoteHostName string   `json:"remote_host"`
	User           string   `json:"user"`
	Options        []string `json:"options"`
	Background     bool     `json:"background"`

	// optional flag for hiding sensitive commands from log output
	LoggingDisabled bool     `json:"disabled_logging"`
	EnvVars         []string `json:"env_vars"`

	// set after the command is started
	Cmd *exec.Cmd `json:"-"`
}

func (rc *RemoteCommand) Run(ctx context.Context) error {
	grip.Debugf("RemoteCommand(%s) beginning Run()", rc.Id)
	err := rc.Start()
	if err != nil {
		return err
	}

	if rc.Cmd != nil && rc.Cmd.Process != nil {
		grip.Debugf("RemoteCommand(%s) started process %d", rc.Id, rc.Cmd.Process.Pid)
	} else {
		grip.Warningf("RemoteCommand(%s) has nil Cmd or Cmd.Process in Run()", rc.Id)
	}

	errChan := make(chan error)
	go func() {
		select {
		case errChan <- rc.Cmd.Wait():
		case <-ctx.Done():
		}
	}()

	select {
	case <-ctx.Done():
		err = rc.Cmd.Process.Kill()
		return errors.Wrapf(err,
			"operation '%s' was canceled and terminated.",
			rc.CmdString)
	case err = <-errChan:
		return errors.WithStack(err)
	}
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

	if len(rc.EnvVars) > 0 {
		cmdArray = append(cmdArray, strings.Join(rc.EnvVars, " "))
	}

	// set to the background, if necessary
	cmdString := rc.CmdString
	if rc.Background {
		cmdString = fmt.Sprintf("nohup %s > /tmp/start 2>&1 &", cmdString)
	}

	cmdArray = append(cmdArray, cmdString)

	loggedCommand := fmt.Sprintf("ssh %s", strings.Join(cmdArray, " "))
	loggedCommand = strings.Replace(loggedCommand, "%!h(MISSING)", "%h", -1)
	loggedCommand = strings.Replace(loggedCommand, "%!p(MISSING)", "%p", -1)

	grip.InfoWhen(!rc.LoggingDisabled,
		message.Fields{
			"message":    rc.Id,
			"host":       rc.RemoteHostName,
			"user":       rc.User,
			"cmd":        loggedCommand,
			"background": rc.Background,
		})

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
		grip.Debugf("RemoteCommand(%s) killing process %d", rc.Id, rc.Cmd.Process.Pid)
		err := rc.Cmd.Process.Kill()
		if err == nil || strings.Contains(err.Error(), "process already finished") {
			return nil
		}
		return errors.WithStack(err)
	}
	grip.Warningf("RemoteCommand(%s) Trying to stop command but Cmd / Process was nil", rc.Id)
	return nil
}
