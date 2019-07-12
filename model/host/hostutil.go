package host

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/jasper"
	jaspercli "github.com/mongodb/jasper/cli"
	"github.com/mongodb/jasper/rpc"
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
		return nil, errors.Wrapf(err, "error parsing ssh info %s", h.Host)
	}
	if hostInfo.User == "" {
		hostInfo.User = h.User
	}

	return hostInfo, nil
}

// GetSSHOptions returns the options to SSH into this host.
// EVG-6389: this currently relies on the fact that the EC2 provider has a
// single distro-level SSH key name corresponding to an existing SSH key file on
// the app servers. We should be able to handle multiple keys configured in
// admin settings rather than from a file name in distro settings.
func (h *Host) GetSSHOptions(settings *evergreen.Settings) ([]string, error) {
	keyPath := settings.Keys[h.Distro.SSHKey]
	if keyPath == "" {
		return nil, errors.New("no SSH key specified for host")
	}

	opts := []string{"-i", keyPath}
	hasKnownHostsFile := false

	for _, opt := range h.Distro.SSHOptions {
		opt = strings.Trim(opt, " \t")
		opts = append(opts, "-o", opt)
		if strings.HasPrefix(opt, "UserKnownHostsFile") {
			hasKnownHostsFile = true
		}
	}

	if !hasKnownHostsFile {
		opts = append(opts, "-o", "UserKnownHostsFile=/dev/null")
	}
	return opts, nil
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
		ExtendRemoteArgs("-p", hostInfo.Port, "-t", "-t").ExtendRemoteArgs(sshOptions...).
		SetOutputWriter(output).RedirectErrorToOutput(true).
		Append(cmd).Run(ctx)

	return output.String(), errors.Wrap(err, "error running shell cmd")
}

// InitSystem determines the current Linux init system used by this host.
func (h *Host) InitSystem(ctx context.Context, sshOptions []string) (string, error) {
	logs, err := h.RunSSHCommand(ctx, initSystemCommand(), sshOptions)
	if err != nil {
		return "", errors.Wrapf(err, "init system command returned: %s", logs)
	}

	if strings.Contains(logs, InitSystemSystemd) {
		return InitSystemSystemd, nil
	} else if strings.Contains(logs, InitSystemSysV) {
		return InitSystemSysV, nil
	} else if strings.Contains(logs, InitSystemUpstart) {
		return InitSystemUpstart, nil
	}

	return "", errors.Errorf("could not determine init system: init system command returned: %s", logs)
}

// initSystemCommand returns the string command to determine a Linux host's
// init system. If it succeeds, it returns the init system as a string.
func initSystemCommand() string {
	return `
	if [[ -x /sbin/init ]] && /sbin/init --version 2>/dev/null | grep -i 'upstart' >/dev/null 2>&1; then
		echo 'upstart';
		exit 0;
	fi
	if file /sbin/init 2>/dev/null | grep -i 'systemd' >/dev/null 2>&1; then
		echo 'systemd';
		exit 0;
	elif file /sbin/init 2>/dev/null | grep -i 'upstart' >/dev/null 2>&1; then
		echo 'upstart'
		exit 0;
	elif file /sbin/init 2>/dev/null | grep -i 'sysv' >/dev/null 2>&1; then
		echo 'sysv'
		exit 0;
	fi
	if type systemctl >/dev/null 2>&1; then
		echo 'systemd'
		exit 0;
	fi
	if ps -p 1 2>/dev/null | grep -i 'systemd' >/dev/null 2>&1; then
		echo 'systemd';
		exit 0;
	fi
	`
}

// FetchAndReinstallJasperCommand returns the command to fetch Jasper and
// restart the service with the latest version.
func (h *Host) FetchAndReinstallJasperCommand(config evergreen.HostJasperConfig) string {
	return strings.Join([]string{
		h.FetchJasperCommand(config),
		h.ForceReinstallJasperCommand(config),
	}, " && ")
}

// ForceReinstallJasperCommand returns the command to stop the Jasper service,
// delete the current Jasper service configuration (if it exists), install the
// new configuration, and restart the service.
func (h *Host) ForceReinstallJasperCommand(config evergreen.HostJasperConfig) string {
	params := []string{fmt.Sprintf("--port=%d", config.Port)}
	if h.Distro.JasperCredentialsPath != "" {
		params = append(params, fmt.Sprintf("--creds_path=%s", h.Distro.JasperCredentialsPath))
	}

	return h.jasperServiceCommand(config, jaspercli.ForceReinstallCommand, params...)
}

// RestartJasperCommand returns the command to restart the Jasper service with
// the existing configuration.
func (h *Host) RestartJasperCommand(config evergreen.HostJasperConfig) string {
	return h.jasperServiceCommand(config, jaspercli.RestartCommand)
}

func (h *Host) jasperServiceCommand(config evergreen.HostJasperConfig, subCmd string, args ...string) string {
	cmd := fmt.Sprintf("%s jasper service %s rpc %s", h.jasperBinaryFilePath(config), subCmd, strings.Join(args, " "))
	// Jasper service commands need elevated privileges to execute. On Windows,
	// this is assuming that the command is already being run by Administrator.
	if !h.Distro.IsWindows() {
		cmd = "sudo " + cmd
	}
	return cmd
}

// FetchJasperCommand builds the command to download and extract the Jasper
// binary into the distro-specific binary directory.
func (h *Host) FetchJasperCommand(config evergreen.HostJasperConfig) string {
	return strings.Join(h.fetchJasperCommands(config), " && ")
}

func (h *Host) fetchJasperCommands(config evergreen.HostJasperConfig) []string {
	downloadedFile := h.jasperDownloadedFileName(config)
	extractedFile := h.jasperBinaryFileName(config)
	return []string{
		fmt.Sprintf("cd \"%s\"", h.Distro.CuratorDir),
		fmt.Sprintf("curl -LO '%s/%s'", config.URL, downloadedFile),
		fmt.Sprintf("tar xzf '%s'", downloadedFile),
		fmt.Sprintf("chmod +x '%s'", extractedFile),
		fmt.Sprintf("rm -f '%s'", downloadedFile),
	}
}

// FetchJasperCommandWithPath is the same as FetchJasperCommand but sets the
// PATH variable to path for each command.
func (h *Host) FetchJasperCommandWithPath(config evergreen.HostJasperConfig, path string) string {
	cmds := h.fetchJasperCommands(config)
	for i := range cmds {
		cmds[i] = fmt.Sprintf("PATH=%s %s", path, cmds[i])
	}
	return strings.Join(cmds, " && ")
}

// jasperDownloadedFileName returns the name of the downloaded archive file that
// contains the Jasper binary.
func (h *Host) jasperDownloadedFileName(config evergreen.HostJasperConfig) string {
	os, arch := h.Distro.Platform()
	return fmt.Sprintf("%s-%s-%s-%s.tar.gz", config.DownloadFileName, os, arch, config.Version)
}

// jasperBinaryFileName return the filename of the Jasper binary.
func (h *Host) jasperBinaryFileName(config evergreen.HostJasperConfig) string {
	if h.Distro.IsWindows() {
		return config.BinaryName + ".exe"
	}
	return config.BinaryName
}

// jasperBinaryFilePath returns the full path to the Jasper b inary.
func (h *Host) jasperBinaryFilePath(config evergreen.HostJasperConfig) string {
	return filepath.Join(h.Distro.CuratorDir, h.jasperBinaryFileName(config))
}

// BootstrapScript creates the user data script to bootstrap the host.
func (h *Host) BootstrapScript(config evergreen.HostJasperConfig, creds *rpc.Credentials) (string, error) {
	writeCredentialsCmd, err := h.WriteJasperCredentialsFileCommand(config, creds)
	if err != nil {
		return "", errors.Wrap(err, "could not build command to write Jasper credentials file")
	}

	if h.Distro.IsWindows() {
		cmds := []string{
			h.FetchJasperCommandWithPath(config, "/bin"),
			writeCredentialsCmd,
			h.ForceReinstallJasperCommand(config),
		}
		// PowerShell nested quotation marks are handled by using two quotation
		// marks.
		quotedCmds := make([]string, 0, len(cmds))
		for _, cmd := range cmds {
			quotedCmds = append(quotedCmds, strings.Replace(cmd, "'", "''", -1))
		}
		commands := []string{
			"<powershell>",
			fmt.Sprintf("%s -c '%s'", h.Distro.ShellPath, quotedCmds),
			"</powershell>",
		}
		return strings.Join(commands, "\r\n"), nil
	}
	return strings.Join([]string{"#!/bin/bash",
		h.FetchJasperCommand(config),
		writeCredentialsCmd,
		h.ForceReinstallJasperCommand(config),
	}, "\n"), nil
}

// WriteJasperCredentialsCommand builds the command to write the Jasper
// credentials to a file.
func (h *Host) WriteJasperCredentialsFileCommand(config evergreen.HostJasperConfig, creds *rpc.Credentials) (string, error) {
	if h.Distro.JasperCredentialsPath == "" {
		return "", errors.New("cannot write Jasper credentials without a credentials file path")
	}
	exportedCreds, err := creds.Export()
	if err != nil {
		return "", errors.Wrap(err, "problem exporting credentials to file format")
	}
	return fmt.Sprintf("cat > %s <<EOF\n%s\nEOF", h.Distro.JasperCredentialsPath, exportedCreds), nil
}

// RunJasperProcess makes a request to the host's Jasper service to create the
// process with the given options, wait for its completion, and return the
// output from it.
func (h *Host) RunJasperProcess(ctx context.Context, env evergreen.Environment, opts *jasper.CreateOptions) (string, error) {
	client, err := h.JasperClient(ctx, env)
	if err != nil {
		return "", errors.Wrap(err, "could not get a Jasper client")
	}
	output := &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1 MB
	}

	proc, err := client.CreateProcess(ctx, opts)
	if err != nil {
		return "", errors.Wrap(err, "problem creating process")
	}

	exitCode, err := proc.Wait(ctx)
	if err != nil {
		return "", errors.Wrap(err, "problem waiting for process completion")
	}
	if exitCode != 0 {
		return "", errors.Errorf("process returned exit code %d", exitCode)
	}

	return output.String(), nil
}

// StartJasperProcess makes a request to the host's Jasper service to start a
// process with the given options without waiting for its completion.
func (h *Host) StartJasperProcess(ctx context.Context, env evergreen.Environment, opts *jasper.CreateOptions) error {
	client, err := h.JasperClient(ctx, env)
	if err != nil {
		return errors.Wrap(err, "could not get a Jasper client")
	}

	if _, err := client.CreateProcess(ctx, opts); err != nil {
		return errors.Wrap(err, "problem creating Jasper process")
	}

	return nil
}

// JasperClient returns a remote client that communicates with this host's
// Jasper service.
func (h *Host) JasperClient(ctx context.Context, env evergreen.Environment) (jasper.RemoteClient, error) {
	if h.LegacyBootstrap() || h.LegacyCommunication() {
		return nil, errors.New("legacy host does not support remote Jasper process management")
	}

	settings := env.Settings()

	if h.JasperCommunication() {
		switch h.Distro.CommunicationMethod {
		case distro.CommunicationMethodSSH:
			hostInfo, err := h.GetSSHInfo()
			if err != nil {
				return nil, errors.Wrap(err, "could not get host's SSH info")
			}
			sshOpts, err := h.GetSSHOptions(settings)
			if err != nil {
				return nil, errors.Wrap(err, "could not get host's SSH options")
			}

			remoteOpts := jasper.RemoteOptions{
				Host: hostInfo.Hostname,
				User: hostInfo.User,
				Args: sshOpts,
			}
			clientOpts := jaspercli.ClientOptions{
				BinaryPath:          h.jasperBinaryFilePath(settings.HostJasper),
				Type:                jaspercli.RPCService,
				Port:                settings.HostJasper.Port,
				CredentialsFilePath: h.Distro.JasperCredentialsPath,
			}

			return jaspercli.NewSSHClient(remoteOpts, clientOpts, true)
		case distro.CommunicationMethodRPC:
			creds, err := h.JasperClientCredentials(ctx, env)
			if err != nil {
				return nil, errors.Wrap(err, "could not get client credentials to communicate with the host's Jasper service")
			}

			var hostName string
			if h.Host != "" {
				hostName = h.Host
			} else if h.IP != "" {
				hostName = fmt.Sprintf("[%s]", h.IP)
			} else {
				return nil, errors.New("cannot resolve Jasper service address if neither host name nor IP is set")
			}

			addrStr := fmt.Sprintf("%s:%d", hostName, settings.HostJasper.Port)

			serviceAddr, err := net.ResolveTCPAddr("tcp", addrStr)
			if err != nil {
				return nil, errors.Wrapf(err, "could not resolve Jasper service address at '%s'", addrStr)
			}

			return rpc.NewClient(ctx, serviceAddr, creds)
		}
	}

	return nil, errors.Errorf("host does not have recognized communication method '%s'", h.Distro.CommunicationMethod)
}
