package hostinit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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
	GUID     string
}

func (init *HostInit) startHosts(ctx context.Context) error {

	startTime := time.Now()

	hostsToStart, err := host.Find(host.IsUninitialized)
	if err != nil {
		return errors.Wrap(err, "error fetching uninitialized hosts")
	}

	hostPipe := make(chan Host, len(hostsToStart))
	wg := &sync.WaitGroup{}
	catcher := grip.NewSimpleCatcher()
	for _, host := range hostsToStart {
		hostPipe <- host
	}
	close(hostPipe)

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			for {
				select {
				case <-ctx.Done():
					catcher.Add(errors.New("hostinit run canceled"))
					return
				case h := <-hostPipe:
					grip.Info(message.Fields{
						"GUID":    init.GUID,
						"message": "attempting to start host",
						"hostid":  h.Id,
						"distro":  h.Distro.Id,
						"tag":     h.Tag,
					})

					cloudManager, err := providers.GetCloudManager(h.Provider, init.Settings)
					if err != nil {
						catcher.Add(errors.Wrapf(err, "error getting cloud provider for host %s", h.Id))
						continue
					}

					err = h.Remove()
					if err != nil {
						catcher.Add(errors.Wrapf(err, "error removing intent host %s", h.Id))
						continue
					}

					_, err = cloudManager.SpawnHost(&h)
					if err != nil {
						catcher.Add(errors.Wrapf(err, "error spawning host %s", h.Id))
						continue
					}

					h.Status = evergreen.HostStarting

					_, err = h.Upsert()
					if err != nil {
						catcher.Add(errors.Wrapf(err, "error updating host %v", h.Id))
						continue
					}

					grip.Infof("successfully started host %s", h.Id)
				}
			}
		}()
	}

	wg.Wait()

	grip.Info(message.Fields{
		"GUID":    init.GUID,
		"runner":  RunnerName,
		"method":  "startHosts",
		"runtime": time.Since(startTime),
	})

	return catcher.Resolve()
}

// setupReadyHosts runs the distro setup script of all hosts that are up and reachable.
func (init *HostInit) setupReadyHosts(ctx context.Context) error {
	startTime := time.Now()

	// set SSH timeout duration
	if timeoutSecs := init.Settings.HostInit.SSHTimeoutSeconds; timeoutSecs <= 0 {
		grip.Warningf("SSH timeout set to %vs (<= 0s) using %vs instead", timeoutSecs, SSHTimeoutSeconds)
	} else {
		SSHTimeoutSeconds = timeoutSecs
	}

	// find all hosts in the uninitialized state
	uninitializedHosts, err := host.Find(host.IsStarting)
	if err != nil {
		return errors.Wrap(err, "error fetching starting hosts")
	}

	grip.Debugf("There are %d uninitialized hosts", len(uninitializedHosts))

	// used for making sure we don't exit before a setup script is done
	wg := &sync.WaitGroup{}

	for _, h := range uninitializedHosts {
		if ctx.Err() != nil {
			return errors.New("hostinit run canceled")
		}

		grip.Info(message.Fields{
			"GUID":    init.GUID,
			"message": "attempting to setup host",
			"hostid":  h.Id,
			"distro":  h.Distro.Id,
			"tag":     h.Tag,
		})

		// check whether or not the host is ready for its setup script to be run
		ready, err := init.IsHostReady(&h)
		if err != nil {
			grip.Infof("Error checking host %s for readiness: %+v", h.Id, err)
			continue
		}

		// if the host isn't ready (for instance, it might not be up yet), skip it
		if !ready {
			grip.Debugf("Host %s not ready for setup", h.Id)
			continue
		}

		grip.Infoln("Running setup script for host", h.Id)

		// kick off the setup, in its own goroutine, so pending setups don't have
		// to wait for it to finish
		wg.Add(1)
		go func(h host.Host) {
			if ctx.Err() != nil {
				return
			}

			if err := init.ProvisionHost(ctx, &h); err != nil {
				grip.Errorf("Error provisioning host %s: %+v", h.Id, err)

				// notify the admins of the failure
				subject := fmt.Sprintf("%v Evergreen provisioning failure on %v",
					notify.ProvisionFailurePreface, h.Distro.Id)
				hostLink := fmt.Sprintf("%v/host/%v", init.Settings.Ui.Url, h.Id)
				message := fmt.Sprintf("Provisioning failed on %v host -- %v: see %v",
					h.Distro.Id, h.Id, hostLink)
				if err := notify.NotifyAdmins(subject, message, init.Settings); err != nil {
					grip.Errorf("Error sending email: %+v", err)
				}
			}

			wg.Done()

		}(h)

	}

	if ctx.Err() != nil {
		return errors.New("hostinit run canceled")
	}

	// let all setup routines finish
	wg.Wait()

	grip.Info(message.Fields{
		"GUID":    init.GUID,
		"runner":  RunnerName,
		"method":  "setupReadyHosts",
		"runtime": time.Since(startTime),
	})

	return nil
}

// IsHostReady returns whether or not the specified host is ready for its setup script
// to be run.
func (init *HostInit) IsHostReady(host *host.Host) (bool, error) {

	// fetch the appropriate cloud provider for the host
	cloudMgr, err := providers.GetCloudManager(host.Distro.Provider, init.Settings)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get cloud manager for provider %s",
			host.Distro.Provider)
	}

	// ask for the instance's status
	hostStatus, err := cloudMgr.GetInstanceStatus(host)
	if err != nil {
		return false, errors.Wrapf(err, "error checking instance status of host %s", host.Id)
	}

	grip.Debugf("Checking readiness for %s with DNS %s. has cloud status %s and local status: %s",
		host.Id, host.Host, hostStatus, host.Status)

	// if the host has failed, terminate it and return that this host is not ready
	if hostStatus == cloud.StatusFailed {
		err = errors.WithStack(cloudMgr.TerminateInstance(host))
		if err != nil {
			return false, err
		}
		return false, errors.Errorf("host %s terminated due to failure before setup", host.Id)
	}

	// if the host isn't up yet, we can't do anything
	if hostStatus != cloud.StatusRunning {
		return false, nil
	}

	// set the host's dns name, if it is not set
	if host.Host == "" {
		var hostDNS string

		// get the DNS name for the host
		hostDNS, err = cloudMgr.GetDNSName(host)
		if err != nil {
			return false, errors.Wrapf(err, "error checking DNS name for host %s", host.Id)
		}

		// sanity check for the host DNS name
		if hostDNS == "" {
			return false, errors.Errorf("instance %s is running but not returning a DNS name",
				host.Id)
		}

		// update the host's DNS name
		if err = host.SetDNSName(hostDNS); err != nil {
			return false, errors.Wrapf(err, "error setting DNS name for host %s", host.Id)
		}
	}

	// check if the host is reachable via SSH
	cloudHost, err := providers.GetCloudHost(host, init.Settings)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get cloud host for %s", host.Id)
	}
	reachable, err := cloudHost.IsSSHReachable()
	if err != nil {
		return false, errors.Wrapf(err, "error checking if host %s is reachable", host.Id)
	}

	// at this point, we can run the setup if the host is reachable
	return reachable, nil
}

// setupHost runs the specified setup script for an individual host. Returns
// the output from running the script remotely, as well as any error that
// occurs. If the script exits with a non-zero exit code, the error will be non-nil.
func (init *HostInit) setupHost(ctx context.Context, targetHost *host.Host) (string, error) {
	// fetch the appropriate cloud provider for the host
	cloudMgr, err := providers.GetCloudManager(targetHost.Provider, init.Settings)
	if err != nil {
		return "", errors.Wrapf(err,
			"failed to get cloud manager for host %s with provider %s",
			targetHost.Id, targetHost.Provider)
	}

	// mark the host as initializing
	if err = targetHost.SetInitializing(); err != nil {
		if err == mgo.ErrNotFound {
			return "", ErrHostAlreadyInitializing
		} else {
			return "", errors.Wrapf(err, "database error")
		}
	}

	/* TESTING ONLY
	setupDebugSSHTunnel(path_to_ssh_key, targetHost.User, targetHost.Host)
	*/

	// run the function scheduled for when the host is up
	err = cloudMgr.OnUp(targetHost)
	if err != nil {
		err = errors.Wrapf(err, "OnUp callback failed for host %s", targetHost.Id)
		grip.Error(err)
		return "", err
	}
	cloudHost, err := providers.GetCloudHost(targetHost, init.Settings)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get cloud host for %s", targetHost.Id)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return "", errors.Wrapf(err, "error getting ssh options for host %s", targetHost.Id)
	}

	// get expansions mapping using settings
	if targetHost.Distro.Setup == "" {
		exp := util.NewExpansions(init.Settings.Expansions)
		targetHost.Distro.Setup, err = exp.ExpandString(targetHost.Distro.Setup)
		if err != nil {
			return "", errors.Wrap(err, "expansions error")
		}
	}

	if targetHost.Distro.Setup != "" {
		err = init.copyScript(ctx, targetHost, setupScriptName, targetHost.Distro.Setup)
		if err != nil {
			return "", errors.Errorf("error copying script %v to host %v: %v",
				setupScriptName, targetHost.Id, err)
		}
		var logs string
		logs, err = hostutil.RunRemoteScript(targetHost, setupScriptName, sshOptions)
		if err != nil {
			return logs, errors.Errorf("error running setup script over ssh: %v", err)
		}
	}

	if targetHost.Distro.Teardown != "" {
		err = init.copyScript(ctx, targetHost, teardownScriptName, targetHost.Distro.Teardown)
		if err != nil {
			return "", errors.Wrapf(err, "error copying script %v to host %v",
				teardownScriptName, targetHost.Id)
		}
	}

	return "", nil
}

// copyScript writes a given script as file "name" to the target host. This works
// by creating a local copy of the script on the runner's machine, scping it over
// then removing the local copy.
func (init *HostInit) copyScript(ctx context.Context, target *host.Host, name, script string) error {
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
		return errors.Wrap(err, "error creating temporary script file")
	}
	defer func() {
		grip.Error(file.Close())
		grip.Error(os.Remove(file.Name()))
	}()

	expanded, err := init.expandScript(script)
	if err != nil {
		return errors.Wrapf(err, "error expanding script for host %s", target.Id)
	}
	if _, err := io.WriteString(file, expanded); err != nil {
		return errors.Wrap(err, "error writing local script")
	}

	cloudHost, err := providers.GetCloudHost(target, init.Settings)
	if err != nil {
		return errors.Wrapf(err, "failed to get cloud host for %s", target.Id)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return errors.Wrapf(err, "error getting ssh options for host %v", target.Id)
	}

	cappedMakeDirectoryLog := util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}

	makeDirectoryCmd := &subprocess.RemoteCommand{
		Id:             fmt.Sprintf("create-workdir-%d", rand.Int()),
		CmdString:      fmt.Sprintf("mkdir -m 777 -p %s", target.Distro.WorkDir),
		Stdout:         &cappedMakeDirectoryLog,
		Stderr:         &cappedMakeDirectoryLog,
		RemoteHostName: hostInfo.Hostname,
		User:           user,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
	}
	if target.Distro.SetupAsSudo {
		makeDirectoryCmd.CmdString = fmt.Sprintf("sudo %s", makeDirectoryCmd.CmdString)
	}

	grip.Infof("Directories command: '%#v'", makeDirectoryCmd)

	// run the make shell command with a timeout
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, SCPTimeout)
	defer cancel()

	if err = makeDirectoryCmd.Run(ctx); err != nil {
		if err == util.ErrTimedOut {
			grip.Notice(makeDirectoryCmd.Stop())
			return errors.Errorf("creating remote directories timed out: %s",
				cappedMakeDirectoryLog.String())
		}
		return errors.Wrapf(err, "error creating directories on remote machine: %s",
			cappedMakeDirectoryLog.String())
	}

	var scpCmdStderr bytes.Buffer
	scpCmd := &subprocess.ScpCommand{
		Source:         file.Name(),
		Dest:           filepath.Join(target.Distro.WorkDir, name),
		Stdout:         &scpCmdStderr,
		Stderr:         &scpCmdStderr,
		RemoteHostName: hostInfo.Hostname,
		User:           user,
		Options:        append([]string{"-P", hostInfo.Port}, sshOptions...),
	}
	// run the command to scp the agent with a timeout
	ctx, cancel = context.WithTimeout(ctx, SCPTimeout)
	defer cancel()
	if err = scpCmd.Run(ctx); err != nil {
		if err == util.ErrTimedOut {
			grip.Warning(scpCmd.Stop())
			return errors.Wrap(err, "scp-ing script timed out")
		}
		return errors.Wrapf(err, "error (%v) copying script to remote machine",
			scpCmdStderr.String())
	}
	return nil
}

// Build the setup script that will need to be run on the specified host.
func (init *HostInit) expandScript(s string) (string, error) {
	// replace expansions in the script
	exp := util.NewExpansions(init.Settings.Expansions)
	script, err := exp.ExpandString(s)
	if err != nil {
		return "", errors.Wrap(err, "expansions error")
	}
	return script, err
}

// Provision the host, and update the database accordingly.
func (init *HostInit) ProvisionHost(ctx context.Context, h *host.Host) error {

	grip.Infoln("Setting up host", h.Id)
	output, err := init.setupHost(ctx, h)

	if err != nil {
		// another hostinit process beat us there
		if err == ErrHostAlreadyInitializing {
			grip.Debugln("Attempted to initialize already initializing host %s", h.Id)
			return nil
		}

		grip.Warning(alerts.RunHostProvisionFailTriggers(h))
		event.LogProvisionFailed(h.Id, output)

		// mark the host's provisioning as failed
		if err := h.SetUnprovisioned(); err != nil {
			grip.Errorf("unprovisioning host %s failed: %+v", h.Id, err)
		}

		return errors.Wrapf(err, "error initializing host %s", h.Id)
	}

	grip.Infof("Setup complete for host %s", h.Id)

	if h.ProvisionOptions != nil &&
		h.ProvisionOptions.LoadCLI &&
		h.ProvisionOptions.OwnerId != "" {
		grip.Infof("Uploading client binary to host %s", h.Id)
		lcr, err := init.LoadClient(ctx, h)
		if err != nil {
			grip.Errorf("Failed to load client binary onto host %s: %+v", h.Id, err)
		} else if err == nil && len(h.ProvisionOptions.TaskId) > 0 {
			grip.Infof("Fetching data for task %s onto host %s", h.ProvisionOptions.TaskId, h.Id)
			err = init.fetchRemoteTaskData(ctx, h.ProvisionOptions.TaskId, lcr.BinaryPath, lcr.ConfigPath, h)
			grip.ErrorWhenf(err != nil, "Failed to fetch data onto host %s: %v", h.Id, err)
		}
	}

	// the setup was successful. update the host accordingly in the database
	if err := h.MarkAsProvisioned(); err != nil {
		return errors.Wrapf(err, "error marking host %s as provisioned", h.Id)
	}

	grip.Infof("Host %s successfully provisioned", h.Id)

	return nil
}

// LocateCLIBinary returns the (absolute) path to the CLI binary for the given architecture, based
// on the system settings. Returns an error if the file does not exist.
func LocateCLIBinary(settings *evergreen.Settings, architecture string) (string, error) {
	clientsSubDir := "clients"
	if settings.ClientBinariesDir != "" {
		clientsSubDir = settings.ClientBinariesDir
	}

	binaryName := "evergreen"
	if strings.HasPrefix(architecture, "windows") {
		binaryName += ".exe"
	}

	path := filepath.Join(clientsSubDir, architecture, binaryName)
	if !filepath.IsAbs(clientsSubDir) {
		path = filepath.Join(evergreen.FindEvergreenHome(), path)
	}

	_, err := os.Stat(path)
	if err != nil {
		return path, errors.WithStack(err)
	}

	path, err = filepath.Abs(path)
	return path, errors.WithStack(err)
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
func (init *HostInit) LoadClient(ctx context.Context, target *host.Host) (*LoadClientResult, error) {
	// Make sure we have the binary we want to upload - if it hasn't been built for the given
	// architecture, fail early
	cliBinaryPath, err := LocateCLIBinary(init.Settings, target.Distro.Arch)
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't locate CLI binary for upload")
	}
	if target.ProvisionOptions == nil {
		return nil, errors.New("ProvisionOptions is nil")
	}
	if target.ProvisionOptions.OwnerId == "" {
		return nil, errors.New("OwnerId not set")
	}

	// get the information about the owner of the host
	owner, err := user.FindOne(user.ById(target.ProvisionOptions.OwnerId))
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't fetch owner %v for host", target.ProvisionOptions.OwnerId)
	}

	// 1. mkdir the destination directory on the host,
	//    and modify ~/.profile so the target binary will be on the $PATH
	targetDir := "cli_bin"
	hostSSHInfo, err := util.ParseSSHInfo(target.Host)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing ssh info %s", target.Host)
	}

	cloudHost, err := providers.GetCloudHost(target, init.Settings)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to get cloud host for %s", target.Id)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return nil, errors.Wrapf(err, "Error getting ssh options for host %v", target.Id)
	}
	sshOptions = append(sshOptions, "-o", "UserKnownHostsFile=/dev/null")

	mkdirOutput := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}

	// Create the directory for the binary to be uploaded into.
	// Also, make a best effort to add the binary's location to $PATH upon login. If we can't do
	// this successfully, the command will still succeed, it just means that the user will have to
	// use an absolute path (or manually set $PATH in their shell) to execute it.
	makeShellCmd := &subprocess.RemoteCommand{
		CmdString:      fmt.Sprintf("mkdir -m 777 -p ~/%s && (echo 'PATH=$PATH:~/%s' >> ~/.profile || true; echo 'PATH=$PATH:~/%s' >> ~/.bash_profile || true)", targetDir, targetDir, targetDir),
		Stdout:         mkdirOutput,
		Stderr:         mkdirOutput,
		RemoteHostName: hostSSHInfo.Hostname,
		User:           target.User,
		Options:        append([]string{"-p", hostSSHInfo.Port}, sshOptions...),
	}

	scpOut := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}

	// run the make shell command with a timeout
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err = makeShellCmd.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "error running setup command for cli, %v",
			mkdirOutput.Buffer.String())
	}

	// place the binary into the directory
	scpSetupCmd := &subprocess.ScpCommand{
		Source:         cliBinaryPath,
		Dest:           fmt.Sprintf("~/%s/evergreen", targetDir),
		Stdout:         scpOut,
		Stderr:         scpOut,
		RemoteHostName: hostSSHInfo.Hostname,
		User:           target.User,
		Options:        append([]string{"-P", hostSSHInfo.Port}, sshOptions...),
	}

	// run the command to scp the setup script with a timeout
	ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err = scpSetupCmd.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "error running SCP command for cli, %v: '%v'", scpOut.Buffer.String())
	}

	// 4. Write a settings file for the user that owns the host, and scp it to the directory
	outputStruct := model.CLISettings{
		User:          owner.Id,
		APIKey:        owner.APIKey,
		APIServerHost: init.Settings.ApiUrl + "/api",
		UIServerHost:  init.Settings.Ui.Url,
	}
	outputJSON, err := json.Marshal(outputStruct)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	tempFileName, err := util.WriteTempFile("", outputJSON)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer os.Remove(tempFileName)

	scpYmlCommand := &subprocess.ScpCommand{
		Source:         tempFileName,
		Dest:           fmt.Sprintf("~/%s/.evergreen.yml", targetDir),
		Stdout:         scpOut,
		Stderr:         scpOut,
		RemoteHostName: hostSSHInfo.Hostname,
		User:           target.User,
		Options:        append([]string{"-P", hostSSHInfo.Port}, sshOptions...),
	}
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err = scpYmlCommand.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "error running SCP command for evergreen.yml, %v", scpOut.Buffer.String())
	}

	return &LoadClientResult{
		BinaryPath: fmt.Sprintf("~/%s/evergreen", targetDir),
		ConfigPath: fmt.Sprintf("~/%s/.evergreen.yml", targetDir),
	}, nil
}

func (init *HostInit) fetchRemoteTaskData(ctx context.Context, taskId, cliPath, confPath string, target *host.Host) error {
	hostSSHInfo, err := util.ParseSSHInfo(target.Host)
	if err != nil {
		return errors.Wrapf(err, "error parsing ssh info %s", target.Host)
	}

	cloudHost, err := providers.GetCloudHost(target, init.Settings)
	if err != nil {
		return errors.Wrapf(err, "Failed to get cloud host for %v", target.Id)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return errors.Wrapf(err, "Error getting ssh options for host %v", target.Id)
	}
	sshOptions = append(sshOptions, "-o", "UserKnownHostsFile=/dev/null")

	/* TESTING ONLY
	setupDebugSSHTunnel(path_to_ssh_keys, target.User, hostSSHInfo.Hostname)
	*/

	// When testing, use this writer to force a copy of the output to be written to standard out so
	// that remote command failures also show up in server log output.
	//cmdOutput := io.MultiWriter(&util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}, os.Stdout)

	cmdOutput := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}
	makeShellCmd := &subprocess.RemoteCommand{
		CmdString:      fmt.Sprintf("%s -c '%s' fetch -t %s --source --artifacts --dir='%s'", cliPath, confPath, taskId, target.Distro.WorkDir),
		Stdout:         cmdOutput,
		Stderr:         cmdOutput,
		RemoteHostName: hostSSHInfo.Hostname,
		User:           target.User,
		Options:        append([]string{"-p", hostSSHInfo.Port}, sshOptions...),
	}

	// run the make shell command with a timeout
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	return errors.WithStack(makeShellCmd.Run(ctx))
}
