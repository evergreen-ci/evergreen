package host

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/userdata"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	jcli "github.com/mongodb/jasper/cli"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"gopkg.in/yaml.v3"
)

const OutputBufferSize = 1000

// SetupCommand returns the command to run the host setup script.
func (h *Host) SetupCommand() string {
	cmd := fmt.Sprintf("cd %s && ./%s host setup", h.Distro.HomeDir(), h.Distro.BinaryName())

	if h.Distro.SetupAsSudo {
		cmd += " --setup_as_sudo"
	}

	cmd += fmt.Sprintf(" --working_directory=%s", h.Distro.WorkDir)

	return cmd
}

func ShCommandWithSudo(script string, sudo bool) []string {
	args := []string{}
	if sudo {
		args = append(args, "sudo")
	}
	return append(args, "sh", script)
}

func ChmodCommandWithSudo(script string, sudo bool) []string {
	args := []string{}
	if sudo {
		args = append(args, "sudo")
	}
	return append(args, "chmod", "+x", script)
}

// CurlCommand returns the command to download the evergreen client.
func (h *Host) CurlCommand(env evergreen.Environment) (string, error) {
	cmds, err := h.curlCommands(env, "")
	if err != nil {
		return "", errors.WithStack(err)
	}
	return strings.Join(cmds, " && "), nil
}

// CurlCommandWithRetry is the same as CurlCommand but retries the request.
func (h *Host) CurlCommandWithRetry(env evergreen.Environment, numRetries, maxRetrySecs int) (string, error) {
	var retryArgs string
	if numRetries != 0 && maxRetrySecs != 0 {
		retryArgs = " " + curlRetryArgs(numRetries, maxRetrySecs)
	}
	cmds, err := h.curlCommands(env, retryArgs)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return strings.Join(cmds, " && "), nil
}

// CurlCommandWithDefaultRetry is the same as CurlCommandWithRetry using the
// default retry parameters.
func (h *Host) CurlCommandWithDefaultRetry(env evergreen.Environment) (string, error) {
	return h.CurlCommandWithRetry(env, curlDefaultNumRetries, curlDefaultMaxSecs)
}

func (h *Host) curlCommands(env evergreen.Environment, curlArgs string) ([]string, error) {
	if env.ClientConfig().S3URLPrefix == "" {
		return nil, errors.Errorf("S3 downloads are not configured")
	}

	cmds := []string{
		fmt.Sprintf("cd %s", h.Distro.HomeDir()),
	}
	if h.Distro.IsMacOS() {
		// Ensure the Evergreen client file is deleted on MacOS hosts before
		// downloading it again. This is necessary to fix a MacOS-specific issue
		// where if the host has System Integrity Protection (SIP) enabled and
		// it runs an Evergreen client that has a problem (e.g. the binary was
		// not signed by Apple), running that client results in SIGKILL. The
		// SIGKILL issue will persist even if a valid agent is downloaded to
		// replace it. Removing the binary before downloading it is the only
		// known workaround to ensure that MacOS can run the client.
		cmds = append(cmds, fmt.Sprintf("rm -f %s", h.Distro.BinaryName()))
	}
	cmds = append(cmds,
		// Download the agent from S3. Include -f to return an error code from curl if the HTTP request
		// fails (e.g. it receives 403 Forbidden or 404 Not Found).
		fmt.Sprintf("curl -fLO %s%s", h.Distro.S3ClientURL(env), curlArgs),
		fmt.Sprintf("chmod +x %s", h.Distro.BinaryName()),
	)

	return cmds, nil
}

// Constants representing default curl retry arguments.
const (
	curlDefaultNumRetries = 10
	curlDefaultMaxSecs    = 100
)

func curlRetryArgs(numRetries, maxSecs int) string {
	return fmt.Sprintf("--retry %d --retry-max-time %d", numRetries, maxSecs)
}

const (
	defaultSSHTimeout = 2 * time.Minute
)

// DefaultSSHPort is the default port to connect to hosts using SSH.
const DefaultSSHPort = 22

// GetSSHPort returns the host's SSH port. If no port has been specified, it
// returns the standard SSH port.
func (h *Host) GetSSHPort() int {
	if h.SSHPort == 0 {
		return DefaultSSHPort
	}
	return h.SSHPort
}

// GetSSHOptions returns the options to SSH into this host from an application
// server.
func (h *Host) GetSSHOptions(settings *evergreen.Settings) ([]string, error) {
	var keyPaths []string
	for _, pair := range settings.SSHKeyPairs {
		if _, err := os.Stat(pair.PrivatePath(settings)); err == nil {
			keyPaths = append(keyPaths, pair.PrivatePath(settings))
		} else {
			grip.Warning(message.WrapError(err, message.Fields{
				"message": "could not find local SSH key file (this should only be a temporary problem until SSH keys are written to the static host)",
				"host_id": h.Id,
				"key":     pair.Name,
			}))
		}
	}

	if _, err := os.Stat(settings.KanopySSHKeyPath); err == nil {
		keyPaths = append(keyPaths, settings.KanopySSHKeyPath)
	}
	if len(keyPaths) == 0 {
		return nil, errors.New("no SSH identity files available")
	}

	var opts []string
	for _, path := range keyPaths {
		opts = append(opts, "-i", path)
	}

	var hasKnownHostsFile bool
	var distroPortOption string

	// Apply distro-level SSH options first.
	for _, opt := range h.Distro.SSHOptions {
		opt = strings.TrimSpace(opt)
		// Do not set the port yet since a host-specific port might be set.
		if strings.HasPrefix(opt, "Port") {
			distroPortOption = opt
			continue
		}
		opts = append(opts, "-o", opt)
		if strings.HasPrefix(opt, "UserKnownHostsFile") {
			hasKnownHostsFile = true
		}
	}

	if !hasKnownHostsFile {
		opts = append(opts, "-o", "UserKnownHostsFile=/dev/null")
	}

	// If a port has been specified for this host in particular, prefer to use
	// that. Otherwise, fall back to using the distro SSH port if it's been set.
	if h.SSHPort != 0 {
		opts = append(opts, "-o", fmt.Sprintf("Port=%d", h.SSHPort))
	} else if distroPortOption != "" {
		opts = append(opts, "-o", distroPortOption)
	}

	// Run SSH without pseudo-terminal allocation because we will never connect
	// with an interactive terminal.
	opts = append(opts, "-o", "RequestTTY=no")

	return opts, nil
}

// RunSSHCommand runs an SSH command on the host with the default SSH timeout.
func (h *Host) RunSSHCommand(ctx context.Context, cmd string) (string, error) {
	return h.RunSSHCommandWithTimeout(ctx, cmd, time.Duration(0))
}

// RunSSHCommandWithTimeout runs an SSH command on the host with the given
// timeout.
func (h *Host) RunSSHCommandWithTimeout(ctx context.Context, cmd string, timeout time.Duration) (string, error) {
	return h.runSSHCommandWithOutput(ctx, func(c *jasper.Command) *jasper.Command {
		return c.Add([]string{cmd})
	}, timeout)
}

// RunSSHShellScript runs a shell script on a remote host over SSH with the
// default SSH timeout.
func (h *Host) RunSSHShellScript(ctx context.Context, script string, sudo bool, sudoUser string) (string, error) {
	return h.RunSSHShellScriptWithTimeout(ctx, script, sudo, sudoUser, time.Duration(0))
}

// RunSSHShellScript runs a shell script on a remote host over SSH with the
// given timeout.
func (h *Host) RunSSHShellScriptWithTimeout(ctx context.Context, script string, sudo bool, sudoUser string, timeout time.Duration) (string, error) {
	// We read the shell script verbatim from stdin  (i.e. with "bash -s"
	// instead of "bash -c") to avoid shell parsing errors.
	return h.runSSHCommandWithOutput(ctx, func(c *jasper.Command) *jasper.Command {
		var cmd []string
		if sudo {
			cmd = append(cmd, "sudo")
			if sudoUser != "" {
				cmd = append(cmd, fmt.Sprintf("--user=%s", sudoUser))
			}
		}
		cmd = append(cmd, "bash", "-s")
		return c.Add(cmd).SetInputBytes([]byte(script))
	}, timeout)
}

func (h *Host) runSSHCommandWithOutput(ctx context.Context, addCommands func(*jasper.Command) *jasper.Command, timeout time.Duration) (string, error) {
	env := evergreen.GetEnvironment()
	sshOpts, err := h.GetSSHOptions(env.Settings())
	if err != nil {
		return "", errors.Wrap(err, "getting host's SSH options")
	}

	output := util.NewMBCappedWriter()

	var cancel context.CancelFunc
	if timeout != 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	} else {
		ctx, cancel = context.WithTimeout(ctx, defaultSSHTimeout)
		defer cancel()
	}

	err = addCommands(env.JasperManager().CreateCommand(ctx)).
		Host(h.Host).User(h.User).
		ExtendRemoteArgs(sshOpts...).
		SetCombinedWriter(output).
		Run(ctx)

	return output.String(), errors.Wrap(err, "running SSH command")
}

// FetchAndReinstallJasperCommands returns the command to fetch Jasper and
// restart the service with the latest version.
func (h *Host) FetchAndReinstallJasperCommands(settings *evergreen.Settings) string {
	return strings.Join([]string{
		h.FetchJasperCommand(settings.HostJasper),
		h.ForceReinstallJasperCommand(settings),
	}, " && ")
}

// ForceReinstallJasperCommand returns the command to stop the Jasper service
// (if it's running), delete the current Jasper service configuration (if it
// exists), install the new configuration, and restart the service.
func (h *Host) ForceReinstallJasperCommand(settings *evergreen.Settings) string {
	params := []string{"--host=0.0.0.0", fmt.Sprintf("--port=%d", settings.HostJasper.Port)}
	if h.Distro.BootstrapSettings.JasperCredentialsPath != "" {
		params = append(params, fmt.Sprintf("--creds_path=%s", h.Distro.AbsPathNotCygwinCompatible(h.Distro.BootstrapSettings.JasperCredentialsPath)))
	}

	if user := h.Distro.BootstrapSettings.ServiceUser; user != "" {
		if h.Distro.IsWindows() {
			user = `.\\` + user
		}
		params = append(params, fmt.Sprintf("--user=%s", user))
		if h.ServicePassword != "" {
			params = append(params, fmt.Sprintf("--password='%s'", h.ServicePassword))
		}
	} else if h.User != "" {
		params = append(params, fmt.Sprintf("--user=%s", h.User))
	}

	if settings.Splunk.SplunkConnectionInfo.Populated() && h.StartedBy == evergreen.User {
		params = append(params,
			fmt.Sprintf("--splunk_url=%s", settings.Splunk.SplunkConnectionInfo.ServerURL),
			fmt.Sprintf("--splunk_token_path=%s", h.Distro.AbsPathNotCygwinCompatible(h.splunkTokenFilePath())),
		)
		if settings.Splunk.SplunkConnectionInfo.Channel != "" {
			params = append(params, fmt.Sprintf("--splunk_channel=%s", settings.Splunk.SplunkConnectionInfo.Channel))
		}
	}

	for _, envVar := range h.Distro.BootstrapSettings.Env {
		params = append(params, fmt.Sprintf("--env '%s=%s'", envVar.Key, envVar.Value))
	}

	if os, _ := h.Distro.Platform(); os == "linux" {
		if numProcs := h.Distro.BootstrapSettings.ResourceLimits.NumProcesses; numProcs != 0 {
			params = append(params, fmt.Sprintf("--limit_num_procs=%d", numProcs))
		}
		if numFiles := h.Distro.BootstrapSettings.ResourceLimits.NumFiles; numFiles != 0 {
			params = append(params, fmt.Sprintf("--limit_num_files=%d", numFiles))
		}
		if numTasks := h.Distro.BootstrapSettings.ResourceLimits.NumTasks; numTasks != 0 {
			params = append(params, fmt.Sprintf("--limit_num_tasks=%d", numTasks))
		}
		if lockedMem := h.Distro.BootstrapSettings.ResourceLimits.LockedMemoryKB; lockedMem != 0 {
			params = append(params, fmt.Sprintf("--limit_locked_memory=%d", lockedMem))
		}
		if virtualMem := h.Distro.BootstrapSettings.ResourceLimits.VirtualMemoryKB; virtualMem != 0 {
			params = append(params, fmt.Sprintf("--limit_virtual_memory=%d", virtualMem))
		}
	}

	for _, ps := range h.Distro.BootstrapSettings.PreconditionScripts {
		params = append(params, fmt.Sprintf("--precondition=%s", ps.Path))
	}

	return h.jasperServiceCommand(settings.HostJasper, jcli.ServiceForceReinstallCommand, params...)
}

// RestartJasperCommand returns the command to restart the Jasper service with
// the existing configuration.
func (h *Host) RestartJasperCommand(config evergreen.HostJasperConfig) string {
	return h.jasperServiceCommand(config, jcli.ServiceRestartCommand)
}

// QuietUninstallJasperCommand returns the command to uninstall the Jasper
// service. If the service is already not installed, this no-ops.
func (h *Host) QuietUninstallJasperCommand(config evergreen.HostJasperConfig) string {
	return h.jasperServiceCommand(config, jcli.ServiceUninstallCommand, "--quiet")
}

func (h *Host) jasperServiceCommand(config evergreen.HostJasperConfig, subCmd string, args ...string) string {
	cmd := append(jcli.BuildServiceCommand(h.JasperBinaryFilePath(config)), subCmd, jcli.RPCService)
	cmd = append(cmd, args...)
	// Jasper service commands generally need elevated privileges to execute. On
	// Windows, this is assuming that the command is already being run by
	// Administrator.
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
		fmt.Sprintf("cd %s", h.Distro.BootstrapSettings.JasperBinaryDir),
		fmt.Sprintf("curl -fLO %s/%s %s", config.URL, downloadedFile, curlRetryArgs(curlDefaultNumRetries, curlDefaultMaxSecs)),
		fmt.Sprintf("tar xzf '%s'", downloadedFile),
		fmt.Sprintf("chmod +x '%s'", extractedFile),
		fmt.Sprintf("rm -f '%s'", downloadedFile),
	}
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

// JasperBinaryFilePath returns the full path to the Jasper binary.
func (h *Host) JasperBinaryFilePath(config evergreen.HostJasperConfig) string {
	return filepath.Join(h.Distro.BootstrapSettings.JasperBinaryDir, h.jasperBinaryFileName(config))
}

// GenerateUserDataProvisioningScript creates a script to provision a host that
// is provisioning in user data. If, for some reason, this script gets
// interrupted, there's no guarantee that it will succeed if run again, since we
// cannot enforce idempotency on the setup script.
func (h *Host) GenerateUserDataProvisioningScript(ctx context.Context, settings *evergreen.Settings, creds *certdepot.Credentials, githubAppToken string, moduleTokens []string) (string, error) {
	var err error
	checkProvisioningStarted := h.CheckUserDataProvisioningStartedCommand()

	var setupUserCmds string
	if h.Distro.IsWindows() {
		setupUserCmds, err = h.SetupServiceUserCommands(ctx)
		if err != nil {
			return "", errors.Wrap(err, "getting commands to set up service user")
		}
	}

	var postFetchClient string
	if h.StartedBy == evergreen.User {
		// Start the host with an agent monitor to run tasks.
		if postFetchClient, err = h.StartAgentMonitorRequest(ctx, settings); err != nil {
			return "", errors.Wrap(err, "creating command to start agent monitor")
		}
	} else if h.ProvisionOptions != nil && h.UserHost {
		// Set up a spawn host.
		if postFetchClient, err = h.SpawnHostSetupCommands(settings); err != nil {
			return "", errors.Wrap(err, "creating commands to load task data")
		}
		if h.ProvisionOptions.TaskId != "" {
			// We have to run this in the Cygwin shell in order for git clone to
			// use the correct SSH key. Additionally, since this can take a long
			// time to download all the task data, user data may time out this
			// operation, which would prevent user data from completing and the
			// host would be stuck in provisioning. To alleviate this, we
			// download the task data using Jasper.
			var fetchCmd []string
			if h.ProvisionOptions.TaskSync {
				fetchCmd = []string{h.Distro.ShellBinary(), "-l", "-c", strings.Join(h.SpawnHostPullTaskSyncCommand(), " ")}
			} else {
				fetchCmd = []string{h.Distro.ShellBinary(), "-l", "-c", strings.Join(h.SpawnHostGetTaskDataCommand(ctx, githubAppToken, moduleTokens), " ")}
			}
			var getTaskDataCmd string
			getTaskDataCmd, err = h.buildLocalJasperClientRequest(
				settings.HostJasper,
				strings.Join([]string{jcli.ManagerCommand, jcli.CreateProcessCommand}, " "),
				options.Create{
					Args: fetchCmd,
					Tags: []string{evergreen.HostFetchTag},
				})
			if err != nil {
				return "", errors.Wrap(err, "constructing Jasper command to fetch task data")
			}
			postFetchClient += " && " + getTaskDataCmd
		}
	}

	makeJasperDirs := h.MakeJasperDirsCommand()

	markDone := h.MarkUserDataProvisioningDoneCommand()
	if err != nil {
		return "", errors.Wrap(err, "creating command to mark when user data is done")
	}

	fixJasperDirsOwner := h.ChangeJasperDirsOwnerCommand()

	setupScriptCmds, err := h.setupScriptCommands(settings)
	if err != nil {
		return "", errors.Wrap(err, "creating setup script")
	}

	writeCredentialsCmds, err := h.WriteJasperCredentialsFilesCommands(settings.Splunk.SplunkConnectionInfo, creds)
	if err != nil {
		return "", errors.Wrap(err, "creating commands to write Jasper credentials file")
	}

	setupJasperCmds := []string{makeJasperDirs, writeCredentialsCmds}
	if writePreconditionScriptsCmd := h.WriteJasperPreconditionScriptsCommands(); len(writePreconditionScriptsCmd) != 0 {
		setupJasperCmds = append(setupJasperCmds, writePreconditionScriptsCmd)
	}
	setupJasperCmds = append(setupJasperCmds,
		h.FetchJasperCommand(settings.HostJasper),
		h.ForceReinstallJasperCommand(settings),
		fixJasperDirsOwner,
	)

	shellPrefix := []string{"set -o errexit", "set -o verbose"}
	shellCmds := append(shellPrefix, checkProvisioningStarted, setupUserCmds, setupScriptCmds)
	shellCmds = append(shellCmds, setupJasperCmds...)
	shellCmds = append(shellCmds, postFetchClient, markDone)

	return strings.Join(shellCmds, "\n"), nil
}

// changeJasperDirsOwnerCommand returns the command to ensure that the Jasper
// directories have proper permissions.
func (h *Host) ChangeJasperDirsOwnerCommand() string {
	return strings.Join([]string{
		h.changeOwnerCommand(h.Distro.BootstrapSettings.JasperBinaryDir),
		h.changeOwnerCommand(filepath.Dir(h.Distro.BootstrapSettings.JasperCredentialsPath)),
		h.changeOwnerCommand(h.Distro.BootstrapSettings.ClientDir),
	}, " && ")
}

// MakeJasperDirsCommand creates the directories with the correct permissions to
// provision the host with Jasper.
func (h *Host) MakeJasperDirsCommand() string {
	return fmt.Sprintf("mkdir -m 777 -p %s %s %s",
		h.Distro.BootstrapSettings.JasperBinaryDir,
		filepath.Dir(h.Distro.BootstrapSettings.JasperCredentialsPath),
		h.Distro.BootstrapSettings.ClientDir,
	)
}

// changeOwnerCommand returns the command to modify the given file on the host
// to be owned by the distro owner.
func (h *Host) changeOwnerCommand(paths ...string) string {
	if len(paths) == 0 {
		return ""
	}
	cmd := strings.Join(append([]string{"chown", "-R", h.User}, paths...), " ")
	if h.Distro.IsWindows() {
		return cmd
	}
	return "sudo " + cmd
}

// CheckUserDataProvisioningStarted checks whether the provisioning script has
// already run on a user data host. If it has, it exits. Otherwise, it creates
// the file marking it as started.
func (h *Host) CheckUserDataProvisioningStartedCommand() string {
	path := h.UserDataProvisioningStartedFile()
	makeFileCmd := fmt.Sprintf("mkdir -m 777 -p %s && touch %s", filepath.Dir(path), path)
	return fmt.Sprintf("[ -a %s ] && exit || %s", path, makeFileCmd)
}

// SetupServiceUserCommands returns the commands to create a passwordless
// service user in the Administrator group in Windows. It also creates the
// service user's password if none is set.
func (h *Host) SetupServiceUserCommands(ctx context.Context) (string, error) {
	if !h.Distro.IsWindows() {
		return "", nil
	}
	if h.Distro.BootstrapSettings.ServiceUser == "" {
		return "", errors.New("distro is missing service user name")
	}
	if h.ServicePassword == "" {
		if err := h.CreateServicePassword(ctx); err != nil {
			return "", errors.Wrap(err, "generating service user's password")
		}
	}

	cmd := func(cmd string) string {
		return fmt.Sprintf("cmd.exe /C '%s'", cmd)
	}

	loginServicePermCmd := fmt.Sprintf("editrights -u %s -a SeServiceLogonRight", h.Distro.BootstrapSettings.ServiceUser)
	return strings.Join(
		[]string{
			// Create new user.
			cmd(fmt.Sprintf("net user %s %s /add", h.Distro.BootstrapSettings.ServiceUser, h.ServicePassword)),
			// Add the user to the Administrators group.
			cmd(fmt.Sprintf("net localgroup Administrators %s /add", h.Distro.BootstrapSettings.ServiceUser)),
			// Allow the user to run the service by granting the "Log on as a
			// service" right.
			loginServicePermCmd,
		}, "\n"), nil
}

const passwordCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

func generatePassword(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = passwordCharset[rand.Int()%len(passwordCharset)]
	}
	return string(b)
}

// CreateServicePassword creates the password for the host's service user.
func (h *Host) CreateServicePassword(ctx context.Context) error {
	var password string
	var valid bool
	for i := 0; i < 1000; i++ {
		password = generatePassword(12)
		if valid = ValidateRDPPassword(password); valid {
			break
		}
	}
	if !valid {
		return errors.New("generating valid service password")
	}

	err := UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{ServicePasswordKey: password}},
	)
	if err != nil {
		return errors.Wrap(err, "updating service password")
	}
	h.ServicePassword = password
	return nil
}

// each regex matches one of the 5 categories listed here:
// https://technet.microsoft.com/en-us/library/cc786468(v=ws.10).aspx
var passwordRegexps = []*regexp.Regexp{
	regexp.MustCompile(`[\p{Ll}]`), // lowercase letter
	regexp.MustCompile(`[\p{Lu}]`), // uppercase letter
	regexp.MustCompile(`[0-9]`),
	regexp.MustCompile(`[~!@#$%^&*_\-+=|\\\(\){}\[\]:;"'<>,.?/` + "`]"),
	regexp.MustCompile(`[\p{Lo}]`), // letters without upper/lower variants (ex: Japanese)
}

// XXX: if modifying any of the password validation logic, you changes must
// also be ported into public/static/js/directives/directives.spawn.js
func ValidateRDPPassword(password string) bool {
	// Golang regex doesn't support lookarounds, so we can't use
	// the regex as found in public/static/js/directives/directives.spawn.js
	if len([]rune(password)) < 6 || len([]rune(password)) > 255 {
		return false
	}

	// valid passwords need to match 3 of 5 categories listed on:
	// https://technet.microsoft.com/en-us/library/cc786468(v=ws.10).aspx
	matchedCategories := 0
	for _, regex := range passwordRegexps {
		if regex.MatchString(password) {
			matchedCategories++
		}
	}

	return matchedCategories >= 3
}

// buildLocalJasperClientRequest builds the command string to a Jasper CLI to
// make a request to the local Jasper service. This builds a valid command
// assuming the CLI is on the same local machine as the Jasper service. To make
// requests to a remote Jasper service using RPC, make the request through
// JasperClient instead.
func (h *Host) buildLocalJasperClientRequest(config evergreen.HostJasperConfig, subCmd string, input interface{}) (string, error) {
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return "", errors.Wrap(err, "marshalling input as JSON")
	}

	flags := fmt.Sprintf("--service=%s --port=%d --creds_path=%s", jcli.RPCService, config.Port, h.Distro.AbsPathNotCygwinCompatible(h.Distro.BootstrapSettings.JasperCredentialsPath))
	clientInput := fmt.Sprintf("<<EOF\n%s\nEOF", inputBytes)

	return strings.Join([]string{
		strings.Join(jcli.BuildClientCommand(h.JasperBinaryFilePath(config)), " "),
		subCmd,
		flags,
		clientInput,
	}, " "), nil
}

// WriteJasperCredentialsFilesCommands builds the command to write the Jasper
// credentials and Splunk credentials to files.
func (h *Host) WriteJasperCredentialsFilesCommands(splunk send.SplunkConnectionInfo, creds *certdepot.Credentials) (string, error) {
	exportedCreds, err := creds.Export()
	if err != nil {
		return "", errors.Wrap(err, "exporting host Jasper credentials to file format")
	}
	writeFileContentCmd := func(path, content string) string {
		return fmt.Sprintf("echo '%s' > %s", content, path)
	}

	cmds := []string{
		writeFileContentCmd(h.Distro.BootstrapSettings.JasperCredentialsPath, string(exportedCreds)),
		fmt.Sprintf("chmod 666 %s", h.Distro.BootstrapSettings.JasperCredentialsPath),
	}

	if splunk.Populated() && h.StartedBy == evergreen.User {
		cmds = append(cmds, writeFileContentCmd(h.splunkTokenFilePath(), splunk.Token))
		cmds = append(cmds, fmt.Sprintf("chmod 666 %s", h.splunkTokenFilePath()))
	}

	return strings.Join(cmds, " && "), nil
}

// WriteJasperPreconditionScriptsCommands returns the command to write the
// Jasper precondition scripts to files.
func (h *Host) WriteJasperPreconditionScriptsCommands() string {
	var cmds []string
	for _, ps := range h.Distro.BootstrapSettings.PreconditionScripts {
		cmds = append(cmds, fmt.Sprintf("tee %s <<'EOF'\n%s\nEOF", ps.Path, ps.Script))
		cmds = append(cmds, fmt.Sprintf("chmod 755 %s", ps.Path))
	}
	return strings.Join(cmds, "\n")
}

func (h *Host) splunkTokenFilePath() string {
	if h.Distro.BootstrapSettings.JasperCredentialsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(h.Distro.BootstrapSettings.JasperCredentialsPath), "splunk.txt")
}

// RunJasperProcess makes a request to the host's Jasper service to create the
// process with the given options, wait for its completion, and returns the
// output from it.
func (h *Host) RunJasperProcess(ctx context.Context, env evergreen.Environment, opts *options.Create) ([]string, error) {
	client, err := h.JasperClient(ctx, env)
	if err != nil {
		return nil, errors.Wrap(err, "getting Jasper client")
	}
	defer func() {
		grip.Warning(message.WrapError(client.CloseConnection(), message.Fields{
			"message": "could not close connection to Jasper",
			"host_id": h.Id,
			"distro":  h.Distro.Id,
		}))
	}()

	inMemoryLoggerExists := false
	for _, logger := range opts.Output.Loggers {
		if logger.Type() == options.LogInMemory {
			inMemoryLoggerExists = true
			break
		}
	}
	if !inMemoryLoggerExists {
		logger, loggerErr := jasper.NewInMemoryLogger(OutputBufferSize)
		if err != nil {
			return nil, errors.Wrap(loggerErr, "creating new in-memory logger")
		}
		opts.Output.Loggers = append(opts.Output.Loggers, logger)
	}

	proc, err := client.CreateProcess(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating process")
	}

	catcher := grip.NewBasicCatcher()
	if _, err = proc.Wait(ctx); err != nil {
		catcher.Wrap(err, "problem waiting for process completion")
	}

	logStream, err := client.GetLogStream(ctx, proc.ID(), OutputBufferSize)
	if err != nil {
		catcher.Wrap(err, "can't get output of process")
	}

	return logStream.Logs, catcher.Resolve()
}

// StartJasperProcess makes a request to the host's Jasper service to start a
// process with the given options without waiting for its completion.
func (h *Host) StartJasperProcess(ctx context.Context, env evergreen.Environment, opts *options.Create) (string, error) {
	client, err := h.JasperClient(ctx, env)
	if err != nil {
		return "", errors.Wrap(err, "getting Jasper client")
	}
	defer func() {
		grip.Warning(message.WrapError(client.CloseConnection(), message.Fields{
			"message": "could not close connection to Jasper",
			"host_id": h.Id,
			"distro":  h.Distro.Id,
		}))
	}()

	proc, err := client.CreateProcess(ctx, opts)
	if err != nil {
		return "", errors.Wrap(err, "creating Jasper process")
	}

	return proc.ID(), nil
}

// GetJasperProcess makes a request to the host's Jasper service to get a started
// process's status. Processes with an output logger return output.
func (h *Host) GetJasperProcess(ctx context.Context, env evergreen.Environment, processID string) (complete bool, output string, err error) {
	client, err := h.JasperClient(ctx, env)
	if err != nil {
		return false, "", errors.Wrap(err, "getting Jasper client")
	}
	defer func() {
		grip.Warning(message.WrapError(client.CloseConnection(), message.Fields{
			"message": "could not close connection to Jasper",
			"host_id": h.Id,
			"distro":  h.Distro.Id,
		}))
	}()

	proc, err := client.Get(ctx, processID)
	if err != nil {
		return false, "", errors.Wrap(err, "getting Jasper process")
	}
	info := proc.Info(ctx)
	if !info.Complete {
		return false, "", nil
	}

	// using MaxInt32 because we can assume the in-mem buffer size is small
	// enough and want to get ALL logs in the buffer with one call to
	// GetLogsStream.
	logStream, err := client.GetLogStream(ctx, processID, math.MaxInt32)
	if err != nil {
		return true, "", errors.Wrap(err, "getting output of Jasper process")
	}

	return true, strings.Join(logStream.Logs, "\n"), nil
}

const jasperDialTimeout = 15 * time.Second

// JasperClient returns a remote client that communicates with this host's
// Jasper service.
func (h *Host) JasperClient(ctx context.Context, env evergreen.Environment) (remote.Manager, error) {
	if (h.Distro.LegacyBootstrap() || h.Distro.LegacyCommunication()) && h.NeedsReprovision != ReprovisionToLegacy {
		return nil, errors.New("legacy host does not support remote Jasper process management")
	}
	if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodNone {
		return nil, errors.New("hosts without any provisioning method cannot use Jasper")
	}

	settings := env.Settings()
	if h.Distro.BootstrapSettings.Communication == distro.CommunicationMethodSSH || h.NeedsReprovision == ReprovisionToLegacy {
		sshOpts, err := h.GetSSHOptions(settings)
		if err != nil {
			return nil, errors.Wrap(err, "getting host's SSH options")
		}

		var remoteOpts options.Remote
		remoteOpts.Host = h.Host
		remoteOpts.User = h.User
		remoteOpts.Args = sshOpts
		clientOpts := jcli.ClientOptions{
			BinaryPath:          h.JasperBinaryFilePath(settings.HostJasper),
			Type:                jcli.RPCService,
			Port:                settings.HostJasper.Port,
			CredentialsFilePath: h.Distro.AbsPathNotCygwinCompatible(h.Distro.BootstrapSettings.JasperCredentialsPath),
		}

		return jcli.NewSSHClient(clientOpts, remoteOpts)
	}
	if h.Distro.BootstrapSettings.Communication == distro.CommunicationMethodRPC {
		creds, err := h.JasperClientCredentials(ctx, env)
		if err != nil {
			return nil, errors.Wrap(err, "getting client credentials to communicate with the host's Jasper service")
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
			return nil, errors.Wrapf(err, "resolving Jasper service address at '%s'", addrStr)
		}

		dialCtx, cancel := context.WithTimeout(ctx, jasperDialTimeout)
		defer cancel()

		return remote.NewRPCClient(dialCtx, serviceAddr, creds)
	}

	return nil, errors.Errorf("host does not have recognized communication method '%s'", h.Distro.BootstrapSettings.Communication)
}

// setupScriptCommands returns the commands contained in the setup script with
// the expansions applied from the settings.
func (h *Host) setupScriptCommands(settings *evergreen.Settings) (string, error) {
	if h.SpawnOptions.SpawnedByTask || h.Distro.Setup == "" {
		return "", nil
	}

	expansions := util.NewExpansions(settings.Expansions)
	setupScript, err := expansions.ExpandString(h.Distro.Setup)
	if err != nil {
		return "", errors.Wrap(err, "expanding setup script variables")
	}
	return setupScript, nil
}

// StartAgentMonitorRequest builds the Jasper client request that starts the
// agent monitor on the host. The host secret is created if it doesn't exist
// yet.
func (h *Host) StartAgentMonitorRequest(ctx context.Context, settings *evergreen.Settings) (string, error) {
	if h.Secret == "" {
		if err := h.CreateSecret(ctx, false); err != nil {
			return "", errors.Wrap(err, "creating host secret")
		}
	}

	return h.buildLocalJasperClientRequest(
		settings.HostJasper,
		strings.Join([]string{jcli.ManagerCommand, jcli.CreateProcessCommand}, " "),
		h.AgentMonitorOptions(settings),
	)
}

// withTaggedProcs runs the given handler on all Jasper processes on this host
// matching the given tag.
func (h *Host) withTaggedProcs(ctx context.Context, env evergreen.Environment, tag string, handleTaggedProcs func(taggedProcs []jasper.Process) error) error {
	client, err := h.JasperClient(ctx, env)
	if err != nil {
		return errors.Wrap(err, "getting Jasper client")
	}

	defer func() {
		grip.Warning(message.WrapError(client.CloseConnection(), message.Fields{
			"message": "could not close connection to Jasper",
			"host_id": h.Id,
			"distro":  h.Distro.Id,
		}))
	}()

	procs, err := client.Group(ctx, tag)
	if err != nil {
		return errors.Wrapf(err, "getting processes with tag '%s'", tag)
	}

	return handleTaggedProcs(procs)
}

func (h *Host) CheckTaskDataFetched(ctx context.Context, env evergreen.Environment) error {
	timer := time.NewTimer(0)
	defer timer.Stop()

	const (
		checkAttempts      = 10
		checkRetryMinDelay = 3 * time.Second
		checkRetryMaxDelay = time.Minute
	)

	return h.withTaggedProcs(ctx, env, evergreen.HostFetchTag, func(procs []jasper.Process) error {
		grip.WarningWhen(len(procs) > 1, message.Fields{
			"message":   "host is attempting to fetch task data multiple times",
			"num_procs": len(procs),
			"host_id":   h.Id,
			"distro":    h.Distro.Id,
		})
		catcher := grip.NewBasicCatcher()
		for _, proc := range procs {
			err := utility.Retry(
				ctx,
				func() (bool, error) {
					if proc.Complete(ctx) {
						return false, nil
					}
					return true, errors.New(evergreen.FetchingTaskDataUnfinishedError)
				}, utility.RetryOptions{
					MaxAttempts: checkAttempts,
					MinDelay:    checkRetryMinDelay,
					MaxDelay:    checkRetryMaxDelay,
				})
			// If we see a process that's completed then we can suppress errors from erroneous duplicates.
			if err == nil {
				return nil
			}
			catcher.Add(err)
		}
		return catcher.Resolve()
	})
}

// WithAgentMonitor runs the given handler on all agent monitor processes
// running on the host.
func (h *Host) WithAgentMonitor(ctx context.Context, env evergreen.Environment, handleAgentMonitor func(procs []jasper.Process) error) error {
	return h.withTaggedProcs(ctx, env, evergreen.AgentMonitorTag, func(procs []jasper.Process) error {
		return handleAgentMonitor(procs)
	})
}

// StopAgentMonitor stops the agent monitor (if it is running) on the host via
// its Jasper service. On legacy hosts, this is a no-op.
// Stopping the agent monitor manually like this is only necessary for legacy
// reasons. There are some static hosts that have been quarantined for a long
// time, and they could have very old versions of the agent monitor running on
// them. Newer versions of the agent monitor shut themselves down when
// appropriate, making this operation unnecessary. However, we have no guarantee
// on how long ago hosts were quarantined and when they might be unquarantined,
// meaning we can't get rid of this unless we know every single static host has
// is running a relatively recent version of the agent monitor.
func (h *Host) StopAgentMonitor(ctx context.Context, env evergreen.Environment) error {
	if (h.Distro.LegacyBootstrap() && h.NeedsReprovision != ReprovisionToLegacy) || h.NeedsReprovision == ReprovisionToNew {
		return nil
	}

	return h.WithAgentMonitor(ctx, env, func(procs []jasper.Process) error {
		catcher := grip.NewBasicCatcher()
		var numRunning int
		for _, proc := range procs {
			if proc.Running(ctx) {
				numRunning++
				catcher.Wrapf(proc.Signal(ctx, syscall.SIGTERM), "signalling agent monitor process with ID '%s'", proc.ID())
			}
		}
		grip.WarningWhen(numRunning > 1, message.Fields{
			"message": fmt.Sprintf("host should be running at most one agent monitor, but found %d", len(procs)),
			"host_id": h.Id,
			"distro":  h.Distro.Id,
		})

		return catcher.Resolve()
	})
}

// AgentCommand returns the arguments to start the agent. If executablePath is not specified, it
// will be assumed to be in the regular place (only pass this for container distros)
func (h *Host) AgentCommand(settings *evergreen.Settings, executablePath string) []string {
	if executablePath == "" {
		executablePath = h.Distro.AbsPathCygwinCompatible(h.Distro.HomeDir(), h.Distro.BinaryName())
	}
	return []string{
		executablePath,
		"agent",
		fmt.Sprintf("--api_server=%s", settings.Api.URL),
		"--mode=host",
		fmt.Sprintf("--host_id=%s", h.Id),
		fmt.Sprintf("--host_secret=%s", h.Secret),
		fmt.Sprintf("--provider=%s", h.Distro.Provider),
		"--log_output=file",
		fmt.Sprintf("--log_prefix=%s", filepath.Join(h.Distro.WorkDir, "agent")),
		fmt.Sprintf("--working_directory=%s", h.Distro.WorkDir),
		"--cleanup",
	}
}

// AgentMonitorOptions assembles the input to a Jasper request to start the
// agent monitor.
func (h *Host) AgentMonitorOptions(settings *evergreen.Settings) *options.Create {
	clientPath := h.Distro.AbsPathNotCygwinCompatible(h.Distro.BootstrapSettings.ClientDir, h.Distro.BinaryName())
	credsPath := h.Distro.AbsPathNotCygwinCompatible(h.Distro.BootstrapSettings.JasperCredentialsPath)
	shellPath := h.Distro.AbsPathNotCygwinCompatible(h.Distro.BootstrapSettings.ShellPath)

	args := append(h.AgentCommand(settings, ""), "monitor")
	args = append(args,
		fmt.Sprintf("--client_path=%s", clientPath),
		fmt.Sprintf("--distro=%s", h.Distro.Id),
		fmt.Sprintf("--shell_path=%s", shellPath),
		fmt.Sprintf("--jasper_port=%d", settings.HostJasper.Port),
		fmt.Sprintf("--credentials=%s", credsPath),
		"--log_output=file",
		fmt.Sprintf("--log_prefix=%s", filepath.Join(h.Distro.WorkDir, "agent.monitor")),
	)

	return &options.Create{
		Args: args,
		Tags: []string{evergreen.AgentMonitorTag},
	}
}

// AddPublicKeyScript returns the shell script to add a public key to the
// authorized keys file on the host. If the public key already exists on the
// host, the authorized keys file will not be modified.
func (h *Host) AddPublicKeyScript(pubKey string) string {
	authorizedKeysFile := h.Distro.GetAuthorizedKeysFile()
	// Any trailing/leading newlines have to be removed from the public key or
	// else the shell script may not work as intended.
	pubKey = strings.TrimSpace(pubKey)

	return fmt.Sprintf("grep -qxF \"%s\" %s || echo \"\n%s\" >> %s", pubKey, authorizedKeysFile, pubKey, authorizedKeysFile)
}

// SpawnHostSetupCommands returns the commands to handle setting up a spawn
// host with the evergreen binary and config file for the owner.
func (h *Host) SpawnHostSetupCommands(settings *evergreen.Settings) (string, error) {
	if h.ProvisionOptions == nil {
		return "", errors.New("missing spawn host provisioning options")
	}
	if h.ProvisionOptions.OwnerId == "" {
		return "", errors.New("missing spawn host owner")
	}

	conf, err := h.spawnHostConfig(settings)
	if err != nil {
		return "", errors.Wrap(err, "creating spawn host configuration settings")
	}

	return h.spawnHostSetupConfigDirCommands(conf), nil
}

// spawnHostSetupConfigDirCommands the shell script that sets up the
// config directory on a spawn host. In particular, it makes the client binary
// directory, puts both the evergreen yaml and the client into it, and attempts
// to add the directory to the path.
func (h *Host) spawnHostSetupConfigDirCommands(conf []byte) string {
	return strings.Join([]string{
		fmt.Sprintf("mkdir -m 777 -p %s", h.spawnHostConfigDir()),
		// We have to do this because on most of the distro (but not all of
		// them), the evergreen config file is already baked into the AMI and
		// owned by the privileged user. This is allowed to fail since some
		// distros don't have the evergreen config file.
		fmt.Sprintf("(%s || true)", h.changeOwnerCommand(h.spawnHostConfigFile())),
		// Note: this will likely fail if the configuration file content
		// contains quotes.
		fmt.Sprintf("echo \"%s\" > %s", conf, h.spawnHostConfigFile()),
		fmt.Sprintf("chmod +x %s", filepath.Join(h.AgentBinary())),
		fmt.Sprintf("cp %s %s", h.AgentBinary(), h.spawnHostConfigDir()),
		fmt.Sprintf("(echo '\nexport PATH=\"${PATH}:%s\"\n' >> %s/.profile || true; echo '\nexport PATH=\"${PATH}:%s\"\n' >> %s/.bash_profile || true)", h.spawnHostConfigDir(), h.Distro.HomeDir(), h.spawnHostConfigDir(), h.Distro.HomeDir()),
		fmt.Sprintf("(%s || true)", h.changeOwnerCommand(filepath.Join(h.Distro.HomeDir(), ".profile"), filepath.Join(h.Distro.HomeDir(), ".bash_profile"))),
	}, " && ")
}

// AgentBinary returns the path to the evergreen agent binary.
func (h *Host) AgentBinary() string {
	return filepath.Join(h.Distro.HomeDir(), h.Distro.BinaryName())
}

// spawnHostConfigDir returns the directory containing the CLI and evergreen
// yaml for a spawn host.
func (h *Host) spawnHostConfigDir() string {
	return filepath.Join(h.Distro.HomeDir(), "cli_bin")
}

// spawnHostConfigFile returns the path to the evergreen yaml for a spawn host.
func (h *Host) spawnHostConfigFile() string {
	return filepath.Join(h.Distro.HomeDir(), evergreen.DefaultEvergreenConfig)
}

// spawnHostCLIConfig returns the evergreen configuration for a spawn host CLI
// in yaml format.
func (h *Host) spawnHostConfig(settings *evergreen.Settings) ([]byte, error) {
	owner, err := user.FindOne(user.ById(h.ProvisionOptions.OwnerId))
	if err != nil {
		return nil, errors.Wrapf(err, "getting owner '%s' for host", h.ProvisionOptions.OwnerId)
	}

	conf := struct {
		User          string `yaml:"user"`
		APIKey        string `yaml:"api_key"`
		APIServerHost string `yaml:"api_server_host"`
		UIServerHost  string `yaml:"ui_server_host"`
	}{
		User:          owner.Id,
		APIKey:        owner.APIKey,
		APIServerHost: settings.Api.URL + "/api",
		UIServerHost:  settings.Ui.Url,
	}

	return yaml.Marshal(conf)
}

// SpawnHostGetTaskDataCommand returns the command that fetches the task data
// for a spawn host.
func (h *Host) SpawnHostGetTaskDataCommand(ctx context.Context, githubAppToken string, moduleTokens []string) []string {
	s := []string{
		// We can't use the absolute path for the binary because we always run
		// it in a Cygwin context on Windows.
		h.AgentBinary(),
		"-c", h.Distro.AbsPathNotCygwinCompatible(h.spawnHostConfigFile()),
		"fetch",
		"-t", h.ProvisionOptions.TaskId,
		"--source", "--artifacts",
		// We can't use a Cygwin path for the working directory because it's
		// symlinked with a Cygwin symlink, which is not a real directory or
		// Windows shortcut.
		"--dir", h.Distro.WorkDir,
	}

	if githubAppToken != "" || moduleTokens != nil {
		s = append(s, "--use-app-token")
		s = append(s, "--revoke-tokens")
	}

	if githubAppToken != "" {
		s = append(s, "--token", githubAppToken)
	}

	for _, moduleToken := range moduleTokens {
		s = append(s, "-m", moduleToken)
	}

	return s
}

// SpawnHostPullTaskSyncCommand returns the command that pulls the task sync
// directory for a spawn host.
func (h *Host) SpawnHostPullTaskSyncCommand() []string {
	return []string{
		h.AgentBinary(),
		"-c", h.Distro.AbsPathNotCygwinCompatible(h.spawnHostConfigFile()),
		"pull",
		"--task", h.ProvisionOptions.TaskId,
		"--dir", h.Distro.WorkDir,
	}
}

const (
	userDataProvisioningStartedFile = "user_data_started"
	userDataProvisioningDoneFile    = "user_data_done"
)

// UserDataProvisioningStartedFile returns the path to the user data
// provisioning started marker file.
func (h *Host) UserDataProvisioningStartedFile() string {
	return filepath.Join(h.Distro.BootstrapSettings.JasperBinaryDir, userDataProvisioningStartedFile)
}

// UserDataProvisioningDoneFile returns the path to the user data provisioning
// done marker file.
func (h *Host) UserDataProvisioningDoneFile() string {
	return filepath.Join(h.Distro.BootstrapSettings.JasperBinaryDir, userDataProvisioningDoneFile)
}

// MarkUserDataDoneProvisioningCommand creates the command to make the marker
// file indicating user data provisioning has finished executing.
func (h *Host) MarkUserDataProvisioningDoneCommand() string {
	path := h.UserDataProvisioningDoneFile()
	return fmt.Sprintf("touch %s", path)
}

// SetUserDataHostProvisioned sets the host to running if it was bootstrapped
// with user data but has not yet been marked as done provisioning.
func (h *Host) SetUserDataHostProvisioned(ctx context.Context) error {
	if h.Distro.BootstrapSettings.Method != distro.BootstrapMethodUserData {
		return nil
	}

	if h.Status != evergreen.HostStarting {
		return nil
	}

	if !h.Provisioned {
		return nil
	}

	if err := h.UpdateStartingToRunning(ctx); err != nil {
		return errors.Wrap(err, "marking host as done provisioning itself and now running")
	}

	grip.Info(message.Fields{
		"message":              "host successfully provisioned",
		"host_id":              h.Id,
		"distro":               h.Distro.Id,
		"time_to_running_secs": time.Since(h.CreationTime).Seconds(),
	})

	return nil
}

// GenerateFetchProvisioningScriptUserData creates the user data script to fetch
// the host provisioning script.
func (h *Host) GenerateFetchProvisioningScriptUserData(ctx context.Context, env evergreen.Environment) (*userdata.Options, error) {
	if h.Secret == "" {
		if err := h.CreateSecret(ctx, false); err != nil {
			return nil, errors.Wrap(err, "creating host secret")
		}
	}

	fetchClient, err := h.CurlCommandWithDefaultRetry(env)
	if err != nil {
		return nil, errors.Wrap(err, "creating curl command for evergreen client")
	}
	// User data runs as the privileged user, so ensure that the binary has
	// permissions that allow it to be modified after user data is finished.
	fixClientOwner := h.changeOwnerCommand(filepath.Join(h.Distro.HomeDir(), h.Distro.BinaryName()))
	fetchScriptCmd := strings.Join([]string{
		filepath.Join(h.Distro.HomeDir(), h.Distro.BinaryName()),
		"host",
		"provision",
		fmt.Sprintf("--api_server=%s", env.Settings().Api.URL),
		fmt.Sprintf("--host_id=%s", h.Id),
		fmt.Sprintf("--host_secret=%s", h.Secret),
		fmt.Sprintf("--provider=%s", h.Distro.Provider),
		fmt.Sprintf("--working_dir=%s", h.Distro.AbsPathNotCygwinCompatible(h.Distro.BootstrapSettings.JasperBinaryDir)),
		fmt.Sprintf("--shell_path=%s", h.Distro.ShellBinary()),
	}, " ")

	makeJasperDirs := h.MakeJasperDirsCommand()

	cmds := []string{
		makeJasperDirs,
		fetchClient,
		fixClientOwner,
		fetchScriptCmd,
	}

	var directive userdata.Directive
	if h.Distro.IsWindows() {
		for i := range cmds {
			cmds[i] = fmt.Sprintf("%s -l -c %s", h.Distro.ShellBinary(), util.PowerShellQuotedString(cmds[i]))
		}
		directive = userdata.PowerShellScript
	} else {
		shellPrefix := []string{"set -o errexit", "set -o verbose"}
		cmds = append(shellPrefix, cmds...)
		directive = userdata.ShellScript + userdata.Directive(h.Distro.BootstrapSettings.ShellPath)
	}

	return &userdata.Options{
		Content:   strings.Join(cmds, "\n"),
		Directive: directive,
	}, nil
}
