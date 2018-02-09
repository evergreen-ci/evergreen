package host

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(h.Host)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing ssh info %v", h.Host)
	}

	// store up to 1MB of streamed command output to print if a command fails
	output := &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}
	opts := subprocess.OutputOptions{Output: output, SendErrorToOutput: true}

	proc := subprocess.NewRemoteCommand(
		cmd,
		hostInfo.Hostname,
		h.User,
		nil,   // env
		false, // background
		append([]string{"-p", hostInfo.Port, "-t", "-t"}, sshOptions...),
		false, // loggingDisabled
	)

	if err = proc.SetOutput(opts); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"function":  "RunSSHCommand",
			"operation": "setting up command output",
			"distro":    h.Distro.Id,
			"host":      h.Id,
			"output":    output,
			"cause":     "programmer error",
		}))

		return "", errors.Wrap(err, "problem setting up command output")
	}

	grip.Info(message.Fields{
		"command": proc,
		"host_id": h.Id,
		"message": "running command over ssh",
	})

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, sshTimeout)
	defer cancel()

	err = proc.Run(ctx)
	grip.Notice(proc.Stop())
	return output.String(), errors.Wrap(err, "error running shell cmd")
}
