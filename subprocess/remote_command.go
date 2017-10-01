package subprocess

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

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
	err := rc.Start(ctx)
	if err != nil {
		return err
	}

	if rc.Cmd != nil && rc.Cmd.Process != nil {
		grip.Debugf("RemoteCommand(%s) started process %d", rc.Id, rc.Cmd.Process.Pid)
	} else {
		grip.Warningf("RemoteCommand(%s) has nil Cmd or Cmd.Process in Run()", rc.Id)
	}

	chckCtx, cancel := context.WithCancel(ctx)

	errChan := make(chan error)

	go func() {
		errChan <- rc.Cmd.Wait()
	}()

	go func() {
		timer := time.NewTimer(0)
		defer timer.Stop()
		startAt := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if rc.Cmd.ProcessState.Exited() {
					grip.Info(message.Fields{
						"message": "remote command process ended early",
						"id":      rc.Id,
						"cmd":     rc.CmdString,
						"runtime": time.Since(startAt),
						"span":    time.Since(startAt).String(),
					})
					cancel()
					return
				}
				timer.Reset(100 * time.Millisecond)
			}
		}
	}()

	select {
	case err = <-errChan:
		return errors.WithStack(err)
	case <-chckCtx.Done():
		return nil
	case <-ctx.Done():
		return errors.Errorf("operation '%s' was canceled and terminated.",
			rc.CmdString)
	}
}

func (rc *RemoteCommand) Start(ctx context.Context) error {
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
	cmd := exec.CommandContext(ctx, "ssh", cmdArray...)
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
