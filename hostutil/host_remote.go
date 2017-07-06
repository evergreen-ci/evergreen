package hostutil

import (
	"bytes"
	"golang.org/x/net/context"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

const SSHTimeout = time.Minute * 10

// RunRemoteScript executes a shell script that already exists on the remote host,
// returning logs and any errors that occur. Logs may still be returned for some errors.
func RunRemoteScript(h *host.Host, script string, sshOptions []string) (string, error) {
	// parse the hostname into the user, host and port
	hostInfo, err := util.ParseSSHInfo(h.Host)
	if err != nil {
		return "", err
	}
	user := h.Distro.User
	if hostInfo.User != "" {
		user = hostInfo.User
	}

	// run the remote script as sudo, if appropriate
	sudoStr := ""
	if h.Distro.SetupAsSudo {
		sudoStr = "sudo "
	}
	// run command to ssh into remote machine and execute script
	sshCmdStd := &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}
	cmd := &subprocess.RemoteCommand{
		CmdString:      sudoStr + "sh " + script,
		Stdout:         sshCmdStd,
		Stderr:         sshCmdStd,
		RemoteHostName: hostInfo.Hostname,
		User:           user,
		Options:        []string{"-p", hostInfo.Port},
		Background:     false,
	}
	// force creation of a tty if sudo
	if h.Distro.SetupAsSudo {
		cmd.Options = []string{"-t", "-t", "-p", hostInfo.Port}
	}
	cmd.Options = append(cmd.Options, sshOptions...)

	// run the ssh command with given timeout
	ctx, cancel := context.WithTimeout(context.TODO(), SSHTimeout)
	defer cancel()
	err = cmd.Run(ctx)

	return sshCmdStd.String(), errors.WithStack(err)
}
