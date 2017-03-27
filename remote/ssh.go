package remote

import (
	"io"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

var (
	ErrCmdTimedOut = errors.New("ssh command timed out")
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
		return nil, errors.Wrap(err, "error configuring ssh")
	}

	// open a connection to the ssh server
	conn, err := ssh.Dial("tcp", cmd.Host, clientConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "error connecting to ssh server at `%v`", cmd.Host)
	}

	// initiate a session for running an ssh command
	session, err := conn.NewSession()
	if err != nil {
		return nil, errors.Wrapf(err, "error creating an ssh session to `%v`", cmd.Host)
	}
	defer session.Close()

	// set stdin appropriately
	session.Stdin = cmd.Stdin

	// terminal modes for the pty we'll be using
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	// request a pseudo terminal
	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		return nil, errors.Errorf("error requesting pty: %v", err)
	}

	// kick the ssh command off
	errChan := make(chan error)
	output := []byte{}
	go func() {
		output, err = session.CombinedOutput(cmd.Command)
		errChan <- errors.WithStack(err)
	}()

	// wait for the command to finish, or time out
	select {

	case err := <-errChan:
		return output, errors.WithStack(err)

	case <-time.After(cmd.Timeout):
		// command timed out; kill the remote process
		session.Signal(ssh.SIGKILL)
		return nil, ErrCmdTimedOut
	}

}
