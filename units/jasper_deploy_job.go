package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	jasperDeployJobName    = "jasper-deploy"
	expirationCutoff       = 7 * 24 * time.Hour // 1 week
	jasperDeployRetryLimit = 75
)

func init() {
	registry.AddJobType(jasperDeployJobName, func() amboy.Job { return makeJasperDeployJob() })
}

type jasperDeployJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID   string

	env                   evergreen.Environment
	host                  *host.Host
	credentialsExpiration time.Time
}

func makeJasperDeployJob() *jasperDeployJob {
	j := &jasperDeployJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    jasperDeployJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewJasperDeployJob creates a job that deploys a new Jasper service to a
// host currently running a Jasper service.
func NewJasperDeployJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeJasperDeployJob()
	j.env = env
	j.host = &h
	j.SetID(fmt.Sprintf("%s.%s.%s", jasperDeployJobName, j.HostID, id))
	return j
}

// Run deploys new Jasper credentials to a host.
func (j *jasperDeployJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	// If error, deploy has failed and still need to deploy new Jasper credentials.
	defer func() {
		if j.HasErrors() {
			if j.tryRequeue(ctx) {
				// kim: TODO: retry deploy case. Just put on a new job.
			} else if j.credentialsExpireAfter(24 * time.Hour) {
				// TODO: make host fall back on using SSH instead of RPC.
				grip.Warning(message.Fields{
					"message":  "Jasper credentials are expiring, but no more Jasper deploy attempts remaining",
					"host":     j.host.Id,
					"distro":   j.host.Distro.Id,
					"expires":  j.credentialsExpiration,
					"attempts": j.host.JasperDeployAttempts,
					"job":      j.ID(),
				})
				// kim: TODO: catastrophe case, Jasper credentials will expire
				// and we cannot deploy anymore.
			}
			// Re-queue the job if it failed and there are more attempts
			// remaining.
		}
	}()

	// kim; TODO: choose method of communication.

	if err := j.populateIfUnset(); err != nil {
		j.AddError(err)
		return
	}

	if j.credentialsExpireAfter(0) {
		j.AddError(errors.New)
	}

	// Atomically deploy Jasper credentials without any other job getting to
	// this.
	// Note: since this job runs infrequently (once a day) and only queues a
	// retry if it errors, there's no need to make this atomic.

	// Update LCT to prevent other jobs from trying to terminate this host,
	// which is about to kill the agent.
	if err := j.host.UpdateLastCommunicated(); err != nil {
		j.AddError(errors.Wrapf(err, "error setting LCT on host %s".j.host.Id))
	}

	grip.Info(message.Fields{
		"message":   "deploy new Jasper credentials",
		"operation": "redeploy Jasper to host",
		"host":      j.host.Id,
		"distro":    j.host.Distro.Id,
		"job":       j.ID(),
	})

	// Disable poisoned host if we fail to deploy new credentials. I have no
	// idea what to do otherwise if new credentials can't be put on the static
	// hosts.

	// Kill the current Jasper service in a platform-dependent way. It should
	// restart, but disable the poisoned host if we can't kill it.

	// Set NeedsNewAgentMonitor to true to make the agent monitor deploy job
	// run. Or just run it right now.

	settings := j.env.Settings()

	// Check if the host was running a task and reset if necessary.
	if j.host.RunningTask != "" {
		if err := model.ClearAndResetStrandedTask(j.host); err != nil {
			j.AddError(err)
		}
	}

	event.LogHostJasperDeployed(j.host.Id, settings.HostJasper.Version)

	// Note: can't use pkill/pgrep because it doesn't return itself as a
	// process.
	// Call bash -c 'echo $PPID | xargs kill -9', or something equivalent.
	// Unfortunately, no easy way of checking that it succeeded except by doing
	// some kind of ping to the new RPC service.

	// Kill Jasper command:
	// Windows: "bash -c 'curator jasper service force-reinstall rpc'"
	// MacOS: bash -c "pgrep -f 'curator jasper service | xargs kill'
	// Linux: bash -c "echo $PPID | xargs kill" (maybe can just use same as
	// MacOS)
}

func (j *jasperDeployJob) putNewCredentials(ctx context.Context) error {
	return nil
}

func (j *jasperDeployJob) tryRequeue(ctx context.Context) error {
	if j.shouldRetryDeploy(ctx) {
		// TODO: replace attempts with new Jasper deploy attempts count
		job := NewJasperDeployJob(j.env, *j.host, fmt.Sprintf("attempt-%d", j.host.ProvisionAttempts))
		job.UpdateTimeInfo(amboy.JobTimeInfo{
			WaitUntil: time.Now().Add(time.Minute),
		})
		// TODO: if

		if err := j.env.RemoteQueue().Put(ctx, job); err != nil {
			grip.Critical(message.WrapError(err, message.Fields{
				"message": "failed to requeue Jasper redeploy job",
				"host":    j.host.Id,
				"job":     j.ID(),
				"distro":  j.host.Distro.Id,
				// TODO: replace attempts with new Jasper deploy attempts count
				"attempts": j.host.ProvisionAttempts,
			}))
			return err
		}

		return nil
	}
	return nil
}

// TODO: replace with a different way to check attempts. Maybe add a "Jasper
// deploy attempts count" of some kind.
func (j *jasperDeployJob) shouldRetryDeploy(ctx context.Context) bool {
	return j.host.JasperDeployAttempts <= jasperDeployRetryLimit && j.credentialsExpireAfter(0)
}

// credentialsExpireAfter returns whether or not the host's Jasper credentials
// expire after the given cutoff.
func (j *jasperDeployJob) credentialsExpireAfter(cutoff time.Duration) bool {
	return time.Now().Add(cutoff).Before(j.credentialsExpiration)
}

// populateIfUnset populates the unset job fields.
func (j *jasperDeployJob) populateIfUnset(ctx context.Context) error {
	if j.host == nil {
		host, err := host.FindOneId(j.HostID)
		if err != nil || host == nil {
			return errors.Wrapf(err, "could not find host %s for job %s", j.HostID, j.TaskID)
		}
		j.host = host
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.credentialsExpiration.IsZero() {
		expiration, err := j.host.JasperCredentialsExpiration(ctx, j.env)
		if err != nil {
			return errors.Wrap(err, "could not get credentials expiration time for host %s in job %s", j.HostID, j.TaskID)
		}
		j.credentialsExpiration = expiration
	}

	return nil
}
