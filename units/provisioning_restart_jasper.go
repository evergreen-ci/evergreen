package units

import (
	"context"
	"fmt"
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
	"github.com/pkg/errors"
)

const (
	restartJasperJobName = "restart-jasper"
	expirationCutoff     = 7 * 24 * time.Hour // 1 week

	// maxHostReprovisioningJobTime is the maximum amount of time a
	// reprovisioning job (i.e. a job that modifies how the host is provisioned
	// after initial provisioning is complete) can run.
	maxHostReprovisioningJobTime = 5 * time.Minute
)

func init() {
	registry.AddJobType(restartJasperJobName, func() amboy.Job { return makeRestartJasperJob() })
}

type restartJasperJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`

	env      evergreen.Environment
	settings *evergreen.Settings
	host     *host.Host
}

func makeRestartJasperJob() *restartJasperJob {
	j := &restartJasperJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    restartJasperJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewRestartJasperJob creates a job that restarts an existing Jasper service
// with new credentials.
func NewRestartJasperJob(env evergreen.Environment, h host.Host, ts string) amboy.Job {
	j := makeRestartJasperJob()
	j.env = env
	j.settings = env.Settings()
	j.HostID = h.Id
	j.host = &h
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: maxHostReprovisioningJobTime,
	})
	j.SetScopes([]string{reprovisioningJobScope(h.Id)})
	j.SetShouldApplyScopesOnEnqueue(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(10),
		WaitUntil:   utility.ToTimeDurationPtr(time.Minute),
	})
	j.SetID(fmt.Sprintf("%s.%s.%s", restartJasperJobName, h.Id, ts))
	return j
}

func (j *restartJasperJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(ctx); err != nil {
		j.AddError(err)
		return
	}

	if j.host.NeedsReprovision != host.ReprovisionRestartJasper || (j.host.Status != evergreen.HostProvisioning && j.host.Status != evergreen.HostRunning) {
		return
	}

	defer func() {
		if j.HasErrors() {
			// Static hosts should be quarantined if they've run out of attempts
			// to restart jasper.
			if j.RetryInfo().GetRemainingAttempts() == 0 && j.host.Provider == evergreen.ProviderNameStatic {
				j.AddError(j.host.SetStatusAtomically(evergreen.HostQuarantined, evergreen.User, "static host has run out of attempts to reprovision"))
			}

			event.LogHostJasperRestartError(j.host.Id, j.Error())
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
func (j *restartJasperJob) populateIfUnset(ctx context.Context) error {
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
