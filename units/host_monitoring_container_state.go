package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const hostMonitorContainerStateJobName = "host-monitoring-container-state"

func init() {
	registry.AddJobType(hostMonitorContainerStateJobName, func() amboy.Job {
		return makeHostMonitorContainerStateJob()
	})
}

type hostMonitorContainerStateJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"base" json:"base" yaml:"base"`

	// cache
	host     *host.Host
	env      evergreen.Environment
	provider string
	settings *evergreen.Settings
}

func makeHostMonitorContainerStateJob() *hostMonitorContainerStateJob {
	j := &hostMonitorContainerStateJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostMonitorContainerStateJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostMonitorContainerStateJob(env evergreen.Environment, h *host.Host, providerName, id string) amboy.Job {
	job := makeHostMonitorContainerStateJob()

	job.host = h
	job.provider = providerName
	job.HostID = h.Id

	job.SetID(fmt.Sprintf("%s.%s.%s", hostMonitorContainerStateJobName, job.HostID, id))

	return job
}

func (j *hostMonitorContainerStateJob) Run(ctx context.Context) {
	var cancel context.CancelFunc

	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	defer j.MarkComplete()

	var err error
	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		j.AddError(err)
	}
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.settings == nil {
		j.settings = j.env.Settings()
	}

	if j.HasErrors() {
		return
	}

	// get containers on parent
	containersFromDB, err := j.host.GetContainers()
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting containers on parent %s from DB", j.HostID))
		return
	}

	// list containers using Docker provider
	m, err := cloud.GetContainerManager(ctx, j.provider, j.settings)
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting Docker manager"))
		return
	}
	containersIdsFromDocker, err := m.GetContainers(ctx, j.host)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting containers on parent %s from Docker", j.HostID))
		return
	}

	// for each non-terminated container in containersDB that is not running in
	// containersDB, mark it as terminated
	var found bool
	for _, container := range containersFromDB {
		found = false
		for _, id := range containersIdsFromDocker {
			if container.Id == id {
				found = true
				break
			}
		}
		if !found {
			j.AddError(container.SetTerminated(evergreen.User))
		}
	}
}
