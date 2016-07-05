package hostinit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/util"
	"gopkg.in/mgo.v2"
)

const (
	SCPTimeout         = time.Minute
	setupScriptName    = "setup.sh"
	teardownScriptName = "teardown.sh"
)

// Error indicating another hostinit got to the setup first.
var (
	ErrHostAlreadyInitializing = errors.New("Host already initializing")
)

// Longest duration allowed for running setup script.
var (
	SSHTimeoutSeconds = int64(300) // 5 minutes
)

// HostInit is responsible for running setup scripts on Evergreen hosts.
type HostInit struct {
	Settings *evergreen.Settings
}

// setupReadyHosts runs the distro setup script of all hosts that are up and reachable.
func (init *HostInit) setupReadyHosts() error {
	// set SSH timeout duration
	if timeoutSecs := init.Settings.HostInit.SSHTimeoutSeconds; timeoutSecs <= 0 {
		evergreen.Logger.Logf(slogger.WARN, "SSH timeout set to %vs (<= 0s) using %vs instead", timeoutSecs, SSHTimeoutSeconds)
	} else {
		SSHTimeoutSeconds = timeoutSecs
	}

	// find all hosts in the uninitialized state
	uninitializedHosts, err := host.Find(host.IsUninitialized)
	if err != nil {
		return fmt.Errorf("error fetching uninitialized hosts: %v", err)
	}

	evergreen.Logger.Logf(slogger.DEBUG, "There are %v uninitialized hosts",
		len(uninitializedHosts))

	// used for making sure we don't exit before a setup script is done
	wg := &sync.WaitGroup{}

	for _, h := range uninitializedHosts {

		// check whether or not the host is ready for its setup script to be run
		ready, err := init.IsHostReady(&h)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error checking host %v for readiness: %v",
				h.Id, err)
			continue
		}

		// if the host isn't ready (for instance, it might not be up yet), skip it
		if !ready {
			evergreen.Logger.Logf(slogger.DEBUG, "Host %v not ready for setup", h.Id)
			continue
		}

		evergreen.Logger.Logf(slogger.INFO, "Running setup script for host %v", h.Id)

		// kick off the setup, in its own goroutine, so pending setups don't have
		// to wait for it to finish
		wg.Add(1)
		go func(h host.Host) {

			if err := init.ProvisionHost(&h, ProvisionOptions{}); err != nil {
				evergreen.Logger.Logf(slogger.ERROR, "Error provisioning host %v: %v", h.Id, err)

				// notify the admins of the failure
				subject := fmt.Sprintf("%v Evergreen provisioning failure on %v",
					notify.ProvisionFailurePreface, h.Distro.Id)
				hostLink := fmt.Sprintf("%v/host/%v", init.Settings.Ui.Url, h.Id)
				message := fmt.Sprintf("Provisioning failed on %v host -- %v: see %v",
					h.Distro.Id, h.Id, hostLink)
				if err := notify.NotifyAdmins(subject, message, init.Settings); err != nil {
					evergreen.Logger.Errorf(slogger.ERROR, "Error sending email: %v", err)
				}
			}

			wg.Done()

		}(h)

	}

	// let all setup routines finish
	wg.Wait()

	return nil
}

// IsHostReady returns whether or not the specified host is ready for its setup script
// to be run.
func (init *HostInit) IsHostReady(host *host.Host) (bool, error) {

	// fetch the appropriate cloud provider for the host
	cloudMgr, err := providers.GetCloudManager(host.Distro.Provider, init.Settings)
	if err != nil {
		return false,
			fmt.Errorf("failed to get cloud manager for provider %v: %v", host.Distro.Provider, err)
	}

	// ask for the instance's status
	hostStatus, err := cloudMgr.GetInstanceStatus(host)
	if err != nil {
		return false, fmt.Errorf("error checking instance status of host %v: %v", host.Id, err)
	}

	// if the host isn't up yet, we can't do anything
	if hostStatus != cloud.StatusRunning {
		return false, nil
	}

	// set the host's dns name, if it is not set
	if host.Host == "" {

		// get the DNS name for the host
		hostDNS, err := cloudMgr.GetDNSName(host)
		if err != nil {
			return false, fmt.Errorf("error checking DNS name for host %v: %v", host.Id, err)
		}

		// sanity check for the host DNS name
		if hostDNS == "" {
			return false, fmt.Errorf("instance %v is running but not returning a DNS name",
				host.Id)
		}

		// update the host's DNS name
		if err := host.SetDNSName(hostDNS); err != nil {
			return false, fmt.Errorf("error setting DNS name for host %v: %v", host.Id, err)
		}

	}

	// check if the host is reachable via SSH
	cloudHost, err := providers.GetCloudHost(host, init.Settings)
	if err != nil {
		return false, fmt.Errorf("failed to get cloud host for %v: %v", host.Id, err)
	}
	reachable, err := cloudHost.IsSSHReachable()
	if err != nil {
		return false, fmt.Errorf("error checking if host %v is reachable: %v", host.Id, err)
	}

	// at this point, we can run the setup if the host is reachable
	return reachable, nil
}

// setupHost runs the specified setup script for an individual host. Returns
// the output from running the script remotely, as well as any error that
// occurs. If the script exits with a non-zero exit code, the error will be non-nil.
func (init *HostInit) setupHost(targetHost *host.Host) (string, error) {

	// fetch the appropriate cloud provider for the host
	cloudMgr, err := providers.GetCloudManager(targetHost.Provider, init.Settings)
	if err != nil {
		return "",
			fmt.Errorf("failed to get cloud manager for host %v with provider %v: %v",
				targetHost.Id, targetHost.Provider, err)
	}

	// mark the host as initializing
	if err := targetHost.SetInitializing(); err != nil {
		if err == mgo.ErrNotFound {
			return "", ErrHostAlreadyInitializing
		} else {
			return "", fmt.Errorf("database error: %v", err)
		}
	}

	/* TESTING ONLY
	setupDebugSSHTunnel(path_to_ssh_key, targetHost.User, targetHost.Host)
	*/

	// run the function scheduled for when the host is up
	err = cloudMgr.OnUp(targetHost)
	if err != nil {
		// if this fails it is probably due to an API hiccup, so we keep going.
		evergreen.Logger.Logf(slogger.WARN, "OnUp callback failed for host '%v': '%v'", targetHost.Id, err)
	}
	cloudHost, err := providers.GetCloudHost(targetHost, init.Settings)
	if err != nil {
		return "", fmt.Errorf("failed to get cloud host for %v: %v", targetHost.Id, err)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return "", fmt.Errorf("error getting ssh options for host %v: %v", targetHost.Id, err)
	}

	if targetHost.Distro.Teardown != "" {
		err = init.copyScript(targetHost, teardownScriptName, targetHost.Distro.Teardown)
		if err != nil {
			return "", fmt.Errorf("error copying script %v to host %v: %v",
				teardownScriptName, targetHost.Id, err)
		}
	}

	if targetHost.Distro.Setup != "" {
		err = init.copyScript(targetHost, setupScriptName, targetHost.Distro.Setup)
		if err != nil {
			return "", fmt.Errorf("error copying script %v to host %v: %v",
				setupScriptName, targetHost.Id, err)
		}
		logs, err := hostutil.RunRemoteScript(targetHost, setupScriptName, sshOptions)
		if err != nil {
			return logs, fmt.Errorf("error running setup script over ssh: %v", err)
		}
		return logs, nil
	}
	return "", nil
}

// copyScript writes a given script as file "name" to the target host. This works
// by creating a local copy of the script on the runner's machine, scping it over
// then removing the local copy.
func (init *HostInit) copyScript(target *host.Host, name, script string) error {
	// parse the hostname into the user, host and port
	hostInfo, err := util.ParseSSHInfo(target.Host)
	if err != nil {
		return err
	}
	user := target.Distro.User
	if hostInfo.User != "" {
		user = hostInfo.User
	}

	// create a temp file for the script
	file, err := ioutil.TempFile("", name)
	if err != nil {
		return fmt.Errorf("error creating temporary script file: %v", err)
	}
	defer func() {
		file.Close()
		os.Remove(file.Name())
	}()

	expanded, err := init.expandScript(script)
	if err != nil {
		return fmt.Errorf("error expanding script for host %v: %v", target.Id, err)
	}
	if _, err := io.WriteString(file, expanded); err != nil {
		return fmt.Errorf("error writing local script: %v", err)
	}

	cloudHost, err := providers.GetCloudHost(target, init.Settings)
	if err != nil {
		return fmt.Errorf("failed to get cloud host for %v: %v", target.Id, err)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return fmt.Errorf("error getting ssh options for host %v: %v", target.Id, err)
	}

	var scpCmdStderr bytes.Buffer
	scpCmd := &command.ScpCommand{
		Source:         file.Name(),
		Dest:           name,
		Stdout:         &scpCmdStderr,
		Stderr:         &scpCmdStderr,
		RemoteHostName: hostInfo.Hostname,
		User:           user,
		Options:        append([]string{"-P", hostInfo.Port}, sshOptions...),
	}
	err = util.RunFunctionWithTimeout(scpCmd.Run, SCPTimeout)
	if err != nil {
		if err == util.ErrTimedOut {
			scpCmd.Stop()
			return fmt.Errorf("scp-ing script timed out")
		}
		return fmt.Errorf("error (%v) copying script to remote machine: %v",
			err, scpCmdStderr.String())
	}
	return nil
}

// Build the setup script that will need to be run on the specified host.
func (init *HostInit) expandScript(s string) (string, error) {
	// replace expansions in the script
	exp := command.NewExpansions(init.Settings.Expansions)
	script, err := exp.ExpandString(s)
	if err != nil {
		return "", fmt.Errorf("expansions error: %v", err)
	}
	return script, err
}

//
type ProvisionOptions struct {
	// LoadCLI indicates (if set) that while provisioning the host, the CLI binary should
	// be placed onto the host after startup.
	LoadCLI bool

	// TaskId if non-empty will trigger the CLI tool to fetch source and artifacts for the given task.
	// Ignored if LoadCLI is false.
	TaskId string

	// Owner is the user associated with the host used to populate any necessary metadata.
	Owner *user.DBUser
}

// Provision the host, and update the database accordingly.
func (init *HostInit) ProvisionHost(h *host.Host, opts ProvisionOptions) error {

	// run the setup script
	evergreen.Logger.Logf(slogger.INFO, "Setting up host %v", h.Id)
	output, err := init.setupHost(h)

	// deal with any errors that occured while running the setup
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Error running setup script: %v", err)

		// another hostinit process beat us there
		if err == ErrHostAlreadyInitializing {
			evergreen.Logger.Logf(slogger.DEBUG, "Attempted to initialize already initializing host %v", h.Id)
			return nil
		}

		alerts.RunHostProvisionFailTriggers(h)
		event.LogProvisionFailed(h.Id, output)

		// setup script failed, mark the host's provisioning as failed
		if err := h.SetUnprovisioned(); err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "unprovisioning host %v failed: %v", h.Id, err)
		}

		return fmt.Errorf("error initializing host %v: %v", h.Id, err)

	}

	evergreen.Logger.Logf(slogger.INFO, "Setup complete for host %v", h.Id)

	if opts.LoadCLI && opts.Owner != nil {
		evergreen.Logger.Logf(slogger.INFO, "Uploading client binary to host %v", h.Id)
		lcr, err := init.LoadClient(h, opts.Owner)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Failed to load client binary onto host %v: %v", h.Id, err)
		} else if err == nil && len(opts.TaskId) > 0 {
			evergreen.Logger.Logf(slogger.INFO, "Fetching data for task %v onto host %v", opts.TaskId, h.Id)
			err = init.fetchRemoteTaskData(opts.TaskId, lcr.BinaryPath, lcr.ConfigPath, h)
			evergreen.Logger.Logf(slogger.ERROR, "Failed to fetch data onto host %v: %v", h.Id, err)
		}
	}

	// the setup was successful. update the host accordingly in the database
	if err := h.MarkAsProvisioned(); err != nil {
		return fmt.Errorf("error marking host %v as provisioned: %v", err)
	}

	evergreen.Logger.Logf(slogger.INFO, "Host %v successfully provisioned", h.Id)

	return nil
}

// LocateCLIBinary returns the (absolute) path to the CLI binary for the given architecture, based
// on the system settings. Returns an error if the file does not exist.
func LocateCLIBinary(settings *evergreen.Settings, architecture string) (string, error) {
	clientsSubDir := "clients"
	if settings.ClientBinariesDir != "" {
		clientsSubDir = settings.ClientBinariesDir
	}

	var path string
	if filepath.IsAbs(clientsSubDir) {
		path = filepath.Join(clientsSubDir, architecture, "main")
	} else {
		path = filepath.Join(evergreen.FindEvergreenHome(), clientsSubDir, architecture, "main")
	}
	_, err := os.Stat(path)
	if err != nil {
		return path, err
	}
	return filepath.Abs(path)
}

// LoadClientResult indicates the locations on a target host where the CLI binary and it's config
// file have been written to.
type LoadClientResult struct {
	BinaryPath string
	ConfigPath string
}

// LoadClient places the evergreen command line client on the host, places a copy of the user's
// settings onto the host, and makes the binary appear in the $PATH when the user logs in.
// If successful, returns an instance of LoadClientResult which contains the paths where the
// binary and config file were written to.
func (init *HostInit) LoadClient(target *host.Host, user *user.DBUser) (*LoadClientResult, error) {
	// Make sure we have the binary we want to upload - if it hasn't been built for the given
	// architecture, fail early
	cliBinaryPath, err := LocateCLIBinary(init.Settings, target.Distro.Arch)
	if err != nil {
		return nil, fmt.Errorf("Couldn't locate CLI binary for upload: %v", err)
	}

	// 1. mkdir the destination directory on the host,
	//    and modify ~/.profile so the target binary will be on the $PATH
	targetDir := "cli_bin"
	hostSSHInfo, err := util.ParseSSHInfo(target.Host)
	if err != nil {
		return nil, fmt.Errorf("error parsing ssh info %v: %v", target.Host, err)
	}

	cloudHost, err := providers.GetCloudHost(target, init.Settings)
	if err != nil {
		return nil, fmt.Errorf("Failed to get cloud host for %v: %v", target.Id, err)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return nil, fmt.Errorf("Error getting ssh options for host %v: %v", target.Id, err)
	}
	sshOptions = append(sshOptions, "-o", "UserKnownHostsFile=/dev/null")

	mkdirOutput := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}

	// Create the directory for the binary to be uploaded into.
	// Also, make a best effort to add the binary's location to $PATH upon login. If we can't do
	// this successfully, the command will still succeed, it just means that the user will have to
	// use an absolute path (or manually set $PATH in their shell) to execute it.
	makeShellCmd := &command.RemoteCommand{
		CmdString:      fmt.Sprintf("mkdir -m 777 -p ~/%s && (echo 'PATH=$PATH:~/%s' >> ~/.profile || true; echo 'PATH=$PATH:~/%s' >> ~/.bash_profile || true)", targetDir, targetDir, targetDir),
		Stdout:         mkdirOutput,
		Stderr:         mkdirOutput,
		RemoteHostName: hostSSHInfo.Hostname,
		User:           target.User,
		Options:        append([]string{"-p", hostSSHInfo.Port}, sshOptions...),
	}

	scpOut := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}
	// run the make shell command with a timeout
	err = util.RunFunctionWithTimeout(makeShellCmd.Run, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("error running setup command for cli, %v: '%v'", mkdirOutput.Buffer.String(), err)
	}
	// place the binary into the directory
	scpSetupCmd := &command.ScpCommand{
		Source:         cliBinaryPath,
		Dest:           fmt.Sprintf("~/%s/evergreen", targetDir),
		Stdout:         scpOut,
		Stderr:         scpOut,
		RemoteHostName: hostSSHInfo.Hostname,
		User:           target.User,
		Options:        append([]string{"-P", hostSSHInfo.Port}, sshOptions...),
	}

	// run the command to scp the setup script with a timeout
	err = util.RunFunctionWithTimeout(scpSetupCmd.Run, 3*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("error running SCP command for cli, %v: '%v'", scpOut.Buffer.String(), err)
	}

	// 4. Write a settings file for the user that owns the host, and scp it to the directory
	outputStruct := model.CLISettings{
		User:          user.Id,
		APIKey:        user.APIKey,
		APIServerHost: init.Settings.ApiUrl + "/api",
		UIServerHost:  init.Settings.Ui.Url,
	}
	outputJSON, err := json.Marshal(outputStruct)
	if err != nil {
		return nil, err
	}

	tempFileName, err := util.WriteTempFile("", outputJSON)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempFileName)

	scpYmlCommand := &command.ScpCommand{
		Source:         tempFileName,
		Dest:           fmt.Sprintf("~/%s/.evergreen.yml", targetDir),
		Stdout:         scpOut,
		Stderr:         scpOut,
		RemoteHostName: hostSSHInfo.Hostname,
		User:           target.User,
		Options:        append([]string{"-P", hostSSHInfo.Port}, sshOptions...),
	}
	err = util.RunFunctionWithTimeout(scpYmlCommand.Run, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("error running SCP command for evergreen.yml, %v: '%v'", scpOut.Buffer.String(), err)
	}

	return &LoadClientResult{
		BinaryPath: fmt.Sprintf("~/%s/evergreen", targetDir),
		ConfigPath: fmt.Sprintf("~/%s/.evergreen.yml", targetDir),
	}, nil
}

func (init *HostInit) fetchRemoteTaskData(taskId, cliPath, confPath string, target *host.Host) error {
	hostSSHInfo, err := util.ParseSSHInfo(target.Host)
	if err != nil {
		return fmt.Errorf("error parsing ssh info %v: %v", target.Host, err)
	}

	cloudHost, err := providers.GetCloudHost(target, init.Settings)
	if err != nil {
		return fmt.Errorf("Failed to get cloud host for %v: %v", target.Id, err)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return fmt.Errorf("Error getting ssh options for host %v: %v", target.Id, err)
	}
	sshOptions = append(sshOptions, "-o", "UserKnownHostsFile=/dev/null")

	/* TESTING ONLY
	setupDebugSSHTunnel(path_to_ssh_keys, target.User, hostSSHInfo.Hostname)
	*/

	// When testing, use this writer to force a copy of the output to be written to standard out so
	// that remote command failures also show up in server log output.
	//cmdOutput := io.MultiWriter(&util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}, os.Stdout)

	cmdOutput := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}
	makeShellCmd := &command.RemoteCommand{
		CmdString:      fmt.Sprintf("%s -c '%s' fetch -t %s --source --artifacts --dir '%s'", cliPath, confPath, taskId, target.Distro.WorkDir),
		Stdout:         cmdOutput,
		Stderr:         cmdOutput,
		RemoteHostName: hostSSHInfo.Hostname,
		User:           target.User,
		Options:        append([]string{"-p", hostSSHInfo.Port}, sshOptions...),
	}

	// run the make shell command with a timeout
	err = util.RunFunctionWithTimeout(makeShellCmd.Run, 15*time.Minute)
	return err
}

// this helper is for local testing--it allows developers to get around
// firewall restrictions by opening up an SSH tunnel.
func setupDebugSSHTunnel(keyPath, hostUser, hostName string) {
	// Note for testing - when running locally, if your API Server's URL is behind a gateway (i.e. not a
	// static IP) the next step will fail because the API server will not be reachable.
	// If you want it to reach your local API server, execute a command here that sets up a reverse ssh tunnel:
	// ssh -f -N -T -R 8080:localhost:8080 -o UserKnownHostsFile=/dev/null
	// ... or, add a time.Sleep() here that gives you enough time to log in and edit the config
	// on the spawnhost manually.
	fmt.Println("starting up tunnel.")
	tunnelCmd := exec.Command("ssh", "-f", "-N", "-T", "-R", "8080:localhost:8080", "-o", "UserKnownHostsFile=/dev/null", "-o", "StrictHostKeyChecking=no", "-i", keyPath, fmt.Sprintf("%s@%s", hostUser, hostName))
	err := tunnelCmd.Start()
	if err != nil {
		fmt.Println("Setting up SSH tunnel failed - manual tunnel setup required.")
		// Give the developer a 30 second grace period to set up the tunnel.
		time.Sleep(30 * time.Second)
	}
	fmt.Println("Tunnel setup complete, starting fetch in 10 seconds...")
	time.Sleep(10 * time.Second)
}
