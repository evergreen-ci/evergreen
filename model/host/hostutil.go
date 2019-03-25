package host

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/jasper"
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

func getSSHOutputOptions() jasper.OutputOptions {
	// store up to 1MB of streamed command output to print if a command fails
	output := &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}

	return jasper.OutputOptions{Output: output, SendErrorToOutput: true}
}

// RunSSHCommand runs an SSH command on a remote host.
func (h *Host) RunSSHCommand(ctx context.Context, env evergreen.Environment, cmd string, sshOptions []string) (string, error) {
	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(h.Host)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing ssh info %v", h.Host)
	}

	opts := getSSHOutputOptions()
	output := opts.Output.(*util.CappedWriter)
	cmdArgs := append(append([]string{"ssh"}, "-p", hostInfo.Port, "-t", "-t", sshOptions...), cmd)

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, sshTimeout)
	defer cancel()

	err := env.JasperManager().CreateCommand().ApplyFromOpts(&jasper.CreateOptions{Output: opts}).Add(cmdArgs).Run(ctx)

	return output.String(), errors.Wrap(err, "error running shell cmd")
}
