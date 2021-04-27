package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	jcli "github.com/mongodb/jasper/cli"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
)

const (
	jasperRestartJobName = "jasper-restart"
	expirationCutoff     = 7 * 24 * time.Hour // 1 week

	// maxHostReprovisioningJobTime is the maximum amount of time a
	// reprovisioning job (i.e. a job that modifies how the host is provisioned
	// after initial provisioning is complete) can run.
	maxHostReprovisioningJobTime = 5 * time.Minute
)

func init() {
	registry.AddJobType(jasperRestartJobName, func() amboy.Job { return makeJasperRestartJob() })
}

type jasperRestartJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	// If set, the restart will be done by sending requests to the Jasper
	// service. Otherwise, the commands will be run as regular SSH commands
	// without communicating with Jasper at all.
	RestartThroughJasper bool `bson:"restart_through_jasper" json:"restart_through_jasper" yaml:"restart_through_jasper"`

	env      evergreen.Environment
	settings *evergreen.Settings
	host     *host.Host
}

func makeJasperRestartJob() *jasperRestartJob {
	j := &jasperRestartJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    jasperRestartJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewJasperRestartJob creates a job that restarts an existing Jasper service
// with new credentials.
func NewJasperRestartJob(env evergreen.Environment, h host.Host, restartThroughJasper bool, ts string) amboy.Job {
	j := makeJasperRestartJob()
	j.env = env
	j.settings = env.Settings()
	j.HostID = h.Id
	j.host = &h
	j.RestartThroughJasper = restartThroughJasper
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: maxHostReprovisioningJobTime,
	})
	j.SetScopes([]string{reprovisioningJobScope(h.Id)})
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(10),
		WaitUntil:   utility.ToTimeDurationPtr(time.Minute),
	})
	jobID := fmt.Sprintf("%s.%s.%s", jasperRestartJobName, j.HostID, ts)
	if restartThroughJasper {
		jobID += ".restart-through-jasper"
	}
	j.SetID(jobID)
	return j
}

func (j *jasperRestartJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(ctx); err != nil {
		j.AddError(err)
		return
	}

	if j.host.NeedsReprovision != host.ReprovisionJasperRestart || (j.host.Status != evergreen.HostProvisioning && j.host.Status != evergreen.HostRunning) {
		return
	}

	defer func() {
		if j.HasErrors() {
			event.LogHostJasperRestartError(j.host.Id, j.Error())

			if j.RetryInfo().GetRemainingAttempts() < j.RetryInfo().GetMaxAttempts()/2 {
				j.RestartThroughJasper = false
			}
		}
	}()

	// The host cannot be reprovisioned until the host's agent monitor has been
	// stopped.
	if j.host.StartedBy == evergreen.User && (!j.host.NeedsNewAgentMonitor || j.host.RunningTask != "") {
		j.AddRetryableError(errors.New("cannot reprovision the host while the host's agent monitor is still running"))
		return
	}

	if err := j.host.UpdateLastCommunicated(); err != nil {
		j.AddError(errors.Wrap(err, "updating host last communication time"))
	}

	creds, err := j.host.GenerateJasperCredentials(ctx, j.env)
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "generating new Jasper credentials"))
		return
	}

	writeCredentialsCmd, err := j.host.WriteJasperCredentialsFilesCommands(j.settings.Splunk, creds)
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "building command to write Jasper credentials file"))
		return
	}
	writePreconditionScriptsCmd := j.host.WriteJasperPreconditionScriptsCommands()

	if j.RestartThroughJasper {
		client, err := j.host.JasperClient(ctx, j.env)
		if err != nil {
			j.AddRetryableError(errors.Wrap(err, "connecting via a Jasper client"))
			return
		}
		defer func(client remote.Manager) {
			// Ignore the error because restarting Jasper will close the
			// connection.
			_ = client.CloseConnection()
		}(client)

		// We use this ID to later verify the current running Jasper service.
		// When Jasper is restarted, its ID should be different to indicate it
		// is a new Jasper service.
		serviceID := client.ID()

		writeCredentialsOpts := &options.Create{
			Args: []string{
				j.host.Distro.ShellBinary(), "-l", "-c", writeCredentialsCmd,
			},
		}
		var output []string
		if output, err = j.host.RunJasperProcess(ctx, j.env, writeCredentialsOpts); err != nil {
			j.AddRetryableError(errors.Wrapf(err, "replacing Jasper credentials on host: %s", output))
			return
		}

		if len(writePreconditionScriptsCmd) != 0 {
			writePreconditionScriptsOpts := &options.Create{
				Args: []string{
					j.host.Distro.ShellBinary(), "-l", "-c", writePreconditionScriptsCmd,
				},
			}
			if output, err = j.host.RunJasperProcess(ctx, j.env, writePreconditionScriptsOpts); err != nil {
				j.AddRetryableError(errors.Wrapf(err, "writing Jasper precondition scripts to host"))
				return
			}
		}

		fetchCmd := j.host.FetchJasperCommand(j.settings.HostJasper)
		// We have to kill the Jasper service from within a process that it
		// creates so that the system restarts the service with the new
		// credentials file. This will not work on Windows.
		killCmd := fmt.Sprintf("pgrep -f '%s' | xargs kill", strings.Join(jcli.BuildServiceCommand(j.settings.HostJasper.BinaryName), " "))
		restartJasperOpts := &options.Create{
			Args: []string{
				j.host.Distro.ShellBinary(), "-c",
				fmt.Sprintf("%s && %s ", fetchCmd, killCmd),
			},
		}

		if _, err = j.host.StartJasperProcess(ctx, j.env, restartJasperOpts); err != nil {
			j.AddRetryableError(errors.Wrap(err, "restarting the Jasper service"))
			j.AddError(err)
			return
		}

		// Verify that the ID of the service has changed to indicate that the
		// service restarted.
		if client, err = j.host.JasperClient(ctx, j.env); err != nil {
			j.AddRetryableError(errors.Wrap(err, "connecting via a Jasper client"))
			return
		}
		defer func(client remote.Manager) {
			j.AddError(errors.Wrap(client.CloseConnection(), "closing connection to Jasper"))
		}(client)

		newServiceID := client.ID()
		if newServiceID == "" {
			j.AddRetryableError(errors.New("new Jasper service's ID was empty, but should be set"))
			return
		}
		if newServiceID == serviceID {
			j.AddRetryableError(errors.New("new Jasper service's ID should not match old Jasper service's ID"))
			j.AddError(err)
			return
		}
	} else {
		if output, err := j.host.RunSSHCommand(ctx, writeCredentialsCmd); err != nil {
			j.AddRetryableError(errors.Wrapf(err, "running SSH command to write credentials file: %s", output))
			return
		}
		if len(writePreconditionScriptsCmd) != 0 {
			if output, err := j.host.RunSSHCommand(ctx, writeCredentialsCmd); err != nil {
				j.AddRetryableError(errors.Wrapf(err, "running SSH command to write precondition scripts: %s", output))
				return
			}
		}

		if output, err := j.host.RunSSHCommand(ctx, j.host.FetchJasperCommand(j.settings.HostJasper)); err != nil {
			j.AddRetryableError(errors.Wrapf(err, "running SSH command to download Jasper: %s", output))
			return
		}

		if output, err := j.host.RunSSHCommand(ctx, j.host.RestartJasperCommand(j.settings.HostJasper)); err != nil {
			j.AddRetryableError(errors.Wrapf(err, "running SSH command to restart Jasper: %s", output))
			return
		}
	}

	if err := j.host.MarkAsReprovisioned(); err != nil {
		j.AddRetryableError(errors.Wrap(err, "marking host as reprovisioned"))
		return
	}

	// Since this updates the TTL on the credentials, can only overwrite the
	// existing Jasper credentials with the new expiration once we have
	// reasonable confidence that the host has a Jasper service running with the
	// new credentials.
	if err := j.host.SaveJasperCredentials(ctx, j.env, creds); err != nil {
		j.AddRetryableError(errors.Wrap(err, "saving new Jasper credentials"))
		return
	}

	event.LogHostJasperRestarted(j.host.Id, j.settings.HostJasper.Version)

	grip.Info(message.Fields{
		"message": "restarted Jasper service with new credentials",
		"host_id": j.host.Id,
		"distro":  j.host.Distro.Id,
		"version": j.settings.HostJasper.Version,
	})

	if j.host.StartedBy != evergreen.User {
		return
	}

	// If this doesn't succeed, a new agent monitor will be deployed when LCT
	// elapses.
	if err := j.host.SetNeedsNewAgentMonitor(true); err != nil {
		j.AddError(errors.Wrap(err, "marking host as needing new agent monitor"))
		return
	}
}

// populateIfUnset populates the unset job fields.
func (j *jasperRestartJob) populateIfUnset(ctx context.Context) error {
	if j.host == nil {
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			return errors.Wrapf(err, "could not find host '%s'", j.HostID)
		}
		j.host = h
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.settings == nil {
		j.settings = j.env.Settings()
	}

	return nil
}

// reprovisioningJobScope returns the job scope for a reprovisioning job.
func reprovisioningJobScope(hostID string) string {
	return fmt.Sprintf("reprovisioning.%s", hostID)
}
