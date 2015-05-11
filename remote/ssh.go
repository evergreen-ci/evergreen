package remote

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"time"
)

var (
	ErrCmdTimedOut = fmt.Errorf("ssh command timed out")
)

// SSHCommand abstracts a single command to be run via ssh, on a remote machine.
type SSHCommand struct {
	// the command to be run on the remote machine
	Command string

	// the remote host to connect to
	Host string

	// the user to connect with
	User string

	// the location of the private key file (PEM-encoded) to be used
	Keyfile string

	// stdin for the remote command
	Stdin io.Reader

	// the threshold at which the command is considered to time out, and will be killed
	Timeout time.Duration
}

// Run the command via ssh. Returns the combined stdout and stderr, as well as any
// error that occurs.
func (cmd *SSHCommand) Run() ([]byte, error) {

	// configure appropriately
	clientConfig, err := createClientConfig(cmd.User, cmd.Keyfile)
	if err != nil {
		return nil, fmt.Errorf("error configuring ssh: %v", err)
	}

	// open a connection to the ssh server
	conn, err := ssh.Dial("tcp", cmd.Host, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("error connecting to ssh server at `%v`: %v", cmd.Host, err)
	}

	// initiate a session for running an ssh command
	session, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("error creating an ssh session to `%v`: %v", cmd.Host, err)
	}
	defer session.Close()

	// set stdin appropriately
	session.Stdin = cmd.Stdin

	// terminal modes for the pty we'll be using
	// TODO: change speeds?
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	// request a pseudo terminal
	// TODO: size?
	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		return nil, fmt.Errorf("error requesting pty: %v", err)
	}

	// kick the ssh command off
	errChan := make(chan error)
	output := []byte{}
	go func() {
		output, err = session.CombinedOutput(cmd.Command)
		errChan <- err
	}()

	// wait for the command to finish, or time out
	select {

	case err := <-errChan:
		return output, err

	case <-time.After(cmd.Timeout):
		// command timed out; kill the remote process
		session.Signal(ssh.SIGKILL)
		return nil, ErrCmdTimedOut
	}

}
