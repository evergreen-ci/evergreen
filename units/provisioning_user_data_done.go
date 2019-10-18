package units

import (
	"context"
	"fmt"
	"path/filepath"

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
func NewUserDataDoneJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeUserDataDoneJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", userDataDoneJobName, j.HostID, id))

	return j
}

func (j *userDataDoneJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(); err != nil {
		j.AddError(err)
		return
	}

	if j.host.Status != evergreen.HostProvisioning {
		return
	}

	path, err := j.host.UserDataDoneFilePath()
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting user data done file path",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	if output, err := j.host.RunJasperProcess(ctx, j.env, &options.Create{
		Args: []string{
			filepath.Join(j.host.Distro.BootstrapSettings.RootDir, j.host.Distro.BootstrapSettings.ShellPath),
			"-l", "-c",
			fmt.Sprintf("ls %s", path)}}); err != nil {
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

	if err := j.host.SetUserDataHostProvisioned(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not mark host that has finished running user data as done provisioning",
			"host":    j.host.Id,
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
