package units

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const setupHostJobName = "provisioning-setup-host"

func init() {
	registry.AddJobType(setupHostJobName, func() amboy.Job {
		return makeSetupHostJob()
	})
}

type setupHostJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	host *host.Host
	env  evergreen.Environment
}

func makeSetupHostJob() *setupHostJob {
	j := &setupHostJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    setupHostJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostSetupJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeSetupHostJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", setupHostJobName, j.HostID, id))
	return j
}

func (j *setupHostJob) Run(ctx context.Context) {
	var err error
	defer j.MarkComplete()

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.TaskID))
			return
		}
	}
	if j.host.Status == evergreen.HostRunning {
		grip.Info(message.Fields{
			"runner":  HostInit,
			"host":    j.host.Id,
			"message": "skipping setup because host is already set up",
		})
		return
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	defer j.tryRequeue()

	settings := j.env.Settings()

	j.AddError(j.setupHost(ctx, j.host, settings))
}

var (
	errRetryHost           = errors.New("host status is starting after running provisioning")
	errIgnorableCreateHost = errors.New("create host encountered internal error")
)

func (j *setupHostJob) setupHost(ctx context.Context, h *host.Host, settings *evergreen.Settings) error {
	grip.Info(message.Fields{
		"message": "attempting to setup host",
		"distro":  h.Distro.Id,
		"hostid":  h.Id,
		"DNS":     h.Host,
		"runner":  HostInit,
	})

	if err := j.setDNSName(ctx, h, settings); err != nil {
		return errors.Wrap(err, "error settings DNS name")
	}

	if err := ctx.Err(); err != nil {
		return errors.Wrapf(err, "hostinit canceled during setup for host %s", h.Id)
	}

	setupStartTime := time.Now()
	grip.Info(message.Fields{
		"message": "provisioning host",
		"runner":  HostInit,
		"distro":  h.Distro.Id,
		"hostid":  h.Id,
		"DNS":     h.Host,
	})

	if err := j.provisionHost(ctx, h, settings); err != nil {
		event.LogHostProvisionError(h.Id)

		grip.Error(message.WrapError(err, message.Fields{
			"message": "provisioning host encountered error",
			"runner":  HostInit,
			"distro":  h.Distro.Id,
			"hostid":  h.Id,
		}))

		// notify the admins of the failure
		subject := fmt.Sprintf("%v Evergreen provisioning failure on %v",
			notify.ProvisionFailurePreface, h.Distro.Id)
		hostLink := fmt.Sprintf("%v/host/%v", settings.Ui.Url, h.Id)
		message := fmt.Sprintf("Provisioning failed on %v host -- %v: see %v",
			h.Distro.Id, h.Id, hostLink)

		if err := notify.NotifyAdmins(subject, message, settings); err != nil {
			return errors.Wrap(err, "problem sending host init error email")
		}
	}

	// ProvisionHost allows hosts to fail provisioning a few
	// times during host start up, to account for the fact
	// that hosts often need extra time to come up.
	//
	// In these cases, ProvisionHost returns a nil error but
	// does not change the host status.
	if h.Status == evergreen.HostProvisioning {
		return errors.Wrapf(errRetryHost, "retrying for '%s', after %d attempts",
			h.Id, h.ProvisionAttempts)
	}

	grip.Info(message.Fields{
		"message":  "successfully finished provisioning host",
		"hostid":   h.Id,
		"DNS":      h.Host,
		"distro":   h.Distro.Id,
		"runner":   HostInit,
		"attempts": h.ProvisionAttempts,
		"runtime":  time.Since(setupStartTime),
	})

	return nil
}

const (
	HostInit   = "hostinit"
	scpTimeout = time.Minute
)

func (j *setupHostJob) setDNSName(ctx context.Context, host *host.Host, settings *evergreen.Settings) error {
	// fetch the appropriate cloud provider for the host
	cloudMgr, err := cloud.GetManager(ctx, host.Distro.Provider, settings)
	if err != nil {
		return errors.Wrapf(err, "failed to get cloud manager for provider %s", host.Distro.Provider)
	}

	if host.Host != "" {
		return nil
	}

	// get the DNS name for the host
	hostDNS, err := cloudMgr.GetDNSName(ctx, host)
	if err != nil {
		return errors.Wrapf(err, "error checking DNS name for host %s", host.Id)
	}

	// sanity check for the host DNS name
	if hostDNS == "" {
		return errors.Errorf("instance %s is running but not returning a DNS name", host.Id)
	}

	// update the host's DNS name
	if err = host.SetDNSName(hostDNS); err != nil {
		return errors.Wrapf(err, "error setting DNS name for host %s", host.Id)
	}

	return nil
}

// runHostSetup runs the specified setup script for an individual host. Returns
// the output from running the script remotely, as well as any error that
// occurs. If the script exits with a non-zero exit code, the error will be non-nil.
func (j *setupHostJob) runHostSetup(ctx context.Context, targetHost *host.Host, settings *evergreen.Settings) (string, error) {
	// fetch the appropriate cloud provider for the host
	cloudMgr, err := cloud.GetManager(ctx, targetHost.Provider, settings)
	if err != nil {
		return "", errors.Wrapf(err,
			"failed to get cloud manager for host %s with provider %s",
			targetHost.Id, targetHost.Provider)
	}

	// run the function scheduled for when the host is up
	if err = cloudMgr.OnUp(ctx, targetHost); err != nil {
		err = errors.Wrapf(err, "OnUp callback failed for host %s", targetHost.Id)
		return "", err
	}

	// get expansions mapping using settings
	if targetHost.Distro.Setup == "" {
		exp := util.NewExpansions(settings.Expansions)
		targetHost.Distro.Setup, err = exp.ExpandString(targetHost.Distro.Setup)
		if err != nil {
			return "", errors.Wrap(err, "expansions error")
		}
	}

	if targetHost.Distro.Setup != "" {
		err = j.copyScript(ctx, settings, targetHost, evergreen.SetupScriptName, targetHost.Distro.Setup)
		if err != nil {
			return "", errors.Wrapf(err, "error copying setup script %v to host %v",
				evergreen.SetupScriptName, targetHost.Id)
		}
	}

	if targetHost.Distro.Teardown != "" {
		err = j.copyScript(ctx, settings, targetHost, evergreen.TeardownScriptName, targetHost.Distro.Teardown)
		if err != nil {
			return "", errors.Wrapf(err, "error copying teardown script %v to host %v",
				evergreen.TeardownScriptName, targetHost.Id)
		}
	}

	return "", nil
}

// copyScript writes a given script as file "name" to the target host. This works
// by creating a local copy of the script on the runner's machine, scping it over
// then removing the local copy.
func (j *setupHostJob) copyScript(ctx context.Context, settings *evergreen.Settings, target *host.Host, name, script string) error {
	// parse the hostname into the user, host and port
	startAt := time.Now()
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
	if err = os.Chmod(file.Name(), 0700); err != nil {
		return errors.Wrap(err, "error setting file permissions")
	}
	defer func() {
		errCtx := message.Fields{
			"runner":    HostInit,
			"operation": "cleaning up after script copy",
			"file":      file.Name(),
			"distro":    target.Distro.Id,
			"host":      target.Host,
			"name":      name,
		}
		grip.Error(message.WrapError(file.Close(), errCtx))
		grip.Error(message.WrapError(os.Remove(file.Name()), errCtx))
		grip.Debug(message.Fields{
			"runner":        HostInit,
			"operation":     "copy script",
			"file":          file.Name(),
			"distro":        target.Distro.Id,
			"host":          target.Host,
			"name":          name,
			"duration_secs": time.Since(startAt).Seconds(),
		})
	}()

	expanded, err := j.expandScript(script, settings)
	if err != nil {
		return errors.Wrapf(err, "error expanding script for host %s", target.Id)
	}
	if _, err = io.WriteString(file, expanded); err != nil {
		return errors.Wrap(err, "error writing local script")
	}

	cloudHost, err := cloud.GetCloudHost(ctx, target, settings)
	if err != nil {
		return errors.Wrapf(err, "failed to get cloud host for %s", target.Id)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return errors.Wrapf(err, "error getting ssh options for host %v", target.Id)
	}

	scpCmdOut := &bytes.Buffer{}

	output := subprocess.OutputOptions{Output: scpCmdOut, SendErrorToOutput: true}
	scpCmd := subprocess.NewSCPCommand(
		file.Name(),
		filepath.Join("~", name),
		hostInfo.Hostname,
		user,
		append([]string{"-vvv", "-P", hostInfo.Port}, sshOptions...))

	if err = scpCmd.SetOutput(output); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"runner":    HostInit,
			"operation": "setting up copy script command",
			"distro":    target.Distro.Id,
			"host":      target.Host,
			"output":    output,
			"cause":     "programmer error",
		}))
		return errors.Wrap(err, "problem configuring output")
	}

	// run the command to scp the script with a timeout
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, scpTimeout)
	defer cancel()
	if err = scpCmd.Run(ctx); err != nil {
		grip.Notice(message.WrapError(err, message.Fields{
			"message": "problem copying script to host",
			"runner":  HostInit,
			"command": scpCmd,
			"distro":  target.Distro.Id,
			"host":    target.Host,
			"output":  scpCmdOut.String(),
		}))

		return errors.Wrapf(err, "error (%v) copying script to remote machine",
			scpCmdOut.String())
	}
	return nil
}

// Build the setup script that will need to be run on the specified host.
func (j *setupHostJob) expandScript(s string, settings *evergreen.Settings) (string, error) {
	// replace expansions in the script
	exp := util.NewExpansions(settings.Expansions)
	script, err := exp.ExpandString(s)
	if err != nil {
		return "", errors.Wrap(err, "expansions error")
	}
	return script, err
}

// Provision the host, and update the database accordingly.
func (j *setupHostJob) provisionHost(ctx context.Context, h *host.Host, settings *evergreen.Settings) error {
	grip.Infoln(message.Fields{
		"runner":  HostInit,
		"host":    h.Id,
		"message": "setting up host",
	})

	output, err := j.runHostSetup(ctx, h, settings)
	if err != nil {
		incErr := h.IncProvisionAttempts()
		grip.Critical(message.WrapError(incErr, message.Fields{
			"runner":    HostInit,
			"host":      h.Id,
			"operation": "increment provisioning errors failed",
		}))

		if shouldRetryProvisioning(h) {
			grip.Debug(message.Fields{
				"runner":   HostInit,
				"host":     h.Id,
				"attempts": h.ProvisionAttempts,
				"output":   output,
				"error":    err.Error(),
				"message":  "provisioning failed, but will retry",
			})
			return nil
		}

		grip.Warning(message.WrapError(alerts.RunHostProvisionFailTriggers(h), message.Fields{
			"operation": "running host provisioning alert trigger",
			"runner":    HostInit,
			"host":      h.Id,
			"attempts":  h.ProvisionAttempts,
		}))
		event.LogProvisionFailed(h.Id, output)

		// mark the host's provisioning as failed
		grip.Error(message.WrapError(h.SetUnprovisioned(), message.Fields{
			"operation": "setting host unprovisioned",
			"attempts":  h.ProvisionAttempts,
			"runner":    HostInit,
			"host":      h.Id,
		}))

		return errors.Wrapf(err, "error initializing host %s", h.Id)
	}

	// If this is a spawn host
	if h.ProvisionOptions != nil && h.ProvisionOptions.LoadCLI {
		grip.Infof("Uploading client binary to host %s", h.Id)
		lcr, err := j.loadClient(ctx, h, settings)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "failed to load client binary onto host",
				"runner":  HostInit,
				"host":    h.Id,
			}))

			grip.Error(message.WrapError(h.SetUnprovisioned(), message.Fields{
				"operation": "setting host unprovisioned",
				"runner":    HostInit,
				"host":      h.Id,
			}))
			return errors.Wrapf(err, "Failed to load client binary onto host %s: %+v", h.Id, err)
		}

		cloudHost, err := cloud.GetCloudHost(ctx, h, settings)
		if err != nil {
			grip.Error(message.WrapError(h.SetUnprovisioned(), message.Fields{
				"operation": "setting host unprovisioned",
				"runner":    HostInit,
				"host":      h.Id,
			}))

			return errors.Wrapf(err, "Failed to get cloud host for %s", h.Id)
		}
		sshOptions, err := cloudHost.GetSSHOptions()
		if err != nil {
			grip.Error(message.WrapError(h.SetUnprovisioned(), message.Fields{
				"operation": "setting host unprovisioned",
				"runner":    HostInit,
				"host":      h.Id,
			}))
			return errors.Wrapf(err, "Error getting ssh options for host %s", h.Id)
		}

		d, err := distro.FindOne(distro.ById(h.Distro.Id))
		if err != nil {
			grip.Error(message.WrapError(h.SetUnprovisioned(), message.Fields{
				"operation": "setting host unprovisioned",
				"runner":    HostInit,
				"host":      h.Id,
			}))

		}
		h.Distro = d

		grip.Infof("Running setup script for spawn host %s", h.Id)
		// run the setup script with the agent
		if logs, err := h.RunSSHCommand(ctx, h.SetupCommand(), sshOptions); err != nil {
			grip.Error(message.WrapError(h.SetUnprovisioned(), message.Fields{
				"operation": "setting host unprovisioned",
				"runner":    HostInit,
				"host":      h.Id,
			}))
			event.LogProvisionFailed(h.Id, logs)
			return errors.Wrapf(err, "error running setup script on remote host: %s", logs)
		}

		if h.ProvisionOptions.OwnerId != "" && len(h.ProvisionOptions.TaskId) > 0 {
			grip.Info(message.Fields{
				"message": "fetching data for task on host",
				"task":    h.ProvisionOptions.TaskId,
				"host":    h.Id,
				"runner":  HostInit,
			})

			grip.Error(message.WrapError(j.fetchRemoteTaskData(ctx, h.ProvisionOptions.TaskId, lcr.BinaryPath, lcr.ConfigPath, h, settings),
				message.Fields{
					"message": "failed to fetch data onto host",
					"task":    h.ProvisionOptions.TaskId,
					"host":    h.Id,
					"runner":  HostInit,
				}))
		}
	}

	grip.Info(message.Fields{
		"message": "setup complete for host",
		"host":    h.Id,
		"runner":  HostInit,
	})

	// the setup was successful. update the host accordingly in the database
	if err := h.MarkAsProvisioned(); err != nil {
		return errors.Wrapf(err, "error marking host %s as provisioned", h.Id)
	}

	grip.Info(message.Fields{
		"host":    h.Id,
		"runner":  HostInit,
		"message": "host successfully provisioned",
	})

	return nil
}

// loadClientResult indicates the locations on a target host where the CLI binary and it's config
// file have been written to.
type loadClientResult struct {
	BinaryPath string
	ConfigPath string
}

// loadClient places the evergreen command line client on the host, places a copy of the user's
// settings onto the host, and makes the binary appear in the $PATH when the user logs in.
// If successful, returns an instance of loadClientResult which contains the paths where the
// binary and config file were written to.
func (j *setupHostJob) loadClient(ctx context.Context, target *host.Host, settings *evergreen.Settings) (*loadClientResult, error) {
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

	cloudHost, err := cloud.GetCloudHost(ctx, target, settings)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to get cloud host for %s", target.Id)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return nil, errors.Wrapf(err, "Error getting ssh options for host %v", target.Id)
	}
	sshOptions = append(sshOptions, "-o", "UserKnownHostsFile=/dev/null")

	mkdirOutput := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}
	opts := subprocess.OutputOptions{Output: mkdirOutput, SendErrorToOutput: true}
	makeShellCmd := subprocess.NewRemoteCommand(
		fmt.Sprintf("mkdir -m 777 -p ~/%s && (echo 'PATH=$PATH:~/%s' >> ~/.profile || true; echo 'PATH=$PATH:~/%s' >> ~/.bash_profile || true)", targetDir, targetDir, targetDir),
		hostSSHInfo.Hostname,
		target.User,
		nil,   // env
		false, // background
		append([]string{"-p", hostSSHInfo.Port}, sshOptions...),
		false, // disable logging
	)

	if err = makeShellCmd.SetOutput(opts); err != nil {
		return nil, errors.Wrap(err, "problem setting up output")
	}

	// Create the directory for the binary to be uploaded into.
	// Also, make a best effort to add the binary's location to $PATH upon login. If we can't do
	// this successfully, the command will still succeed, it just means that the user will have to
	// use an absolute path (or manually set $PATH in their shell) to execute it.

	// run the make shell command with a timeout
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err = makeShellCmd.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "error running setup command for cli, %v",
			mkdirOutput.Buffer.String())
	}

	curlOut := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}
	opts.Output = curlOut

	// place the binary into the directory
	curlSetupCmd := subprocess.NewRemoteCommand(
		target.CurlCommand(settings.Ui.Url),
		hostSSHInfo.Hostname,
		target.User,
		nil,   // env
		false, // background
		append([]string{"-p", hostSSHInfo.Port}, sshOptions...),
		false, // disable logging
	)

	if err = curlSetupCmd.SetOutput(opts); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"runner":    HostInit,
			"operation": "command to fetch the evergreen binary on the host",
			"distro":    target.Distro.Id,
			"host":      target.Host,
			"output":    opts,
			"cause":     "programmer error",
		}))

		return nil, errors.Wrap(err, "problem setting up output")
	}

	// run the command to curl the agent
	ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err = curlSetupCmd.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "error running curl command for cli, %v: '%v'", curlOut.Buffer.String())
	}

	// 4. Write a settings file for the user that owns the host, and scp it to the directory
	outputStruct := struct {
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

	output := subprocess.OutputOptions{Output: scpOut, SendErrorToOutput: true}

	scpYmlCommand := subprocess.NewSCPCommand(
		tempFileName,
		fmt.Sprintf("~/%s/.evergreen.yml", targetDir),
		hostSSHInfo.Hostname,
		target.User,
		append([]string{"-P", hostSSHInfo.Port}, sshOptions...))

	if err = scpYmlCommand.SetOutput(output); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"runner":    HostInit,
			"operation": "setting up copy cli config command",
			"distro":    target.Distro.Id,
			"host":      target.Host,
			"output":    output,
			"cause":     "programmer error",
		}))

		return nil, errors.Wrap(err, "problem configuring output")
	}

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err = scpYmlCommand.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "error running SCP command for evergreen.yml, %v", scpOut.Buffer.String())
	}

	return &loadClientResult{
		BinaryPath: filepath.Join("~", "evergreen"),
		ConfigPath: fmt.Sprintf("%s/.evergreen.yml", targetDir),
	}, nil
}

func (j *setupHostJob) fetchRemoteTaskData(ctx context.Context, taskId, cliPath, confPath string, target *host.Host, settings *evergreen.Settings) error {
	hostSSHInfo, err := util.ParseSSHInfo(target.Host)
	if err != nil {
		return errors.Wrapf(err, "error parsing ssh info %s", target.Host)
	}

	cloudHost, err := cloud.GetCloudHost(ctx, target, settings)
	if err != nil {
		return errors.Wrapf(err, "Failed to get cloud host for %v", target.Id)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return errors.Wrapf(err, "Error getting ssh options for host %v", target.Id)
	}
	sshOptions = append(sshOptions, "-o", "UserKnownHostsFile=/dev/null")

	cmdOutput := &util.CappedWriter{&bytes.Buffer{}, 1024 * 1024}
	fetchCmd := fmt.Sprintf("%s -c %s fetch -t %s --source --artifacts --dir='%s'", cliPath, confPath, taskId, target.Distro.WorkDir)
	makeShellCmd := subprocess.NewRemoteCommand(
		fetchCmd,
		hostSSHInfo.Hostname,
		target.User,
		nil,   // env
		false, // background
		append([]string{"-p", hostSSHInfo.Port}, sshOptions...),
		false, // disable logging
	)

	output := subprocess.OutputOptions{Output: cmdOutput, SendErrorToOutput: true}
	if err := makeShellCmd.SetOutput(output); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"operation": "fetch command",
			"message":   "configuring output for fetch command",
			"hostname":  hostSSHInfo.Hostname,
			"distro":    target.Distro.Id,
			"host_id":   target.Id,
			"cause":     "programmer error",
			"output":    output,
		}))

		return errors.Wrap(err, "problem configuring output for fetch command")
	}

	// run the make shell command with a timeout
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	if err := makeShellCmd.Run(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": fmt.Sprintf("fetch-artifacts-%s", taskId),
			"host":    hostSSHInfo.Hostname,
			"cmd":     fetchCmd,
			"runner":  HostInit,
			"output":  cmdOutput.Buffer.String(),
		}))
		return err
	}
	return nil
}

func (j *setupHostJob) tryRequeue() {
	if shouldRetryProvisioning(j.host) && j.env.RemoteQueue().Started() {
		err := j.env.RemoteQueue().Put(NewHostSetupJob(j.env, *j.host, fmt.Sprintf("attempt-%d", j.host.ProvisionAttempts)))
		grip.Critical(message.WrapError(err, message.Fields{
			"message":  "failed to requeue setup job",
			"host":     j.host.Id,
			"runner":   HostInit,
			"attempts": j.host.ProvisionAttempts,
		}))
		j.AddError(err)
	}
}

func shouldRetryProvisioning(h *host.Host) bool {
	return h.ProvisionAttempts <= 15 && h.Status == evergreen.HostProvisioning
}
