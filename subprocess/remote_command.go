package subprocess

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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
	EnvVars         []string

	// set after the command is started
	Cmd *exec.Cmd
}

func (rc *RemoteCommand) Run(ctx context.Context) error {
	grip.Debugf("RemoteCommand(%s) beginning Run()", rc.Id)
	err := rc.Start()
	if err != nil {
		return errors.WithStack(err)
	}

	if rc.Cmd != nil && rc.Cmd.Process != nil {
		grip.Debugf("RemoteCommand(%s) started process %d", rc.Id, rc.Cmd.Process.Pid)
	} else {
		grip.Warningf("RemoteCommand(%s) has nil Cmd or Cmd.Process in Run()", rc.Id)
	}

	errChan := make(chan error)
	go func() {
		errChan <- rc.Cmd.Wait()
		close(errChan)
	}()

	select {
	case err = <-errChan:
		if rc.Cmd.ProcessState != nil && rc.Cmd.ProcessState.Success() {
			return nil
		}

		return errors.WithStack(err)
	case <-ctx.Done():
		if rc.Cmd.ProcessState != nil && rc.Cmd.ProcessState.Success() {
			return nil
		}

		// this is a bug: we return an error only if Stop
		// returns an error, effectively swallowing errors
		// from the command itself. However, for running
		// scripts on windows hosts we (apparently) rely on
		// this behavior.
		return errors.Wrapf(rc.Stop(),
			"operation '%s' was canceled and terminated.", rc.CmdString)
	}
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
	if rc.Cmd != nil {
		if rc.Cmd.Process != nil {
			grip.Debugf("RemoteCommand(%s) killing process %d", rc.Id, rc.Cmd.Process.Pid)
			err := rc.Cmd.Process.Kill()
			if err == nil || strings.Contains(err.Error(), "process already finished") {
				return nil
			}
			return errors.WithStack(err)
		}

		if rc.Cmd.ProcessState != nil {
			if rc.Cmd.ProcessState.Success() {
				return nil
			}

			return errors.Errorf("RemoteCommand(%s) exited with status %s",
				rc.Id, rc.Cmd.ProcessState.String())
		}
	}

	grip.Warningf("RemoteCommand(%s) Trying to stop command but Cmd / Process was nil", rc.Id)

	return nil
}
