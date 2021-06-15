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
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

const (
	userDataDoneJobName = "user-data-done"
)

type userDataDoneJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"base" json:"base" yaml:"base"`

	env      evergreen.Environment
	settings *evergreen.Settings
	host     *host.Host
}

func init() {
	registry.AddJobType(userDataDoneJobName, func() amboy.Job {
		return makeUserDataDoneJob()
	})
}

func makeUserDataDoneJob() *userDataDoneJob {
	j := &userDataDoneJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    userDataDoneJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

// NewUserDataDoneJob creates a job that checks if the host is done provisioning
// with user data (if bootstrapped with user data). This check only applies to
// spawn hosts, since hosts running agents check into the server to verify their
// liveliness.
func NewUserDataDoneJob(env evergreen.Environment, hostID string, ts time.Time) amboy.Job {
	j := makeUserDataDoneJob()
	j.HostID = hostID
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", userDataDoneJobName, j.HostID, ts.Format(TSFormat)))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", userDataDoneJobName, hostID)})
	j.SetShouldApplyScopesOnEnqueue(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(50),
		WaitUntil:   utility.ToTimeDurationPtr(20 * time.Second),
	})
	return j
}

func (j *userDataDoneJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	defer func() {
		if j.HasErrors() && (!j.RetryInfo().ShouldRetry() || j.RetryInfo().GetRemainingAttempts() == 0) {
			event.LogHostProvisionFailed(j.HostID, j.Error().Error())
		}
	}()

	if err := j.populateIfUnset(); err != nil {
		j.AddRetryableError(err)
		return
	}

	if j.host.Status != evergreen.HostStarting {
		j.UpdateRetryInfo(amboy.JobRetryOptions{
			NeedsRetry: utility.TruePtr(),
		})
		return
	}

	path := j.host.UserDataProvisioningDoneFile()

	if output, err := j.host.RunJasperProcess(ctx, j.env, &options.Create{
		Args: []string{
			j.host.Distro.ShellBinary(),
			"-l", "-c",
			fmt.Sprintf("ls %s", path)}}); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "host was checked but is not yet ready",
			"output":  output,
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddRetryableError(err)
		return
	}

	if j.host.IsVirtualWorkstation {
		if err := attachVolume(ctx, j.env, j.host); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "can't attach volume",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			j.AddError(j.host.SetStatus(evergreen.HostProvisionFailed, evergreen.User,
				"decommissioning host after failing to mount volume"))

			terminateJob := NewHostTerminationJob(j.env, j.host, true, "failed to mount volume")
			terminateJob.SetPriority(100)
			j.AddError(amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), terminateJob))

			return
		}
		if err := writeIcecreamConfig(ctx, j.env, j.host); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "can't write icecream config file",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
		}
	}

	if j.host.ProvisionOptions != nil && j.host.ProvisionOptions.SetupScript != "" {
		// Run the spawn host setup script in a separate job to avoid forcing
		// this job to wait for task data to be loaded.
		j.AddError(j.env.RemoteQueue().Put(ctx, NewHostSetupScriptJob(j.env, j.host)))
	}

	j.finishJob()
}

func (j *userDataDoneJob) finishJob() {
	if err := j.host.SetUserDataHostProvisioned(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not mark host that has finished running user data as done provisioning",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddRetryableError(err)
		return
	}
}

func (j *userDataDoneJob) populateIfUnset() error {
	if j.host == nil {
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			return errors.Wrapf(err, "could not find host %s for job %s", j.HostID, j.ID())
		}
		if h == nil {
			return errors.Errorf("could not find host %s for job %s", j.HostID, j.ID())
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
