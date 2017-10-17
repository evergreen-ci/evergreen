package hostutil

import (
	"bytes"
	"context"
	"fmt"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	// SSHTimeout is the timeout for SSH commands.
	SSHTimeout = 2 * time.Minute
)

var (
	// AgentBinaryDirectory is the directory the runner copies the agent,
	// setup, and teardown scripts to. If we cannot determine the user's
	// home directory, use "/usr/local/bin".
	AgentBinaryDirectory = "/usr/local/bin"
)

func init() {
	usr, err := user.Current()
	if err != nil {
		grip.Error(err)
		return
	}

	AgentBinaryDirectory = usr.HomeDir
}

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
		fmt.Sprintf("cd %s;", AgentBinaryDirectory),
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
func executableSubPath(d *distro.Distro) string {
	return filepath.Join(d.Arch, binaryName(d))
}

// BinaryName returns the name of the evergreen binary.
func binaryName(d *distro.Distro) string {
	name := "evergreen"
	if IsWindows(d) {
		return name + ".exe"
	}
	return name
}

// IsWindows returns true if a distro is a Windows distro.
func IsWindows(d *distro.Distro) bool {
	return strings.HasPrefix(d.Arch, "windows")
}

// CurlCommand returns a command for curling an agent binary to a host
func CurlCommand(url string, host *host.Host) string {
	return fmt.Sprintf("cd '%s' && curl -LO '%s/clients/%s' && chmod +x %s",
		AgentBinaryDirectory,
		url,
		executableSubPath(&host.Distro),
		binaryName(&host.Distro))
}
