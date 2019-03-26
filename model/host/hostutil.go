package host

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
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

func (h *Host) CurlCommand(url string) string {
	return fmt.Sprintf("cd ~ && curl -LO '%s/clients/%s' && chmod +x %s",
		url,
		h.Distro.ExecutableSubPath(),
		h.Distro.BinaryName())
}

const (
	// sshTimeout is the timeout for SSH commands.
	sshTimeout = 2 * time.Minute
)

// RunSSHCommand runs an SSH command on a remote host.
func (h *Host) RunSSHCommand(ctx context.Context, cmd string, sshOptions []string) (string, error) {
	env := evergreen.GetEnvironment()
	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(h.Host)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing ssh info %v", h.Host)
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
		SetOutputWriter(output).RedirectErrorToOutput(true).Append(cmd).Run(ctx)

	return output.String(), errors.Wrap(err, "error running shell cmd")
}
