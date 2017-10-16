package hostutil

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

const SSHTimeout = 2 * time.Minute

// RunRemoteScript executes a command, returning logs and any errors that occur.
//
// WARNING: RunRemoteScript is not safe to use for non-trivial scripts, as it
// naively handles shell quoting.
func RunRemoteScript(ctx context.Context, h *host.Host, script string, sshOptions []string) (string, error) {
	// parse the hostname into the user, host and port
	hostInfo, err := util.ParseSSHInfo(h.Host)
	if err != nil {
		return "", err
	}
	user := h.Distro.User
	if hostInfo.User != "" {
		user = hostInfo.User
	}

	cmdArgs := []string{
		fmt.Sprintf("cd %s;", h.Distro.WorkDir),
	}

	// run the remote script as sudo, if appropriate
	if h.Distro.SetupAsSudo {
		cmdArgs = append(cmdArgs, "sudo")
	}

	cmdArgs = append(cmdArgs, "sh", "-c", fmt.Sprintf("'%s'", script))

	// run command to ssh into remote machine and execute script
	sshCmdStd := &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}

	cmd := &subprocess.RemoteCommand{
		CmdString:      strings.Join(cmdArgs, " "),
		Stdout:         sshCmdStd,
		Stderr:         sshCmdStd,
		RemoteHostName: hostInfo.Hostname,
		User:           user,
		Options:        []string{"-t", "-t", "-p", hostInfo.Port},
		Background:     false,
	}

	if len(sshOptions) > 0 {
		cmd.Options = append(cmd.Options, sshOptions...)
	}

	// run the ssh command with given timeout
	ctx, cancel := context.WithTimeout(ctx, SSHTimeout)
	defer cancel()
	err = cmd.Run(ctx)

	return sshCmdStd.String(), errors.WithStack(err)
}
