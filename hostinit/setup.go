package hostinit

import (
	"bytes"
	"context"
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
	"gopkg.in/mgo.v2"
)

const (
	SCPTimeout = time.Minute
)

// Error indicating another hostinit got to the setup first.
var (
	ErrHostAlreadyInitializing = errors.New("Host already initializing")
)

// Longest duration allowed for running setup script.
var (
	SSHTimeoutSeconds = int64(120) // 2 minutes
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

	startQueue := make([]host.Host, len(hostsToStart))
	for i, r := range rand.Perm(len(hostsToStart)) {
		startQueue[i] = hostsToStart[r]
	}

	var started int
	for _, h := range startQueue {
		if ctx.Err() != nil {
			return errors.New("hostinit run canceled")
		}

		if h.UserHost {
			// pass:
			//    always start spawn hosts asap
		} else if started > 4 {
			// throttle hosts, so that we're starting very
			// few hosts on every pass. Hostinit runs very
			// frequently, lets not start too many all at
			// once.

			continue
		}
		started++

		hostStartTime := time.Now()
		grip.Info(message.Fields{
			"GUID":    init.GUID,
			"message": "attempting to start host",
			"hostid":  h.Id,
		})

		cloudManager, err := providers.GetCloudManager(h.Provider, init.Settings)
		if err != nil {
			return errors.Wrapf(err, "error getting cloud provider for host %s", h.Id)
		}

		err = h.Remove()
		if err != nil {
			return errors.Wrapf(err, "error removing intent host %s", h.Id)
		}

		_, err = cloudManager.SpawnHost(&h)
		if err != nil {
			return errors.Wrapf(err, "error spawning host %s", h.Id)
		}

		h.Status = evergreen.HostStarting
		h.StartTime = time.Now()

		_, err = h.Upsert()
		if err != nil {
			return errors.Wrapf(err, "error updating host %v", h.Id)
		}

		grip.Info(message.Fields{
			"GUID":    init.GUID,
			"message": "successfully started host",
			"hostid":  h.Id,
			"DNS":     h.Host,
			"runtime": time.Since(hostStartTime),
		})
	}

	grip.Info(message.Fields{
		"GUID":      init.GUID,
		"runner":    RunnerName,
		"method":    "startHosts",
		"num_hosts": started,
		"runtime":   time.Since(startTime),
	})

	return nil
}

// setupReadyHosts runs the distro setup script of all hosts that are up and reachable.
func (init *HostInit) setupReadyHosts(ctx context.Context) error {
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

	grip.Info(message.Fields{
		"message": "uninitialized hosts",
		"number":  len(uninitializedHosts),
		"GUID":    init.GUID,
	})

	// used for making sure we don't exit before a setup script is done
	wg := &sync.WaitGroup{}
	catcher := grip.NewSimpleCatcher()
	hosts := make(chan host.Host, len(uninitializedHosts))
	for _, idx := range rand.Perm(len(uninitializedHosts)) {
		hosts <- uninitializedHosts[idx]
	}
	close(hosts)

	numThreads := 16
	if len(uninitializedHosts) < 16 {
		numThreads = len(uninitializedHosts)
	}

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					catcher.Add(errors.New("hostinit run canceled"))
					return
				case h, ok := <-hosts:
					if !ok {
						return
					}

					grip.Info(message.Fields{
						"GUID":    init.GUID,
						"message": "attempting to setup host",
						"hostid":  h.Id,
						"DNS":     h.Host,
					})

					// check whether or not the host is ready for its setup script to be run
					ready, err := init.IsHostReady(&h)
					if err != nil {
						err = errors.Wrapf(err, "problem checking host %s for readiness", h.Id)
						catcher.Add(err)
						grip.Error(err.Error())
						continue
					}

					// if the host isn't ready (for instance, it might not be up yet), skip it
					if !ready {
						grip.Debug(message.Fields{
							"GUID":    init.GUID,
							"message": "host not ready for setup",
							"hostid":  h.Id,
							"DNS":     h.Host,
						})
						continue
					}

					if ctx.Err() != nil {
						catcher.Add(errors.New("hostinit run canceled"))
						return
					}

					setupStartTime := time.Now()
					grip.Info(message.Fields{
						"GUID":    init.GUID,
						"message": "running setup script for host",
						"hostid":  h.Id,
						"DNS":     h.Host,
					})

					if err := init.ProvisionHost(ctx, &h); err != nil {
						grip.Errorf("Error provisioning host %s: %+v", h.Id, err)

						// notify the admins of the failure
						subject := fmt.Sprintf("%v Evergreen provisioning failure on %v",
							notify.ProvisionFailurePreface, h.Distro.Id)
						hostLink := fmt.Sprintf("%v/host/%v", init.Settings.Ui.Url, h.Id)
						message := fmt.Sprintf("Provisioning failed on %v host -- %v: see %v",
							h.Distro.Id, h.Id, hostLink)
						if err := notify.NotifyAdmins(subject, message, init.Settings); err != nil {
							err = errors.Wrap(err, "problem sending host init error email")
							catcher.Add(err)
						}
						continue
					}
					grip.Info(message.Fields{
						"GUID":    init.GUID,
						"message": "setup script successfully ran for host",
						"hostid":  h.Id,
						"DNS":     h.Host,
						"runtime": time.Since(setupStartTime),
					})
				}
			}
		}()
	}

	// let all setup routines finish
	wg.Wait()

	return catcher.Resolve()
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

	// run the function scheduled for when the host is up
	err = cloudMgr.OnUp(targetHost)
	if err != nil {
		err = errors.Wrapf(err, "OnUp callback failed for host %s", targetHost.Id)
		grip.Error(err)
		return "", err
	}

	// get expansions mapping using settings
	if targetHost.Distro.Setup == "" {
		exp := util.NewExpansions(init.Settings.Expansions)
		targetHost.Distro.Setup, err = exp.ExpandString(targetHost.Distro.Setup)
		if err != nil {
			return "", errors.Wrap(err, "expansions error")
		}
	}

	if err = init.createWorkingDirectory(ctx, targetHost); err != nil {
		return "", errors.Wrapf(err, "error creating working directory on host %s", targetHost.Id)
	}

	if targetHost.Distro.Setup != "" {
		err = init.copyScript(ctx, targetHost, evergreen.SetupScriptName, targetHost.Distro.Setup)
		if err != nil {
			return "", errors.Errorf("error copying script %v to host %v: %v",
				evergreen.SetupScriptName, targetHost.Id, err)
		}
	}

	if targetHost.Distro.Teardown != "" {
		err = init.copyScript(ctx, targetHost, evergreen.TeardownScriptName, targetHost.Distro.Teardown)
		if err != nil {
			return "", errors.Wrapf(err, "error copying script %v to host %v",
				evergreen.TeardownScriptName, targetHost.Id)
		}
	}

	return "", nil
}

// createWorkingDirectory creates the distro's working directory
func (init *HostInit) createWorkingDirectory(ctx context.Context, target *host.Host) error {
	// parse the hostname into the user, host and port
	hostInfo, err := util.ParseSSHInfo(target.Host)
	if err != nil {
		return err
	}
	user := target.Distro.User
	if hostInfo.User != "" {
		user = hostInfo.User
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

	return nil
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

	var scpCmdStderr bytes.Buffer
	scpCmd := &subprocess.ScpCommand{
		Source:         file.Name(),
		Dest:           filepath.Join("~", name),
		Stdout:         &scpCmdStderr,
		Stderr:         &scpCmdStderr,
		RemoteHostName: hostInfo.Hostname,
		User:           user,
		Options:        append([]string{"-P", hostInfo.Port}, sshOptions...),
	}
	// run the command to scp the script with a timeout
	var cancel context.CancelFunc
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

	// If this is a spawn host
	if h.ProvisionOptions != nil && h.ProvisionOptions.LoadCLI {
		grip.Infof("Uploading client binary to host %s", h.Id)
		lcr, err := init.LoadClient(ctx, h)
		if err != nil {
			err = errors.Wrapf(err, "Failed to load client binary onto host %s: %+v", h.Id, err)
			grip.Errorf("Failed to load client binary onto host %s: %+v", h.Id, err)
			return err
		}

		if h.ProvisionOptions.OwnerId != "" &&
			len(h.ProvisionOptions.TaskId) > 0 {
			grip.Infof("Fetching data for task %s onto host %s", h.ProvisionOptions.TaskId, h.Id)
			err = init.fetchRemoteTaskData(ctx, h.ProvisionOptions.TaskId, lcr.BinaryPath, lcr.ConfigPath, h)
			grip.ErrorWhenf(err != nil, "Failed to fetch data onto host %s: %v", h.Id, err)
		}

		grip.Infof("Running setup script for spawn host %s", h.Id)
		args := []string{lcr.BinaryPath, "host", "setup"}
		if h.Distro.SetupAsSudo {
			args = append(args, "--setup_as_sudo")
		}
		cloudHost, err := providers.GetCloudHost(h, init.Settings)
		if err != nil {
			return errors.Wrapf(err, "error getting cloud host for %v", h.Id)
		}
		sshOptions, err := cloudHost.GetSSHOptions()
		if err != nil {
			return errors.Wrapf(err, "error getting ssh options for host %s", h.Id)
		}
		out, err := hostutil.RunRemoteScript(ctx, h, strings.Join(args, " "), sshOptions)
		if err != nil {
			return errors.Wrapf(err, "error running setup script with agent for host %s (%s)", h.Id, out)
		}
		out, err = hostutil.RunRemoteScript(ctx, h, fmt.Sprintf("mkdir -m 777 -p %s", h.Distro.WorkDir), sshOptions)
		if err != nil {
			return errors.Wrapf(err, "error creating working directory after running setup script for host %s (%s)", h.Id, out)
		}
	}

	grip.Infof("Setup complete for host %s", h.Id)

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

	curlOut := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}

	// run the make shell command with a timeout
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err = makeShellCmd.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "error running setup command for cli, %v",
			mkdirOutput.Buffer.String())
	}

	// place the binary into the directory
	curlSetupCmd := &subprocess.RemoteCommand{
		CmdString:      hostutil.CurlCommand(init.Settings.Ui.Url, target),
		Stdout:         curlOut,
		Stderr:         curlOut,
		RemoteHostName: hostSSHInfo.Hostname,
		User:           target.User,
		Options:        append([]string{"-p", hostSSHInfo.Port}, sshOptions...),
	}

	// run the command to curl the agent
	ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err = curlSetupCmd.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "error running curl command for cli, %v: '%v'", curlOut.Buffer.String())
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

	scpOut := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}
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
		BinaryPath: filepath.Join("~", "evergreen"),
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
