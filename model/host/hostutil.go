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
	binaryName := h.Distro.BinaryName()
	executableSubPath := h.Distro.ExecutableSubPath()
	var curl string
	if h.Distro.IsWindows() {
		// You cannot unlink a running file on Windows. Maybe one day we https://stackoverflow.com/questions/10319526/understanding-a-self-deleting-program-in-c.
		curl = fmt.Sprintf("cd ~ && curl -LO '%s/clients/%s' && chmod +x %s",
			url,
			executableSubPath,
			binaryName)
	} else {
		curl = fmt.Sprintf("cd ~ && if [ -f %s ]; then %s get-update --install --force; else curl -LO '%s/clients/%s' && chmod +x %s; fi",
			binaryName,
			binaryName,
			url,
			executableSubPath,
			binaryName)
	}
	return curl
}

const (
	// sshTimeout is the timeout for SSH commands.
	sshTimeout = 2 * time.Minute
)

func getSSHOutputOptions() subprocess.OutputOptions {
	// store up to 1MB of streamed command output to print if a command fails
	output := &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}

	return subprocess.OutputOptions{Output: output, SendErrorToOutput: true}
}

// RunSSHCommand runs an SSH command on a remote host.
func (h *Host) RunSSHCommand(ctx context.Context, cmd string, sshOptions []string) (string, error) {
	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(h.Host)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing ssh info %v", h.Host)
	}

	opts := getSSHOutputOptions()
	output := opts.Output.(*util.CappedWriter)

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
		"command":  cmd,
		"hostname": hostInfo.Hostname,
		"user":     h.User,
		"host_id":  h.Id,
		"message":  "running command over ssh",
	})

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, sshTimeout)
	defer cancel()

	err = proc.Run(ctx)
	grip.Notice(proc.Stop())

	return output.String(), errors.Wrap(err, "error running shell cmd")
}
