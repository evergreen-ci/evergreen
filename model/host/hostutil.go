package host

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

func (h *Host) SetupCommand() string {
	cmd := fmt.Sprintf("%s host setup", filepath.Join("~", h.Distro.BinaryName()))

	if h.Distro.SetupAsSudo {
		cmd += " --setup_as_sudo"
	}

	cmd += fmt.Sprintf(" --working_directory=%s", h.Distro.WorkDir)

	return cmd
}

// TearDownCommand returns a command for running a teardown script on a host.
func (h *Host) TearDownCommand() string {
	return fmt.Sprintf("%s host teardown", filepath.Join("~", h.Distro.BinaryName()))
}

// TearDownCommandOverSSH returns a command for running a teardown script on a host. This command
// runs if there is a problem running host teardown with the agent and is intended only as a
// backstop if multiple agent deploys interfere with one another
// (https://jira.mongodb.org/browse/EVG-5972). It likely can be removed after work to improve amboy
// job locking or the SSH dependency.
func TearDownCommandOverSSH() string {
	chmod := ChmodCommandWithSudo(context.Background(), evergreen.TeardownScriptName, false).Args
	chmodString := strings.Join(chmod, " ")
	sh := ShCommandWithSudo(context.Background(), evergreen.TeardownScriptName, false).Args
	shString := strings.Join(sh, " ")
	return fmt.Sprintf("%s && %s", chmodString, shString)
}

func ShCommandWithSudo(ctx context.Context, script string, sudo bool) *exec.Cmd {
	if sudo {
		return exec.CommandContext(ctx, "sudo", "sh", script)
	}
	return exec.CommandContext(ctx, "sh", script)
}

func ChmodCommandWithSudo(ctx context.Context, script string, sudo bool) *exec.Cmd {
	args := []string{}
	if sudo {
		args = append(args, "sudo")
	}
	args = append(args, "chmod", "+x", script)
	return exec.CommandContext(ctx, args[0], args[1:]...)
}

func (h *Host) CurlCommand(url string) string {
	return fmt.Sprintf("cd ~ && curl -LO '%s/clients/%s' && chmod +x %s",
		strings.TrimRight(url, "/"),
		h.Distro.ExecutableSubPath(),
		h.Distro.BinaryName())
}

const (
	// sshTimeout is the timeout for SSH commands.
	sshTimeout = 2 * time.Minute
)

func (h *Host) GetSSHInfo() (*util.StaticHostInfo, error) {
	hostInfo, err := util.ParseSSHInfo(h.Host)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing ssh info %v", h.Host)
	}
	if hostInfo.User == "" {
		hostInfo.User = h.User
	}

	return hostInfo, nil
}

// RunSSHCommand runs an SSH command on a remote host.
func (h *Host) RunSSHCommand(ctx context.Context, cmd string, sshOptions []string) (string, error) {
	env := evergreen.GetEnvironment()
	hostInfo, err := h.GetSSHInfo()
	if err != nil {
		return "", errors.WithStack(err)
	}

	output := &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, sshTimeout)
	defer cancel()

	err = env.JasperManager().CreateCommand(ctx).Host(hostInfo.Hostname).User(hostInfo.User).
		ExtendSSHArgs("-p", hostInfo.Port, "-t", "-t").ExtendSSHArgs(sshOptions...).
		SetOutputWriter(output).RedirectErrorToOutput(true).
		Append(cmd).Run(ctx)

	return output.String(), errors.Wrap(err, "error running shell cmd")
}
