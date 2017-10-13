package hostutil

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

const SSHTimeout = 2 * time.Minute

// RunRemoteScript executes a shell script that already exists on the remote host,
// returning logs and any errors that occur. Logs may still be returned for some errors.
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

	cmdArgs = append(cmdArgs, "sh", script)

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

// ExecutableSubPath returns the directory containing the compiled agents.
func ExecutableSubPath(d *distro.Distro) string {
	mainName := "evergreen"
	if IsWindows(d) {
		mainName += ".exe"
	}

	return filepath.Join(d.Arch, mainName)
}

// IsWindows returns true if a distro is a Windows distro.
func IsWindows(d *distro.Distro) bool {
	return strings.HasPrefix(d.Arch, "windows")
}
