package hostutil

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
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
func RunSSHCommand(id, cmd string, sshOptions []string, host host.Host) error {
	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(host.Host)
	if err != nil {
		return errors.Wrapf(err, "error parsing ssh info %v", host.Host)
	}

	output := newCappedOutputLog()
	shellCmd := &subprocess.RemoteCommand{
		Id:             fmt.Sprintf("%s-%s-%d", id, host.Id, rand.Int()),
		CmdString:      cmd,
		Stdout:         output,
		Stderr:         output,
		RemoteHostName: hostInfo.Hostname,
		User:           host.User,
		Options:        append([]string{"-p", hostInfo.Port, "-t", "-t"}, sshOptions...),
	}
	grip.Info(message.Fields{
		"command": shellCmd,
		"host_id": host.Id,
		"message": "running command over ssh",
	})

	ctx, cancel := context.WithTimeout(context.Background(), SSHTimeout)
	defer cancel()
	err = shellCmd.Run(ctx)

	grip.Notice(shellCmd.Stop())
	if err != nil {
		return errors.Errorf("error running shell cmd: %s (%v)", output.String(), err)
	}
	return nil
}

func newCappedOutputLog() *util.CappedWriter {
	// store up to 1MB of streamed command output to print if a command fails
	return &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}
}
