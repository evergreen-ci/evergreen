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
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// SSHTimeout is the timeout for SSH commands.
	SSHTimeout = 2 * time.Minute
)

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
	return fmt.Sprintf("cd ~ && curl -LO '%s/clients/%s' && chmod +x %s",
		url,
		executableSubPath(&host.Distro),
		binaryName(&host.Distro))
}

// SetupCommand returns a command for running the setup script on a host
func SetupCommand(host *host.Host) string {
	cmd := fmt.Sprintf("%s host setup",
		filepath.Join("~", binaryName(&host.Distro)))
	if host.Distro.SetupAsSudo {
		cmd += " --setup_as_sudo"
	}

	cmd += fmt.Sprintf(" --working_directory=%s", host.Distro.WorkDir)

	return cmd
}

// TearDownCommand returns a command for running a teardown script on a host.
func TearDownCommand(host *host.Host) string {
	cmd := fmt.Sprintf("%s host teardown",
		filepath.Join("~", binaryName(&host.Distro)))
	return cmd
}

// RunSSHCommand runs an SSH command on a remote host.
func RunSSHCommand(ctx context.Context, cmd string, sshOptions []string, host host.Host) (string, error) {
	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(host.Host)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing ssh info %v", host.Host)
	}

	output := newCappedOutputLog()
	opts := subprocess.OutputOptions{Output: output, SendErrorToOutput: true}
	proc := subprocess.NewRemoteCommand(
		cmd,
		hostInfo.Hostname,
		host.User,
		nil,   // env
		false, // background
		append([]string{"-p", hostInfo.Port, "-t", "-t"}, sshOptions...),
		false, // loggingDisabled
	)

	if err = proc.SetOutput(opts); err != nil {
		return "", errors.Wrap(err, "problem setting up command output")
	}

	grip.Info(message.Fields{
		"command": proc,
		"host_id": host.Id,
		"message": "running command over ssh",
	})

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, SSHTimeout)
	defer cancel()

	err = proc.Run(ctx)
	grip.Notice(proc.Stop())
	return output.String(), errors.Wrap(err, "error running shell cmd")
}

func newCappedOutputLog() *util.CappedWriter {
	// store up to 1MB of streamed command output to print if a command fails
	return &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}
}
