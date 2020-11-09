package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
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
	jasperRestartJobName    = "jasper-restart"
	expirationCutoff        = 7 * 24 * time.Hour // 1 week
	jasperRestartRetryLimit = 10

	// maxHostReprovisioningJobTime is the maximum amount of time a
	// reprovisioning job (i.e. a job that modifies how the host is provisioned
	// after initial provisioning is complete) can run.
	maxHostReprovisioningJobTime = 5 * time.Minute
)

func init() {
	registry.AddJobType(jasperRestartJobName, func() amboy.Job { return makeJasperRestartJob() })
}

type jasperRestartJob struct {
	job.Base              `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID                string    `bson:"host_id" json:"host_id" yaml:"host_id"`
	CredentialsExpiration time.Time `bson:"credentials_expiration" json:"credentials_expiration" yaml:"credentials_expiration"`
	// If set, the restart will be done by sending requests to the Jasper
	// service. Otherwise, the commands will be run as regular SSH commands
	// without communicating with Jasper at all.
	RestartThroughJasper bool   `bson:"restart_through_jasper" json:"restart_through_jasper" yaml:"restart_through_jasper"`
	CurrentAttempt       int    `bson:"current_attempt" json:"current_attempt" yaml:"current_attempt"`
	Timestamp            string `bson:"timestamp" json:"timestamp" yaml:"timestamp"`

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
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: maxHostReprovisioningJobTime,
	})
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewJasperRestartJob creates a job that restarts an existing Jasper service
// with new credentials.
func NewJasperRestartJob(env evergreen.Environment, h host.Host, expiration time.Time, restartThroughJasper bool, ts string, attempt int) amboy.Job {
	j := makeJasperRestartJob()
	j.env = env
	j.settings = env.Settings()
	j.HostID = h.Id
	j.host = &h
	j.CredentialsExpiration = expiration
	j.RestartThroughJasper = restartThroughJasper
	j.CurrentAttempt = attempt
	j.Timestamp = ts
	jobID := fmt.Sprintf("%s.%s.%s.attempt-%d", jasperRestartJobName, j.HostID, ts, attempt)
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
		grip.Error(message.WrapError(j.host.SetReprovisioningLocked(false), message.Fields{
			"message": "could not clear host provisioning lock",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		if j.HasErrors() {
			event.LogHostJasperRestartError(j.host.Id, j.Error())

			if err := j.tryRequeue(ctx); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":  "could not requeue Jasper restart job",
					"host_id":  j.host.Distro.Id,
					"distro:":  j.host.Distro.Id,
					"expires":  j.CredentialsExpiration,
					"attempts": j.CurrentAttempt,
					"job":      j.ID(),
				}))
				return
			}
		}
	}()

	// Lock the provisioning state to prevent other provisioning jobs from
	// running.
	if err := j.host.SetReprovisioningLockedAtomically(true); err != nil {
		grip.Info(message.WrapError(err, message.Fields{
			"message": "provisioning already locked, returning from job",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		return
	}

	// The host cannot be reprovisioned until the host's agent monitor has been
	// stopped.
	if j.host.StartedBy == evergreen.User && (!j.host.NeedsNewAgentMonitor || j.host.RunningTask != "") {
		grip.Error(message.WrapError(j.tryRequeue(ctx), message.Fields{
			"message": "could not enqueue job to retry provisioning conversion when host's agent monitor is still running",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		return
	}

	// Update LCT to prevent other provisioning jobs from running.
	if err := j.host.UpdateLastCommunicated(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not update host communication time",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	grip.Info(message.Fields{
		"message": "restarting Jasper service on host",
		"host_id": j.host.Id,
		"distro":  j.host.Distro.Id,
		"job":     j.ID(),
	})

	creds, err := j.host.GenerateJasperCredentials(ctx, j.env)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "problem generating new Jasper credentials",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	writeCredentialsCmd, err := j.host.WriteJasperCredentialsFilesCommands(j.settings.Splunk, creds)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not build command to write Jasper credentials file",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}
	writePreconditionScriptsCmd := j.host.WriteJasperPreconditionScriptsCommands()

	if j.RestartThroughJasper {
		client, err := j.host.JasperClient(ctx, j.env)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not get Jasper client",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
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
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not replace existing Jasper credentials on host",
				"logs":    output,
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}

		if len(writePreconditionScriptsCmd) != 0 {
			writePreconditionScriptsOpts := &options.Create{
				Args: []string{
					j.host.Distro.ShellBinary(), "-l", "-c", writePreconditionScriptsCmd,
				},
			}
			if output, err = j.host.RunJasperProcess(ctx, j.env, writePreconditionScriptsOpts); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "could not write Jasper precondition scripts to host",
					"logs":    output,
					"host_id": j.host.Id,
					"distro":  j.host.Distro.Id,
					"job":     j.ID(),
				}))
				j.AddError(err)
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
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not restart Jasper service",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}

		// Verify that the ID of the service has changed to indicate that the
		// service restarted.
		if client, err = j.host.JasperClient(ctx, j.env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not get Jasper client",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}
		defer func(client remote.Manager) {
			grip.Warning(message.WrapError(client.CloseConnection(), message.Fields{
				"message": "could not close connection to Jasper",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
		}(client)

		newServiceID := client.ID()
		if newServiceID == "" {
			err := errors.New("new service ID returned empty")
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "new service ID should be non-empty",
				"host_id":   j.host.Id,
				"distro":    j.host.Distro.Id,
				"jasper_id": serviceID,
				"job":       j.ID(),
			}))
			j.AddError(err)
			return
		}
		if newServiceID == serviceID {
			err := errors.New("new service ID should not match current ID")
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "Jasper service should have been restarted, but service is still showing same ID",
				"host_id":   j.host.Id,
				"distro":    j.host.Distro.Id,
				"jasper_id": serviceID,
				"job":       j.ID(),
			}))
			j.AddError(err)
			return
		}
	} else {
		if output, err := j.host.RunSSHCommand(ctx, writeCredentialsCmd); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not run SSH command to write credentials file",
				"logs":    output,
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}
		if len(writePreconditionScriptsCmd) != 0 {
			if output, err := j.host.RunSSHCommand(ctx, writeCredentialsCmd); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "could not run SSH command to write precondition scripts",
					"logs":    output,
					"host_id": j.host.Id,
					"distro":  j.host.Distro.Id,
					"job":     j.ID(),
				}))
				j.AddError(err)
				return
			}
		}

		if output, err := j.host.RunSSHCommand(ctx, j.host.FetchJasperCommand(j.settings.HostJasper)); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not run SSH command to download Jasper",
				"logs":    output,
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}

		if output, err := j.host.RunSSHCommand(ctx, j.host.RestartJasperCommand(j.settings.HostJasper)); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not run SSH command to restart Jasper",
				"logs":    output,
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}
	}

	if err := j.host.MarkAsReprovisioned(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not mark host as provisioned",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	// Since this updates the TTL on the credentials, can only save the Jasper
	// credentials with the new expiration once we have reasonable confidence
	// that the host has a Jasper service running with the new credentials and
	// the agent monitor will be deployed.
	if err := j.host.SaveJasperCredentials(ctx, j.env, creds); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "problem saving new Jasper credentials",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	grip.Info(message.Fields{
		"message": "restarted Jasper service with new credentials",
		"host_id": j.host.Id,
		"distro":  j.host.Distro.Id,
		"version": j.settings.HostJasper.Version,
	})
	event.LogHostJasperRestarted(j.host.Id, j.settings.HostJasper.Version)

	if j.host.StartedBy != evergreen.User {
		return
	}

	// If this doesn't succeed, a new agent monitor will be deployed
	// when LCT elapses.
	if err := j.host.SetNeedsNewAgentMonitor(true); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not mark host as needing new agent monitor",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		return
	}
}

// credentialsExpireBefore returns whether or not the host's Jasper credentials
// expire before the given cutoff.
func (j *jasperRestartJob) credentialsExpireBefore(cutoff time.Duration) bool {
	return time.Now().Add(cutoff).After(j.CredentialsExpiration)
}

// populateIfUnset populates the unset job fields.
func (j *jasperRestartJob) populateIfUnset(ctx context.Context) error {
	if j.host == nil {
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			return errors.Wrapf(err, "could not find host %s for job %s", j.HostID, j.ID())
		}
		j.host = h
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.settings == nil {
		j.settings = j.env.Settings()
	}

	if j.CredentialsExpiration.IsZero() {
		expiration, err := j.host.JasperCredentialsExpiration(ctx, j.env)
		if err != nil {
			grip.Error(errors.Wrapf(err, "could not get credentials expiration time for host %s in job %s", j.HostID, j.ID()))
			// If we cannot get the credentials for some reason (e.g. the host's
			// credentials were deleted), assume the credentials have expired.
			j.CredentialsExpiration = time.Now()
		} else {
			j.CredentialsExpiration = expiration
		}
	}

	return nil
}

// tryRequeue attempts to requeue the job. If RestartThroughJasper is
// set, it tries to requeue with Jasper again. However, if it cannot
// do so on the next try, it instead requeues the job without using Jasper
// (i.e. SSH).
func (j *jasperRestartJob) tryRequeue(ctx context.Context) error {
	if j.RestartThroughJasper && j.canRetryRestart() && !j.credentialsExpireBefore(time.Hour) {
		return errors.Wrap(j.requeueRestartThroughJasper(ctx, true), "could not requeue job with restart through Jasper")
	}

	if j.RestartThroughJasper {
		j.CurrentAttempt = -1
	}

	if j.canRetryRestart() {
		return errors.Wrap(j.requeueRestartThroughJasper(ctx, false), "could not requeue job without restart through Jasper")
	}

	return errors.New("no more Jasper restart attempts remaining")
}

func (j *jasperRestartJob) requeueRestartThroughJasper(ctx context.Context, restartThroughJasper bool) error {
	job := NewJasperRestartJob(j.env, *j.host, j.CredentialsExpiration, restartThroughJasper, j.Timestamp, j.CurrentAttempt+1)
	job.UpdateTimeInfo(amboy.JobTimeInfo{
		WaitUntil: time.Now().Add(time.Minute),
	})

	if err := j.env.RemoteQueue().Put(ctx, job); err != nil {
		return err
	}

	return nil
}

func (j *jasperRestartJob) canRetryRestart() bool {
	return j.CurrentAttempt <= jasperRestartRetryLimit
}
