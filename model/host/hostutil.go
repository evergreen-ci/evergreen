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
		return nil, errors.Wrapf(err, "error parsing ssh info %s", h.Host)
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
func (h *Host) FetchAndReinstallJasperCommand(config evergreen.JasperConfig) string {
	return strings.Join([]string{
		h.FetchJasperCommand(config),
		h.ForceReinstallJasperCommand(config),
	}, " && ")
}

// ForceReinstallJasperCommand returns the command to stop the Jasper service,
// delete the current Jasper service configuration (if it exists), install the
// new configuration, and restart the service.
func (h *Host) ForceReinstallJasperCommand(config evergreen.JasperConfig) string {
	port := config.Port
	if port == 0 {
		port = evergreen.DefaultJasperPort
	}
	return h.jasperServiceCommand(config, "force-reinstall", fmt.Sprintf("--port=%d", port))
}

func (h *Host) jasperServiceCommand(config evergreen.JasperConfig, subCmd string, args ...string) string {
	binaryPath := filepath.Join(h.Distro.CuratorDir, h.jasperExtractedFileName(config))
	cmd := fmt.Sprintf("%s jasper service %s rpc %s", binaryPath, subCmd, strings.Join(args, " "))
	// Jasper service commands need elevated privileges to execute. On Windows,
	// this is assuming that the command is already being run by Administrator.
	if !h.Distro.IsWindows() {
		cmd = "sudo " + cmd
	}
	return cmd
}

// FetchJasperCommand builds the command to download and extract the Jasper
// binary into the distro-specific binary directory.
func (h *Host) FetchJasperCommand(config evergreen.JasperConfig) string {
	return strings.Join(h.fetchJasperCommands(config), " && ")
}

func (h *Host) fetchJasperCommands(config evergreen.JasperConfig) []string {
	downloadedFile := h.jasperDownloadedFileName(config)
	extractedFile := h.jasperExtractedFileName(config)
	return []string{
		fmt.Sprintf("cd \"%s\"", h.Distro.CuratorDir),
		fmt.Sprintf("curl -LO '%s/%s'", config.URL, downloadedFile),
		fmt.Sprintf("tar xzf '%s'", downloadedFile),
		fmt.Sprintf("chmod +x '%s'", extractedFile),
		fmt.Sprintf("rm -f '%s'", downloadedFile),
	}
}

// FetchJasperCommandsWithPath is the same as FetchJasperCommand but sets the
// PATH variable to path for each command.
func (h *Host) FetchJasperCommandWithPath(config evergreen.JasperConfig, path string) string {
	cmds := h.fetchJasperCommands(config)
	for i := range cmds {
		cmds[i] = fmt.Sprintf("PATH=%s %s", path, cmds[i])
	}
	return strings.Join(cmds, " && ")
}

func (h *Host) jasperDownloadedFileName(config evergreen.JasperConfig) string {
	os, arch := h.Distro.Platform()
	return fmt.Sprintf("%s-%s-%s-%s.tar.gz", config.DownloadFileName, os, arch, config.Version)
}

func (h *Host) jasperExtractedFileName(config evergreen.JasperConfig) string {
	if h.Distro.IsWindows() {
		return config.BinaryName + ".exe"
	}
	return config.BinaryName
}

// BootstrapScript creates the user data script to bootstrap the host.
func (h *Host) BootstrapScript(config evergreen.JasperConfig) string {
	if h.Distro.IsWindows() {
		cmds := []string{
			h.FetchJasperCommandWithPath(config, "/bin"),
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
		return strings.Join(commands, "\r\n")
	}
	return strings.Join([]string{"#!/bin/bash", h.FetchJasperCommand(config), h.ForceReinstallJasperCommand(config)}, "\n")
}
