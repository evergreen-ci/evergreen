package subprocess

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type scpCommand struct {
	Id     string
	Source string
	Dest   string

	Stdout io.Writer
	Stderr io.Writer

	// info about the remote host
	RemoteHostName string
	User           string
	Options        []string

	// tells us whether to copy from local to remote or from remote to local
	SourceIsRemote bool

	// set after the command is started
	Cmd *exec.Cmd
}

func NewSCPCommand(src, dest, hostname, user string, options []string) Command {
	return &scpCommand{
		Source:         src,
		Dest:           dest,
		RemoteHostName: hostname,
		User:           user,
		Options:        options,
	}
}

func (self *scpCommand) SetOutput(opts OutputOptions) error {
	if err := opts.Validate(); err != nil {
		return errors.WithStack(err)
	}

	self.Stderr = opts.GetError()
	self.Stdout = opts.GetOutput()

	return nil
}

func (self *scpCommand) Run(ctx context.Context) error {
	grip.Debugf("SCPCommand(%s) beginning Run()", self.Id)

	if err := self.Start(ctx); err != nil {
		return err
	}

	if self.Cmd != nil && self.Cmd.Process != nil {
		grip.Debugf("SCPCommand(%s) started process %d", self.Id, self.Cmd.Process.Pid)
	} else {
		grip.Warningf("SCPCommand(%s) has nil Cmd or Cmd.Process in Run()", self.Id)
	}

	errChan := make(chan error)
	go func() {
		errChan <- self.Cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		err := self.Cmd.Process.Kill()
		return errors.Wrapf(err,
			"scp operation '%s=>%s' was canceled and terminated.",
			self.Source, self.Dest)
	case err := <-errChan:
		return errors.WithStack(err)
	}
}

func (self *scpCommand) Wait() error {
	return self.Cmd.Wait()
}

func (self *scpCommand) GetPid() int {
	if self.Cmd == nil {
		return -1
	}

	return self.Cmd.Process.Pid
}

func (self *scpCommand) Start(ctx context.Context) error {

	// build the remote side of the connection, in user@host: format
	remote := self.RemoteHostName
	if self.User != "" {
		remote = fmt.Sprintf("%s@%s", self.User, remote)
	}

	// set up the source and destination
	source := self.Source
	dest := self.Dest
	if self.SourceIsRemote {
		source = fmt.Sprintf("%s:%s", remote, source)
	} else {
		dest = fmt.Sprintf("%s:%s", remote, dest)
	}

	// build the command
	cmdArray := append(self.Options, source, dest)

	// set up execution
	cmd := exec.CommandContext(ctx, "scp", cmdArray...)
	cmd.Stdout = self.Stdout
	cmd.Stderr = self.Stderr

	// cache the command running
	self.Cmd = cmd

	return cmd.Start()
}

func (self *scpCommand) Stop() error {
	if self.Cmd != nil && self.Cmd.Process != nil {
		grip.Debugf("SCPCommand(%s) killing process %d", self.Id, self.Cmd.Process.Pid)
		return self.Cmd.Process.Kill()
	}
	grip.Warningf("SCPCommand(%s) Trying to stop command but Cmd / Process was nil", self.Id)
	return nil
}
