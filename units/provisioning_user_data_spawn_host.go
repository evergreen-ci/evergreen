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
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

const (
	userDataSpawnHostReadyJobName = "spawn-host-ready"
)

type userDataSpawnHostReadyJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"base" json:"base" yaml:"base"`

	host *host.Host
	env  evergreen.Environment
}

func init() {
	registry.AddJobType(userDataSpawnHostReadyJobName, func() amboy.Job {
		return makeUserDataSpawnHostReadyJob()
	})
}

func makeUserDataSpawnHostReadyJob() *userDataSpawnHostReadyJob {
	j := &userDataSpawnHostReadyJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    userDataSpawnHostReadyJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

// NewUserDataSpawnHostReadyJob creates a job that checks if the spawn
// host is done running its user data if bootstrapped with user data. Once the
// host is ready, the host is set to running.
func NewUserDataSpawnHostReadyJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeUserDataSpawnHostReadyJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", userDataSpawnHostReadyJobName, j.HostID, id))

	return j
}

func (j *userDataSpawnHostReadyJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(); err != nil {
		j.AddError(err)
		return
	}

	if j.host.Status != evergreen.HostProvisioning {
		grip.Info(message.Fields{
			"message": "skipping user data spawn host check because host is no longer provisioning",
			"job":     j.ID(),
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
		})
		return
	}

	if j.host.Distro.UserDataDonePath == "" {
		err := errors.New("distro must have path to user data done file")
		grip.Error(message.WrapError(err, message.Fields{
			"message": "cannot check user data done file without a distro setting for its path",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	if output, err := j.host.RunJasperProcess(ctx, j.env, &jasper.CreateOptions{Args: []string{"ls", j.host.Distro.UserDataDonePath}}); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "host was checked but is not yet ready",
			"output":  output,
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	if err := j.host.UpdateProvisioningToRunning(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not mark host as done provisioning itself and now running",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	grip.Info(message.Fields{
		"message":                 "host successfully provisioned",
		"host":                    j.host.Id,
		"distro":                  j.host.Distro.Id,
		"job":                     j.ID(),
		"provision_duration_secs": time.Since(j.host.CreationTime).Seconds(),
	})
}

func (j *userDataSpawnHostReadyJob) populateIfUnset() error {
	if j.host == nil {
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			return errors.Wrapf(err, "could not find host %s for job %s", j.HostID, j.ID())
		}
		if h == nil {
			return errors.Errorf("could not find host %s for job %s", j.HostID, j.ID())
		}
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	return nil
}
