package subprocess

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/mongodb/grip"
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

	// set after the command is started
	Cmd *exec.Cmd
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
		errChan <- rc.Cmd.Wait()
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

	// set to the background, if necessary
	cmdString := rc.CmdString
	if rc.Background {
		cmdString = fmt.Sprintf("nohup %v > /tmp/start 2>&1 &", cmdString)
	}
	cmdArray = append(cmdArray, cmdString)

	grip.InfoWhenf(!rc.LoggingDisabled, "Remote command executing: '%s'",
		strings.Join(cmdArray, " "))

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
