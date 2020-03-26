package units

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

const (
	provisionRetryLimit  = 15
	mountRetryLimit      = 10
	mountSleepDuration   = time.Second * 10
	umountMountErrorCode = 32
	setupHostJobName     = "provisioning-setup-host"
	scpTimeout           = time.Minute
)

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

// NewHostSetupJob creates a job that performs any additional provisioning for
// a host to prepare it to run.
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
	if j.host.Status == evergreen.HostRunning || j.host.Provisioned {
		grip.Info(message.Fields{
			"job":     j.ID(),
			"host_id": j.host.Id,
			"message": "skipping setup because host is already set up",
		})
		return
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	defer j.tryRequeue(ctx)

	settings := j.env.Settings()

	j.AddError(j.setupHost(ctx, settings))
}

var (
	errIgnorableCreateHost = errors.New("host.create encountered internal error")
)

func (j *setupHostJob) setupHost(ctx context.Context, settings *evergreen.Settings) error {
	grip.Info(message.Fields{
		"message": "attempting to setup host",
		"distro":  j.host.Distro.Id,
		"host_id": j.host.Id,
		"DNS":     j.host.Host,
		"job":     j.ID(),
	})

	if err := ctx.Err(); err != nil {
		return errors.Wrapf(err, "hostinit canceled during setup for host %s", j.host.Id)
	}

	setupStartTime := time.Now()
	grip.Info(message.Fields{
		"message": "provisioning host",
		"job":     j.ID(),
		"distro":  j.host.Distro.Id,
		"host_id": j.host.Id,
	})

	if err := j.provisionHost(ctx, settings); err != nil {
		event.LogHostProvisionError(j.host.Id)

		if j.host.Distro.BootstrapSettings.Method == distro.BootstrapMethodSSH {
			grip.Error(message.WrapError(j.host.DeleteJasperCredentials(ctx, j.env), message.Fields{
				"message":  "could not delete Jasper credentials after failed provision attempt",
				"host_id":  j.host.Id,
				"distro":   j.host.Distro.Id,
				"attempts": j.host.ProvisionAttempts,
				"job":      j.ID(),
			}))
		}

		grip.Error(message.WrapError(err, message.Fields{
			"message":  "provisioning host encountered error",
			"job":      j.ID(),
			"distro":   j.host.Distro.Id,
			"host_id":  j.host.Id,
			"attempts": j.host.ProvisionAttempts,
		}))
	}

	// ProvisionHost allows hosts to fail provisioning a few
	// times during host start up, to account for the fact
	// that hosts often need extra time to come up.
	//
	// In these cases, ProvisionHost returns a nil error but
	// does not change the host status.
	if j.host.Status == evergreen.HostProvisioning && !j.host.Provisioned {
		grip.Info(message.Fields{
			"attempts": j.host.ProvisionAttempts,
			"host_id":  j.host.Id,
			"job":      j.ID(),
			"message":  "retrying provisioning",
		})
		return nil
	}

	grip.Info(message.Fields{
		"message":  "successfully finished provisioning host",
		"host_id":  j.host.Id,
		"DNS":      j.host.Host,
		"distro":   j.host.Distro.Id,
		"job":      j.ID(),
		"attempts": j.host.ProvisionAttempts,
		"runtime":  time.Since(setupStartTime),
	})

	return nil
}

func (j *setupHostJob) setDNSName(ctx context.Context, cloudMgr cloud.Manager, settings *evergreen.Settings) error {
	if j.host.Host != "" {
		return nil
	}

	// get the DNS name for the host
	hostDNS, err := cloudMgr.GetDNSName(ctx, j.host)
	if err != nil {
		return errors.Wrapf(err, "error checking DNS name for host %s", j.host.Id)
	}

	// sanity check for the host DNS name
	if hostDNS == "" {
		// DNS name not required if IP address set
		if j.host.IP != "" {
			return nil
		}
		return errors.Errorf("instance %s is running but not returning a DNS name or IP address", j.host.Id)
	}

	// update the host's DNS name
	if err = j.host.SetDNSName(hostDNS); err != nil {
		return errors.Wrapf(err, "error setting DNS name for host %s", j.host.Id)
	}

	return nil
}

// runHostSetup transfers the specified setup script for an individual host.
// Returns the output from running the script remotely, as well as any error
// that occurs. If the script exits with a non-zero exit code, the error will be
// non-nil.
func (j *setupHostJob) runHostSetup(ctx context.Context, settings *evergreen.Settings) error {
	// fetch the appropriate cloud provider for the host
	mgrOpts, err := cloud.GetManagerOptions(j.host.Distro)
	if err != nil {
		return errors.Wrapf(err, "can't get ManagerOpts for '%s'", j.host.Id)
	}
	cloudMgr, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		return errors.Wrapf(err,
			"failed to get cloud manager for host %s with provider %s",
			j.host.Id, j.host.Provider)
	}

	if err = j.setDNSName(ctx, cloudMgr, settings); err != nil {
		return errors.Wrap(err, "error setting DNS name")
	}

	// run the function scheduled for when the host is up
	if err = cloudMgr.OnUp(ctx, j.host); err != nil {
		err = errors.Wrapf(err, "OnUp callback failed for host %s", j.host.Id)
		return err
	}

	switch j.host.Distro.BootstrapSettings.Method {
	case distro.BootstrapMethodNone:
		return nil
	case distro.BootstrapMethodUserData:
		// Updating the host LCT prevents the agent monitor deploy job from
		// running. The agent monitor should be started by the user data script.
		grip.Error(message.WrapError(j.host.UpdateLastCommunicated(), message.Fields{
			"message": "failed to update host's last communication time",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))

		// Do not set the host to running - hosts bootstrapped with user data
		// are not considered done provisioning until user data has finished
		// running.
		if err = j.host.SetProvisionedNotRunning(); err != nil {
			return errors.Wrapf(err, "error marking host %s as provisioned", j.host.Id)
		}

		grip.Info(message.Fields{
			"message":                 "host successfully provisioned by app server, awaiting host to finish provisioning itself",
			"host_id":                 j.host.Id,
			"distro":                  j.host.Distro.Id,
			"provisioning":            j.host.Distro.BootstrapSettings.Method,
			"provider":                j.host.Provider,
			"attempts":                j.host.ProvisionAttempts,
			"job":                     j.ID(),
			"provision_duration_secs": time.Since(j.host.CreationTime).Seconds(),
		})
		return nil
	case distro.BootstrapMethodSSH:
		if err = setupJasper(ctx, j.env, settings, j.host); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not set up Jasper",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			return errors.Wrapf(err, "error putting Jasper on host '%s'", j.host.Id)
		}
		grip.Info(message.Fields{
			"message": "successfully fetched Jasper binary and started service",
			"host_id": j.host.Id,
			"job":     j.ID(),
			"distro":  j.host.Distro.Id,
		})
	}

	// Do not copy setup scripts to task-spawned hosts
	if j.host.SpawnOptions.SpawnedByTask {
		return nil
	}

	if j.host.Distro.Setup != "" {
		scriptName := evergreen.SetupScriptName
		if j.host.Distro.IsPowerShellSetup() {
			scriptName = evergreen.PowerShellSetupScriptName
		}
		var output string
		output, err = copyScript(ctx, j.env, settings, j.host, filepath.Join(j.host.Distro.HomeDir(), scriptName), j.host.Distro.Setup)
		if err != nil {
			return errors.Wrapf(err, "error copying setup script %s to host %s: %s",
				scriptName, j.host.Id, output)
		}
	}

	if j.host.Distro.Teardown != "" {
		var output string
		output, err = copyScript(ctx, j.env, settings, j.host, filepath.Join(j.host.Distro.HomeDir(), evergreen.TeardownScriptName), j.host.Distro.Teardown)
		if err != nil {
			return errors.Wrapf(err, "error copying teardown script %s to host %s: %s",
				evergreen.TeardownScriptName, j.host.Id, output)
		}
	}

	return nil
}

// setupJasper sets up the Jasper service on the host by putting the credentials
// on the host, downloading the latest version of Jasper, and restarting the
// Jasper service.
func setupJasper(ctx context.Context, env evergreen.Environment, settings *evergreen.Settings, h *host.Host) error {
	if err := setupServiceUser(ctx, env, settings, h); err != nil {
		return errors.Wrap(err, "error setting up service user")
	}

	if err := putJasperCredentials(ctx, env, settings, h); err != nil {
		return errors.Wrap(err, "error putting Jasper credentials on remote host")
	}

	if err := doFetchAndReinstallJasper(ctx, env, h); err != nil {
		return errors.Wrap(err, "error starting Jasper service on remote host")
	}

	return nil
}

// putJasperCredentials creates Jasper credentials for the host and puts the
// credentials file on the host.
func putJasperCredentials(ctx context.Context, env evergreen.Environment, settings *evergreen.Settings, h *host.Host) error {
	creds, err := h.GenerateJasperCredentials(ctx, env)
	if err != nil {
		return errors.Wrap(err, "could not generate Jasper credentials for host")
	}

	writeCmds, err := h.WriteJasperCredentialsFilesCommands(settings.Splunk, creds)
	if err != nil {
		return errors.Wrap(err, "could not get command to write Jasper credentials file")
	}

	grip.Info(message.Fields{
		"message": "putting Jasper credentials on host",
		"host_id": h.Id,
		"distro":  h.Distro.Id,
	})

	if logs, err := h.RunSSHCommand(ctx, writeCmds); err != nil {
		return errors.Wrapf(err, "error copying credentials to remote machine: command returned %s", logs)
	}

	if err := h.SaveJasperCredentials(ctx, env, creds); err != nil {
		return errors.Wrap(err, "error saving credentials")
	}

	return nil
}

// setupServiceUser runs the SSH commands on the host that sets up the service
// user on the host.
func setupServiceUser(ctx context.Context, env evergreen.Environment, settings *evergreen.Settings, h *host.Host) error {
	if !h.Distro.IsWindows() {
		return nil
	}

	cmds, err := h.SetupServiceUserCommands()
	if err != nil {
		return errors.Wrap(err, "could not get command to set up service user")
	}

	path := filepath.Join(h.Distro.HomeDir(), "setup-user.ps1")
	if output, err := copyScript(ctx, env, settings, h, path, cmds); err != nil {
		return errors.Wrapf(err, "error copying script to set up service user: %s", output)
	}

	if logs, err := h.RunSSHCommand(ctx, fmt.Sprintf("powershell ./%s && rm -f ./%s", filepath.Base(path), filepath.Base(path))); err != nil {
		return errors.Wrapf(err, "error while setting up service user: command returned %s", logs)
	}

	return nil
}

// doFetchAndReinstallJasper runs the SSH command that downloads the latest
// Jasper binary and restarts the service.
func doFetchAndReinstallJasper(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	cmd := h.FetchAndReinstallJasperCommands(env.Settings())
	if logs, err := h.RunSSHCommand(ctx, cmd); err != nil {
		return errors.Wrapf(err, "error while fetching Jasper binary and installing service on remote host: command returned '%s'", logs)
	}
	return nil
}

// copyScript writes a given script as file "name" to the target host. This works
// by creating a local copy of the script on the runner's machine, scping it over
// then removing the local copy.
func copyScript(ctx context.Context, env evergreen.Environment, settings *evergreen.Settings, h *host.Host, name, script string) (string, error) {
	// parse the hostname into the user, host and port
	startAt := time.Now()

	hostInfo, err := h.GetSSHInfo()
	if err != nil {
		return "", errors.WithStack(err)
	}

	user := h.Distro.User
	if hostInfo.User != "" {
		user = hostInfo.User
	}

	// create a temp file for the script
	file, err := ioutil.TempFile("", filepath.Base(name))
	if err != nil {
		return "", errors.Wrap(err, "error creating temporary script file")
	}
	if err = os.Chmod(file.Name(), 0700); err != nil {
		return "", errors.Wrap(err, "error setting file permissions")
	}
	defer func() {
		errCtx := message.Fields{
			"operation": "cleaning up after script copy",
			"file":      file.Name(),
			"distro":    h.Distro.Id,
			"host_id":   h.Host,
			"name":      name,
		}
		grip.Error(message.WrapError(file.Close(), errCtx))
		grip.Error(message.WrapError(os.Remove(file.Name()), errCtx))
		grip.Debug(message.Fields{
			"operation":     "copy script",
			"file":          file.Name(),
			"distro":        h.Distro.Id,
			"host_id":       h.Host,
			"name":          name,
			"duration_secs": time.Since(startAt).Seconds(),
		})
	}()

	expanded, err := expandScript(script, settings)
	if err != nil {
		return "", errors.Wrapf(err, "error expanding script for host %s", h.Id)
	}
	if _, err = io.WriteString(file, expanded); err != nil {
		return "", errors.Wrap(err, "error writing local script")
	}

	sshOptions, err := h.GetSSHOptions(settings)
	if err != nil {
		return "", errors.Wrapf(err, "error getting ssh options for host %s", h.Id)
	}

	scpCmdOut := util.NewMBCappedWriter()
	scpArgs := buildScpCommand(file.Name(), name, hostInfo, user, sshOptions)

	scpCmd := env.JasperManager().CreateCommand(ctx).Add(scpArgs).
		RedirectErrorToOutput(true).SetOutputWriter(scpCmdOut)

	// run the command to scp the script with a timeout
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, scpTimeout)
	defer cancel()

	err = scpCmd.Run(ctx)

	return scpCmdOut.String(), errors.Wrap(err, "error copying script to remote machine")
}

func buildScpCommand(src, dst string, info *util.StaticHostInfo, user string, opts []string) []string {
	return append(append([]string{"scp", "-vvv", "-P", info.Port}, opts...), src, fmt.Sprintf("%s@%s:%s", user, info.Hostname, dst))
}

// Build the setup script that will need to be run on the specified host.
func expandScript(s string, settings *evergreen.Settings) (string, error) {
	// replace expansions in the script
	exp := util.NewExpansions(settings.Expansions)
	script, err := exp.ExpandString(s)
	if err != nil {
		return "", errors.Wrap(err, "expansions error")
	}
	return script, err
}

// Provision the host, and update the database accordingly.
func (j *setupHostJob) provisionHost(ctx context.Context, settings *evergreen.Settings) error {
	grip.Info(message.Fields{
		"job":     j.ID(),
		"host_id": j.host.Id,
		"distro":  j.host.Distro.Id,
		"message": "setting up host",
	})

	incErr := j.host.IncProvisionAttempts()
	grip.Critical(message.WrapError(incErr, message.Fields{
		"job":           j.ID(),
		"host_id":       j.host.Id,
		"attempt_value": j.host.ProvisionAttempts,
		"distro":        j.host.Distro.Id,
		"operation":     "increment provisioning errors failed",
	}))

	err := j.runHostSetup(ctx, settings)
	if err != nil {
		if shouldRetryProvisioning(j.host) {
			return nil
		}

		event.LogProvisionFailed(j.host.Id, "")
		// mark the host's provisioning as failed
		grip.Error(message.WrapError(j.host.SetUnprovisioned(), message.Fields{
			"operation": "setting host unprovisioned",
			"attempts":  j.host.ProvisionAttempts,
			"distro":    j.host.Distro.Id,
			"job":       j.ID(),
			"host_id":   j.host.Id,
		}))

		return errors.Wrapf(err, "error initializing host %s", j.host.Id)
	}

	if j.host.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
		return nil
	}

	if j.host.IsVirtualWorkstation {
		if err = attachVolume(ctx, j.env, j.host); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "can't attach volume",
				"host_id":  j.host.Id,
				"distro":   j.host.Distro.Id,
				"attempts": j.host.ProvisionAttempts,
				"job":      j.ID(),
			}))
			return nil
		}
	}

	// If this is a spawn host
	if j.host.ProvisionOptions != nil && j.host.ProvisionOptions.LoadCLI {
		grip.Infof("Uploading client binary to host %s", j.host.Id)
		if err = j.setupSpawnHost(ctx, settings); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "failed to load client binary onto host",
				"job":     j.ID(),
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
			}))

			grip.Error(message.WrapError(j.host.SetUnprovisioned(), message.Fields{
				"operation": "setting host unprovisioned",
				"job":       j.ID(),
				"distro":    j.host.Distro.Id,
				"host_id":   j.host.Id,
			}))
			return errors.Wrapf(err, "Failed to load client binary onto host %s: %+v", j.host.Id, err)
		}

		grip.Infof("Running setup script for spawn host %s", j.host.Id)
		// run the setup script with the agent
		if logs, err := j.host.RunSSHCommand(ctx, j.host.SetupCommand()); err != nil {
			grip.Error(message.WrapError(j.host.SetUnprovisioned(), message.Fields{
				"operation": "setting host unprovisioned",
				"host_id":   j.host.Id,
				"distro":    j.host.Distro.Id,
				"job":       j.ID(),
			}))
			event.LogProvisionFailed(j.host.Id, logs)
			return errors.Wrapf(err, "error running setup script on remote host: %s", logs)
		}

		if j.host.ProvisionOptions.OwnerId != "" && j.host.ProvisionOptions.TaskId != "" {
			grip.Info(message.Fields{
				"message": "fetching data for task on host",
				"task":    j.host.ProvisionOptions.TaskId,
				"distro":  j.host.Distro.Id,
				"host_id": j.host.Id,
				"job":     j.ID(),
			})

			grip.Error(message.WrapError(j.fetchRemoteTaskData(ctx, settings),
				message.Fields{
					"message": "failed to fetch data onto host",
					"task":    j.host.ProvisionOptions.TaskId,
					"host_id": j.host.Id,
					"job":     j.ID(),
				}))
		}
	}

	grip.Info(message.Fields{
		"message": "setup complete for host",
		"host_id": j.host.Id,
		"job":     j.ID(),
		"distro":  j.host.Distro.Id,
	})

	// the setup was successful. update the host accordingly in the database
	if err := j.host.MarkAsProvisioned(); err != nil {
		return errors.Wrapf(err, "error marking host %s as provisioned", j.host.Id)
	}

	grip.Info(message.Fields{
		"host_id":                 j.host.Id,
		"distro":                  j.host.Distro.Id,
		"provider":                j.host.Provider,
		"attempts":                j.host.ProvisionAttempts,
		"job":                     j.ID(),
		"message":                 "host successfully provisioned",
		"provision_duration_secs": j.host.ProvisionTime.Sub(j.host.CreationTime).Seconds(),
	})

	return nil
}

// setupSpawnHost places the evergreen command line client on the host, places a
// copy of the user's settings onto the host, and makes the binary appear in the
// PATH when the user logs in. If the spawn host is loading task data, it is
// also retrieved.
func (j *setupHostJob) setupSpawnHost(ctx context.Context, settings *evergreen.Settings) error {
	script, err := j.host.SpawnHostSetupCommands(settings)
	if err != nil {
		return errors.Wrap(err, "could not create script to setup spawn host")
	}

	if err != nil {
		return errors.Wrapf(err, "error parsing ssh info %s", j.host.Host)
	}

	curlCtx, cancel := context.WithTimeout(ctx, evergreenCurlTimeout)
	defer cancel()
	output, err := j.host.RunSSHCommand(curlCtx, j.host.CurlCommand(settings))
	if err != nil {
		return errors.Wrapf(err, "error running command to get evergreen binary on  spawn host: %s", output)
	}
	if curlCtx.Err() != nil {
		return errors.Wrap(curlCtx.Err(), "timed out curling evergreen binary")
	}

	spawnHostSetupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if output, err := j.host.RunSSHShellScript(spawnHostSetupCtx, script); err != nil {
		return errors.Wrapf(err, "error running command to set up spawn host: %s", output)
	}

	return nil
}

func (j *setupHostJob) fetchRemoteTaskData(ctx context.Context, settings *evergreen.Settings) error {
	sshInfo, err := j.host.GetSSHInfo()
	if err != nil {
		return errors.Wrapf(err, "error parsing ssh info %s", j.host.Host)
	}

	cmd := strings.Join(j.host.SpawnHostGetTaskDataCommand(), " ")
	var output string
	fetchTimeout := 15 * time.Minute
	getTaskDataCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	if j.host.Distro.LegacyBootstrap() {
		output, err = j.host.RunSSHCommandWithTimeout(getTaskDataCtx, cmd, fetchTimeout)
	} else {
		var logs []string
		// We have to run this in the Cygwin shell in order for git clone to
		// use the correct SSH key.
		logs, err = j.host.RunJasperProcess(getTaskDataCtx, j.env, &options.Create{
			Args: []string{j.host.Distro.ShellBinary(), "-l", "-c", cmd},
		})
		output = strings.Join(logs, " ")
	}

	grip.Error(message.WrapError(err, message.Fields{
		"message": fmt.Sprintf("fetch-artifacts-%s", j.host.ProvisionOptions.TaskId),
		"host_id": sshInfo.Hostname,
		"cmd":     cmd,
		"job":     j.ID(),
		"logs":    output,
	}))

	return errors.Wrap(err, "could not fetch remote task data")
}

func (j *setupHostJob) tryRequeue(ctx context.Context) {
	if shouldRetryProvisioning(j.host) && j.env.RemoteQueue().Started() {
		job := NewHostSetupJob(j.env, *j.host, fmt.Sprintf("attempt-%d", j.host.ProvisionAttempts))
		job.UpdateTimeInfo(amboy.JobTimeInfo{
			WaitUntil: time.Now().Add(time.Minute),
		})
		err := j.env.RemoteQueue().Put(ctx, job)
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "failed to requeue setup job",
			"host_id":  j.host.Id,
			"job":      j.ID(),
			"distro":   j.host.Distro.Id,
			"attempts": j.host.ProvisionAttempts,
		}))
		j.AddError(err)
	}
}

func shouldRetryProvisioning(h *host.Host) bool {
	return h.ProvisionAttempts <= provisionRetryLimit && h.Status == evergreen.HostProvisioning && !h.Provisioned
}

func attachVolume(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	if !(h.Distro.JasperCommunication() && h.Distro.IsLinux()) {
		return errors.Errorf("host '%s' of distro '%s' doesn't support mounting a volume", h.Id, h.Distro.Id)
	}

	if h.HomeVolume() == nil {
		mgrOpts, err := cloud.GetManagerOptions(h.Distro)
		if err != nil {
			return errors.Wrapf(err, "can't get ManagerOpts for '%s'", h.Id)
		}
		cloudMgr, err := cloud.GetManager(ctx, env, mgrOpts)
		if err != nil {
			return errors.Wrapf(err,
				"failed to get cloud manager for host %s with provider %s",
				h.Id, h.Provider)
		}

		var volume *host.Volume
		if h.HomeVolumeID != "" {
			volume, err = host.FindVolumeByID(h.HomeVolumeID)
			if err != nil {
				return errors.Wrapf(err, "can't get volume '%s'", h.HomeVolumeID)
			}
			if volume == nil {
				return errors.Errorf("volume '%s' does not exist", h.HomeVolumeID)
			}
			var sourceHost *host.Host
			sourceHost, err = host.FindHostWithVolume(h.HomeVolumeID)
			if err != nil {
				return errors.Wrapf(err, "can't get source host for volume '%s'", h.HomeVolumeID)
			}
			if sourceHost != nil {
				return errors.Errorf("volume '%s' is already attached to host '%s'", h.HomeVolumeID, sourceHost.Id)
			}
		} else {
			// create the volume
			volume, err = cloudMgr.CreateVolume(ctx, &host.Volume{
				Size:             h.HomeVolumeSize,
				AvailabilityZone: h.Zone,
				CreatedBy:        h.StartedBy,
				Type:             evergreen.DefaultEBSType,
			})
			if err != nil {
				return errors.Wrapf(err, "can't create a new volume for host '%s'", h.Id)
			}
		}

		// attach to the host
		attachment := host.VolumeAttachment{VolumeID: volume.ID, IsHome: true}
		if err = cloudMgr.AttachVolume(ctx, h, &attachment); err != nil {
			return errors.Wrapf(err, "can't attach volume '%s' to host '%s'", volume.ID, h.Id)
		}
	}

	// run the distro's mount script
	var err error
	for i := 0; i < mountRetryLimit; i++ {
		if err = errors.Wrap(mountLinuxVolume(ctx, env, h), "can't mount volume"); err == nil {
			return nil
		}
		time.Sleep(mountSleepDuration)
	}
	return err
}

func mountLinuxVolume(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	client, err := h.JasperClient(ctx, env)
	if err != nil {
		return errors.Wrap(err, "can't get Jasper client")
	}
	defer func() {
		grip.Warning(message.WrapError(client.CloseConnection(), message.Fields{
			"message": "could not close connection to Jasper",
			"host_id": h.Id,
			"distro":  h.Distro.Id,
		}))
	}()

	homeVolume := h.HomeVolume()
	if homeVolume == nil {
		return errors.Errorf("host '%s' has no home volume", h.Id)
	}
	deviceName := homeVolume.DeviceName
	if h.Distro.HomeVolumeSettings.DeviceName != "" {
		deviceName = h.Distro.HomeVolumeSettings.DeviceName
	}

	// continue on umount mount error
	cmd := client.CreateCommand(ctx).Sudo(true).Append(fmt.Sprintf("umount -f /dev/%s", deviceName)).Background(true)
	if err = cmd.Run(ctx); err != nil {
		return errors.Wrap(err, "can't run umount command")
	}
	exitCode, err := cmd.Wait(ctx)
	if err != nil && exitCode != umountMountErrorCode {
		return errors.Wrap(err, "problem waiting for umount command")
	}

	cmd = client.CreateCommand(ctx).Sudo(true)
	// skip formatting if the volume already contains a filesystem
	cmdOut := util.NewMBCappedWriter()
	if err = client.CreateCommand(ctx).Sudo(true).Append(fmt.Sprintf("file -s /dev/%s", deviceName)).SetOutputWriter(cmdOut).Run(ctx); err != nil {
		return errors.Wrap(err, "problem checking for formatted device")
	}
	if strings.Contains(cmdOut.String(), fmt.Sprintf("/dev/%s: data", deviceName)) {
		cmd.Append(fmt.Sprintf("%s /dev/%s", h.Distro.HomeVolumeSettings.FormatCommand, deviceName))
	}

	cmd.Append(fmt.Sprintf("mkdir -p /%s", evergreen.HomeVolumeDir))
	cmd.Append(fmt.Sprintf("mount /dev/%s /%s", deviceName, evergreen.HomeVolumeDir))
	cmd.Append(fmt.Sprintf("chown -R %s:%s /%s", h.User, h.User, evergreen.HomeVolumeDir))
	cmd.Append(fmt.Sprintf("ln -s /%s %s/%s", evergreen.HomeVolumeDir, h.Distro.HomeDir(), evergreen.HomeVolumeDir))
	if err = cmd.Run(ctx); err != nil {
		return errors.Wrap(err, "problem running mount commands")
	}

	// write to fstab so the volume is mounted on restart
	err = client.CreateCommand(ctx).Sudo(true).SetInputBytes([]byte(fmt.Sprintf("/dev/%s /%s auto noatime 0 0", deviceName, evergreen.HomeVolumeDir))).Append("tee --append /etc/fstab").Run(ctx)
	if err != nil {
		return errors.Wrap(err, "problem appending to fstab")
	}

	return nil
}
