package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
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

// NewUserDataDoneJob creates a job that checks if the host is done running its
// user data if bootstrapped with user data.
func NewUserDataDoneJob(env evergreen.Environment, hostID string, ts time.Time) amboy.Job {
	j := makeUserDataDoneJob()
	j.HostID = hostID
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", userDataDoneJobName, j.HostID, ts.Format(TSFormat)))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", userDataDoneJobName, hostID)})
	return j
}

func (j *userDataDoneJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(); err != nil {
		j.AddError(err)
		return
	}

	if j.host.Status != evergreen.HostStarting {
		return
	}

	path := j.host.UserDataDoneFile()

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
		j.AddError(err)
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
			j.AddError(j.env.RemoteQueue().Put(ctx, terminateJob))

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
		// Don't wait on setup script to finish, particularly for hosts waiting on task data.
		j.AddError(j.env.RemoteQueue().Put(ctx, NewHostSetupScriptJob(j.env, j.host, 0)))
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
		j.AddError(err)
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
