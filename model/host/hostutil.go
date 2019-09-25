package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	jcli "github.com/mongodb/jasper/cli"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

func (h *Host) SetupCommand() string {
	cmd := fmt.Sprintf("%s host setup", filepath.Join(h.Distro.HomeDir(), h.Distro.BinaryName()))

	if h.Distro.SetupAsSudo {
		cmd += " --setup_as_sudo"
	}

	cmd += fmt.Sprintf(" --working_directory=%s", h.Distro.WorkDir)

	return cmd
}

// TearDownCommand returns a command for running a teardown script on a host.
func (h *Host) TearDownCommand() string {
	return fmt.Sprintf("%s host teardown", filepath.Join(h.Distro.HomeDir(), h.Distro.BinaryName()))
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

// CurlCommand returns the command to curl the evergreen client.
func (h *Host) CurlCommand(settings *evergreen.Settings) string {
	return h.CurlCommandWithRetry(settings, 0, 0)
}

// CurlCommandWithRetry is the same as CurlCommand but retries the request if
// numRetries and maxRetrySecs are non-zero.
func (h *Host) CurlCommandWithRetry(settings *evergreen.Settings, numRetries, maxRetrySecs int) string {
	var retryArgs string
	if numRetries != 0 && maxRetrySecs != 0 {
		retryArgs = " " + curlRetryArgs(numRetries, maxRetrySecs)
	}
	return fmt.Sprintf("cd %s && curl -LO '%s'%s && chmod +x %s",
		h.Distro.HomeDir(),
		h.ClientURL(settings),
		retryArgs,
		h.Distro.BinaryName(),
	)
}

// Constants representing default curl retry arguments.
const (
	CurlDefaultNumRetries = 10
	CurlDefaultMaxSecs    = 100
)

func curlRetryArgs(numRetries, maxSecs int) string {
	return fmt.Sprintf("--retry %d --retry-max-time %d", numRetries, maxSecs)
}

// ClientURL returns the URL used to get the latest Evergreen client version.
func (h *Host) ClientURL(settings *evergreen.Settings) string {
	return fmt.Sprintf("%s/%s/%s",
		strings.TrimSuffix(settings.Ui.Url, "/"),
		strings.TrimSuffix(settings.ClientBinariesDir, "/"),
		h.Distro.ExecutableSubPath())
}

const (
	// sshTimeout is the timeout for SSH commands.
	sshTimeout = 2 * time.Minute
)

// GetSSHInfo returns the information necessary to SSH into this host.
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
func (h *Host) GetSSHOptions(keyPath string) ([]string, error) {
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
		SetCombinedWriter(output).
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
func (h *Host) FetchAndReinstallJasperCommand(settings *evergreen.Settings) string {
	return strings.Join([]string{
		h.FetchJasperCommand(settings.HostJasper),
		h.ForceReinstallJasperCommand(settings),
	}, " && ")
}

// ForceReinstallJasperCommand returns the command to stop the Jasper service,
// delete the current Jasper service configuration (if it exists), install the
// new configuration, and restart the service.
func (h *Host) ForceReinstallJasperCommand(settings *evergreen.Settings) string {
	params := []string{"--host=0.0.0.0", fmt.Sprintf("--port=%d", settings.HostJasper.Port)}
	if h.Distro.BootstrapSettings.JasperCredentialsPath != "" {
		params = append(params, fmt.Sprintf("--creds_path=%s", h.Distro.BootstrapSettings.JasperCredentialsPath))
	}

	if h.Distro.BootstrapSettings.ServiceUser != "" {
		params = append(params, fmt.Sprintf("--user=%s", h.Distro.BootstrapSettings.ServiceUser))
		if h.ServicePassword != "" {
			params = append(params, fmt.Sprintf("--password=%s", h.ServicePassword))
		}
	} else if h.Distro.User != "" {
		params = append(params, fmt.Sprintf("--user=%s", h.Distro.User))
	}

	if settings.Splunk.Populated() {
		params = append(params,
			fmt.Sprintf("--splunk_url=%s", settings.Splunk.ServerURL),
			fmt.Sprintf("--splunk_token=%s", settings.Splunk.Token),
		)
		if settings.Splunk.Channel != "" {
			params = append(params, fmt.Sprintf("--splunk_channel=%s", settings.Splunk.Channel))
		}
	}

	return h.jasperServiceCommand(settings.HostJasper, jcli.ForceReinstallCommand, params...)
}

// RestartJasperCommand returns the command to restart the Jasper service with
// the existing configuration.
func (h *Host) RestartJasperCommand(config evergreen.HostJasperConfig) string {
	return h.jasperServiceCommand(config, jcli.RestartCommand)
}

func (h *Host) jasperServiceCommand(config evergreen.HostJasperConfig, subCmd string, args ...string) string {
	cmd := append(jcli.BuildServiceCommand(h.jasperBinaryFilePath(config)), subCmd, jcli.RPCService)
	cmd = append(cmd, args...)
	// Jasper service commands need elevated privileges to execute. On Windows,
	// this is assuming that the command is already being run by Administrator.
	if !h.Distro.IsWindows() {
		cmd = append([]string{"sudo"}, cmd...)
	}
	return strings.Join(cmd, " ")
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
		fmt.Sprintf("mkdir -m 777 -p \"%s\"", h.Distro.BootstrapSettings.JasperBinaryDir),
		fmt.Sprintf("cd \"%s\"", h.Distro.BootstrapSettings.JasperBinaryDir),
		fmt.Sprintf("curl -LO '%s/%s' %s", config.URL, downloadedFile, curlRetryArgs(CurlDefaultNumRetries, CurlDefaultMaxSecs)),
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

// jasperBinaryFilePath returns the full path to the Jasper binary.
func (h *Host) jasperBinaryFilePath(config evergreen.HostJasperConfig) string {
	return filepath.Join(h.Distro.BootstrapSettings.JasperBinaryDir, h.jasperBinaryFileName(config))
}

// BootstrapScript creates the user data script to bootstrap the host.
func (h *Host) BootstrapScript(settings *evergreen.Settings, creds *rpc.Credentials, preJasperSetup, postJasperSetup []string) (string, error) {
	bashCmds := append([]string{"set -o errexit"}, preJasperSetup...)

	writeCredentialsCmd, err := h.WriteJasperCredentialsFileCommand(creds)
	if err != nil {
		return "", errors.Wrap(err, "could not get command to write Jasper credentials file")
	}

	if h.Distro.IsWindows() {
		setupUserCmd, err := h.SetupServiceUserCommands()
		if err != nil {
			return "", errors.Wrap(err, "could not get command to set up service user")
		}
		bashCmds = append(bashCmds,
			setupUserCmd,
			writeCredentialsCmd,
			h.FetchJasperCommandWithPath(settings.HostJasper, "/bin"),
			h.ForceReinstallJasperCommand(settings),
		)
		bashCmds = append(bashCmds, postJasperSetup...)

		bashCmdsLiteral := util.PowershellQuotedString(strings.Join(bashCmds, "\r\n"))

		powershellCmds := []string{
			"<powershell>",
			fmt.Sprintf("%s -c %s", h.Distro.BootstrapSettings.ShellPath, bashCmdsLiteral),
			"</powershell>",
		}

		return strings.Join(powershellCmds, "\r\n"), nil
	}

	bashCmds = append(bashCmds, h.FetchJasperCommand(settings.HostJasper), writeCredentialsCmd, h.ForceReinstallJasperCommand(settings))
	bashCmds = append(bashCmds, postJasperSetup...)

	return strings.Join(append([]string{"#!/bin/bash"}, bashCmds...), "\n"), nil
}

// SetupServiceUserCommands returns the commands to create a passwordless
// service user in the Administrator group in Windows.
func (h *Host) SetupServiceUserCommands() (string, error) {
	if !h.Distro.IsWindows() {
		return "", nil
	}
	if h.Distro.BootstrapSettings.ServiceUser == "" {
		return "", errors.New("distro is missing service user name")
	}
	if h.ServicePassword == "" {
		// kim: TODO: generate service password
		if err := h.GenerateServicePassword(); err != nil {
			return "", errors.Wrap(err, "could not generate service user's password")
		}
	}

	cmd := func(cmd string) string {
		return fmt.Sprintf("cmd.exe /C '%s'", cmd)
	}

	return strings.Join(
		[]string{
			// Create new passwordless user.
			cmd(fmt.Sprintf("net user %s %s /add", h.Distro.BootstrapSettings.ServiceUser, h.ServicePassword)),
			// Add the user to the Administrators group.
			cmd(fmt.Sprintf("net localgroup Administrators %s /add", h.Distro.BootstrapSettings.ServiceUser)),
			// kim: TODO: give the user the "Log on as a service" right
		},
		"\r\n"), nil
}

// GenerateServicePassword creates the password for the host's service user.
func (h *Host) GenerateServicePassword() error {
	password := util.RandomString()
	err := UpdateOne(
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{ServicePasswordKey: password}},
	)
	if err != nil {
		return err
	}
	h.ServicePassword = password
	return nil
}

// buildLocalJasperClientRequest builds the command string to a Jasper CLI to
// make a request to the local Jasper service. This builds a valid command
// assuming the CLI is on the same local machine as the Jasper service. To make
// requests to a remote Jasper service using RPC, make the request through
// JasperClient instead.
func (h *Host) buildLocalJasperClientRequest(config evergreen.HostJasperConfig, subCmd string, input interface{}) (string, error) {
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return "", errors.Wrap(err, "could not marshal input")
	}

	flags := fmt.Sprintf("--service=%s --port=%d --creds_path=%s", jcli.RPCService, config.Port, h.Distro.BootstrapSettings.JasperCredentialsPath)

	clientInput := fmt.Sprintf("<<EOF\n%s\nEOF", inputBytes)

	return strings.Join([]string{
		strings.Join(jcli.BuildClientCommand(h.jasperBinaryFilePath(config)), " "),
		subCmd,
		flags,
		clientInput,
	}, " "), nil
}

// WriteJasperCredentialsFileCommand builds the command to write the Jasper
// credentials to a file.
func (h *Host) WriteJasperCredentialsFileCommand(creds *rpc.Credentials) (string, error) {
	if h.Distro.BootstrapSettings.JasperCredentialsPath == "" {
		return "", errors.New("cannot write Jasper credentials without a credentials file path")
	}
	exportedCreds, err := creds.Export()
	if err != nil {
		return "", errors.Wrap(err, "problem exporting credentials to file format")
	}
	return fmt.Sprintf("mkdir -m 777 -p \"%s\" && cat > '%s' <<EOF\n%s\nEOF", filepath.Dir(h.Distro.BootstrapSettings.JasperCredentialsPath), h.Distro.BootstrapSettings.JasperCredentialsPath, exportedCreds), nil
}

// RunJasperProcess makes a request to the host's Jasper service to create the
// process with the given options, wait for its completion, and returns the
// output from it.
func (h *Host) RunJasperProcess(ctx context.Context, settings *evergreen.Settings, opts *options.Create) (string, error) {
	client, err := h.JasperClient(ctx, settings)
	if err != nil {
		return "", errors.Wrap(err, "could not get a Jasper client")
	}
	defer client.CloseConnection()

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
func (h *Host) StartJasperProcess(ctx context.Context, settings *evergreen.Settings, opts *options.Create) error {
	client, err := h.JasperClient(ctx, settings)
	if err != nil {
		return errors.Wrap(err, "could not get a Jasper client")
	}
	defer client.CloseConnection()

	if _, err := client.CreateProcess(ctx, opts); err != nil {
		return errors.Wrap(err, "problem creating Jasper process")
	}

	return nil
}

const jasperDialTimeout = 15 * time.Second

// JasperClient returns a remote client that communicates with this host's
// Jasper service.
func (h *Host) JasperClient(ctx context.Context, settings *evergreen.Settings) (jasper.RemoteClient, error) {
	if h.LegacyBootstrap() || h.LegacyCommunication() {
		return nil, errors.New("legacy host does not support remote Jasper process management")
	}

	if h.JasperCommunication() {
		switch h.Distro.BootstrapSettings.Communication {
		case distro.CommunicationMethodSSH:
			hostInfo, err := h.GetSSHInfo()
			if err != nil {
				return nil, errors.Wrap(err, "could not get host's SSH info")
			}
			sshOpts, err := h.GetSSHOptions(settings.Keys[h.Distro.SSHKey])
			if err != nil {
				return nil, errors.Wrap(err, "could not get host's SSH options")
			}

			remoteOpts := options.Remote{
				Host: hostInfo.Hostname,
				User: hostInfo.User,
				Args: sshOpts,
			}
			clientOpts := jcli.ClientOptions{
				BinaryPath:          h.jasperBinaryFilePath(settings.HostJasper),
				Type:                jcli.RPCService,
				Port:                settings.HostJasper.Port,
				CredentialsFilePath: h.Distro.BootstrapSettings.JasperCredentialsPath,
			}

			return jcli.NewSSHClient(remoteOpts, clientOpts, true)
		case distro.CommunicationMethodRPC:
			creds, err := h.GenerateJasperCredentials(ctx)
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

			dialCtx, cancel := context.WithTimeout(ctx, jasperDialTimeout)
			defer cancel()

			return rpc.NewClient(dialCtx, serviceAddr, creds)
		}
	}

	return nil, errors.Errorf("host does not have recognized communication method '%s'", h.Distro.BootstrapSettings.Communication)
}

// SetupScriptCommands returns the commands contained in the setup script with
// the expansions applied from the settings.
func (h *Host) SetupScriptCommands(settings *evergreen.Settings) (string, error) {
	if h.SpawnOptions.SpawnedByTask || h.Distro.Setup == "" {
		return "", nil
	}

	expansions := util.NewExpansions(settings.Expansions)
	setupScript, err := expansions.ExpandString(h.Distro.Setup)
	if err != nil {
		return "", errors.Wrap(err, "error expanding setup script variables")
	}
	return setupScript, nil
}

// StartAgentMonitorRequest builds the Jasper client request that starts the
// agent monitor on the host. The host secret is created if it doesn't exist
// yet.
func (h *Host) StartAgentMonitorRequest(settings *evergreen.Settings) (string, error) {
	if h.Secret == "" {
		if err := h.CreateSecret(); err != nil {
			return "", errors.Wrapf(err, "problem creating host secret for %s", h.Id)
		}
	}

	return h.buildLocalJasperClientRequest(
		settings.HostJasper,
		strings.Join([]string{jcli.ManagerCommand, jcli.CreateProcessCommand}, " "),
		h.AgentMonitorOptions(settings),
	)
}

// StopAgentMonitor stops the agent monitor (if it is running) on the host via
// its Jasper service . On legacy hosts, this is a no-op.
func (h *Host) StopAgentMonitor(ctx context.Context, settings *evergreen.Settings) error {
	if h.LegacyBootstrap() {
		return nil
	}

	client, err := h.JasperClient(ctx, settings)
	if err != nil {
		return errors.Wrap(err, "could not get a Jasper client")
	}
	defer client.CloseConnection()

	procs, err := client.Group(ctx, evergreen.AgentMonitorTag)
	if err != nil {
		return errors.Wrapf(err, "could not get processes with tag %s", evergreen.AgentMonitorTag)
	}

	grip.WarningWhen(len(procs) != 1, message.Fields{
		"message": fmt.Sprintf("host should be running exactly one agent monitor, but found %d", len(procs)),
		"host":    h.Id,
		"distro":  h.Distro.Id,
	})

	catcher := grip.NewBasicCatcher()
	for _, proc := range procs {
		if proc.Running(ctx) {
			catcher.Wrapf(proc.Signal(ctx, syscall.SIGTERM), "problem signalling process with ID %s", proc.ID())
		}
	}

	return catcher.Resolve()
}

// AgentMonitorOptions  assembles the input to a Jasper request to start the
// agent monitor.
func (h *Host) AgentMonitorOptions(settings *evergreen.Settings) *options.Create {
	binary := filepath.Join(h.Distro.HomeDir(), h.Distro.BinaryName())
	clientPath := filepath.Join(h.Distro.BootstrapSettings.ClientDir, h.Distro.BinaryName())

	args := []string{
		binary,
		"agent",
		fmt.Sprintf("--api_server=%s", settings.ApiUrl),
		fmt.Sprintf("--host_id=%s", h.Id),
		fmt.Sprintf("--host_secret=%s", h.Secret),
		fmt.Sprintf("--log_prefix=%s", filepath.Join(h.Distro.WorkDir, "agent")),
		fmt.Sprintf("--working_directory=%s", h.Distro.WorkDir),
		fmt.Sprintf("--logkeeper_url=%s", settings.LoggerConfig.LogkeeperURL),
		"--cleanup",
		"monitor",
		fmt.Sprintf("--log_prefix=%s", filepath.Join(h.Distro.WorkDir, "agent.monitor")),
		fmt.Sprintf("--client_url=%s", h.ClientURL(settings)),
		fmt.Sprintf("--client_path=%s", clientPath),
		fmt.Sprintf("--jasper_port=%d", settings.HostJasper.Port),
		fmt.Sprintf("--credentials=%s", h.Distro.BootstrapSettings.JasperCredentialsPath),
	}

	return &options.Create{
		Args:        args,
		Environment: buildAgentEnv(settings),
		Tags:        []string{evergreen.AgentMonitorTag},
	}
}

// buildAgentEnv returns the agent environment variables.
func buildAgentEnv(settings *evergreen.Settings) map[string]string {
	env := map[string]string{
		"S3_KEY":    settings.Providers.AWS.S3Key,
		"S3_SECRET": settings.Providers.AWS.S3Secret,
		"S3_BUCKET": settings.Providers.AWS.Bucket,
	}

	if sumoEndpoint, ok := settings.Credentials["sumologic"]; ok {
		env["GRIP_SUMO_ENDPOINT"] = sumoEndpoint
	}

	if settings.Splunk.Populated() {
		env["GRIP_SPLUNK_SERVER_URL"] = settings.Splunk.ServerURL
		env["GRIP_SPLUNK_CLIENT_TOKEN"] = settings.Splunk.Token
		if settings.Splunk.Channel != "" {
			env["GRIP_SPLUNK_CHANNEL"] = settings.Splunk.Channel
		}
	}

	return env
}

// SetupSpawnHostCommand returns the bash commands to handle setting up a spawn host.
func (h *Host) SetupSpawnHostCommand(settings *evergreen.Settings) (string, error) {
	if h.ProvisionOptions == nil {
		return "", errors.New("missing spawn host provisioning options")
	}
	if h.ProvisionOptions.OwnerId == "" {
		return "", errors.New("missing spawn host owner")
	}

	binaryPath := filepath.Join(h.Distro.HomeDir(), h.Distro.BinaryName())
	binDir := filepath.Join(h.Distro.HomeDir(), "cli_bin")
	confPath := filepath.Join(binDir, ".evergreen.yml")

	owner, err := user.FindOne(user.ById(h.ProvisionOptions.OwnerId))
	if err != nil {
		return "", errors.Wrapf(err, "could not get owner %s for host", h.ProvisionOptions.OwnerId)
	}

	confSettings := struct {
		APIKey        string `json:"api_key"`
		APIServerHost string `json:"api_server_host"`
		UIServerHost  string `json:"ui_server_host"`
		User          string `json:"user"`
	}{
		APIKey:        owner.APIKey,
		APIServerHost: settings.ApiUrl + "/api",
		UIServerHost:  settings.Ui.Url,
		User:          owner.Id,
	}

	confJSON, err := json.Marshal(confSettings)
	if err != nil {
		return "", errors.Wrap(err, "could not marshal configuration settings to JSON")
	}

	// Make the client binary directory, put both the evergreen.yml and the
	// client into it, and attempt to add the directory to the path.
	setupBinDirCmds := strings.Join([]string{
		fmt.Sprintf("mkdir -m 777 -p %s", binDir),
		fmt.Sprintf("echo '%s' > %s", confJSON, confPath),
		fmt.Sprintf("cp %s %s", binaryPath, binDir),
		fmt.Sprintf("(echo 'PATH=${PATH}:%s' >> %s/.profile || true; echo 'PATH=${PATH}:%s' >> %s/.bash_profile || true)", binDir, h.Distro.HomeDir(), binDir, h.Distro.HomeDir()),
	}, " && ")

	script := setupBinDirCmds
	if h.ProvisionOptions.TaskId != "" {
		fetchCmd := fmt.Sprintf("%s -c %s fetch -t %s --source --artifacts --dir='%s'", binaryPath, confPath, h.ProvisionOptions.TaskId, h.Distro.WorkDir)
		script += " && " + fetchCmd
	}

	return script, nil
}

const userDataDoneFileName = "user_data_done"

// UserDataDoneFilePath returns the path to the user data done marker file.
func (h *Host) UserDataDoneFilePath() (string, error) {
	if h.Distro.BootstrapSettings.ClientDir == "" {
		return "", errors.New("distro client directory must be specified")
	}

	return filepath.Join(h.Distro.BootstrapSettings.ClientDir, userDataDoneFileName), nil
}

// MarkUserDataDoneCommand creates the command to make the marker file
// indicating user data has finished executing.
func (h *Host) MarkUserDataDoneCommand() (string, error) {
	path, err := h.UserDataDoneFilePath()
	if err != nil {
		return "", errors.Wrap(err, "could not get path to user data done file")
	}

	return fmt.Sprintf("mkdir -m 777 -p %s && touch %s", h.Distro.BootstrapSettings.ClientDir, path), nil
}

// SetUserDataHostProvisioned sets the host to running if it was bootstrapped
// with user data but has not yet been marked as done provisioning.
func (h *Host) SetUserDataHostProvisioned() error {
	if h.Distro.BootstrapSettings.Method != distro.BootstrapMethodUserData {
		return nil
	}

	if h.Status != evergreen.HostProvisioning {
		return nil
	}

	if err := h.UpdateProvisioningToRunning(); err != nil {
		return errors.Wrapf(err, "could not mark host %s as done provisioning itself and now running", h.Id)
	}

	grip.Info(message.Fields{
		"message":              "host successfully provisioned",
		"host":                 h.Id,
		"distro":               h.Distro.Id,
		"time_to_running_secs": time.Since(h.CreationTime).Seconds(),
	})

	return nil
}
