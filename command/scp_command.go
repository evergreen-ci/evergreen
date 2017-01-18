package command

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/tychoish/grip"
)

type ScpCommand struct {
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

func (self *ScpCommand) Run() error {
	grip.Debugf("SCPCommand(%s) beginning Run()", self.Id)

	if err := self.Start(); err != nil {
		return err
	}

	if self.Cmd != nil && self.Cmd.Process != nil {
		grip.Debugf("SCPCommand(%s) started process %d", self.Id, self.Cmd.Process.Pid)
	} else {
		grip.Debugf("SCPCommand(%s) has nil Cmd or Cmd.Process in Run()", self.Id)
	}

	return self.Cmd.Wait()
}

func (self *ScpCommand) Start() error {

	// build the remote side of the connection, in user@host: format
	remote := self.RemoteHostName
	if self.User != "" {
		remote = fmt.Sprintf("%v@%v", self.User, remote)
	}

	// set up the source and destination
	source := self.Source
	dest := self.Dest
	if self.SourceIsRemote {
		source = fmt.Sprintf("%v:%v", remote, source)
	} else {
		dest = fmt.Sprintf("%v:%v", remote, dest)
	}

	// build the command
	cmdArray := append(self.Options, source, dest)

	// set up execution
	cmd := exec.Command("scp", cmdArray...)
	cmd.Stdout = self.Stdout
	cmd.Stderr = self.Stderr

	// cache the command running
	self.Cmd = cmd

	return cmd.Start()
}

func (self *ScpCommand) Stop() error {
	if self.Cmd != nil && self.Cmd.Process != nil {
		grip.Debugf("SCPCommand(%s) killing process %d", self.Id, self.Cmd.Process.Pid)
		return self.Cmd.Process.Kill()
	}
	grip.Warningf("SCPCommand(%s) Trying to stop command but Cmd / Process was nil", self.Id)
	return nil
}
