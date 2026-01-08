package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
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
	Provider string `bson:"provider" json:"provider" yaml:"provider"`

	// cache
	host     *host.Host
	env      evergreen.Environment
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
	return j
}

func NewHostMonitorContainerStateJob(h *host.Host, providerName, id string) amboy.Job {
	job := makeHostMonitorContainerStateJob()

	job.host = h
	job.Provider = providerName
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
		j.host, err = host.FindOneId(ctx, j.HostID)
		j.AddError(err)
		if j.host == nil {
			j.AddError(errors.Errorf("host '%s' not found", j.HostID))
		}
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
	containersFromDB, err := j.host.GetContainers(ctx)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding containers on parent host '%s'", j.HostID))
		return
	}

	// list containers using Docker provider
	mgr, err := cloud.GetManager(ctx, j.env, cloud.ManagerOpts{Provider: j.Provider})
	if err != nil {
		j.AddError(errors.Wrap(err, "getting Docker manager"))
		return
	}
	containerMgr, err := cloud.ConvertContainerManager(mgr)
	if err != nil {
		j.AddError(errors.Wrap(err, "converting manager to container manager"))
		return
	}
	containerIdsFromDocker, err := containerMgr.GetContainers(ctx, j.host)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting containers on parent '%s' from Docker", j.HostID))
		return
	}

	// build map of running container IDs
	isRunningInDocker := make(map[string]bool)
	for _, id := range containerIdsFromDocker {
		isRunningInDocker[id] = true
	}

	// for each non-terminated container in DB that is not actually running on
	// Docker, remove container from Docker and mark it as terminated
	for _, container := range containersFromDB {
		if container.Status == evergreen.HostRunning || container.Status == evergreen.HostDecommissioned {
			if !container.SpawnOptions.SpawnedByTask && !isRunningInDocker[container.Id] {
				if err := containerMgr.TerminateInstance(ctx, &container, evergreen.User, "container is not actually running"); err != nil {
					j.AddError(errors.Wrap(err, "terminating Docker instance"))
					j.AddError(container.Terminate(ctx, evergreen.User, "container is not actually running"))
				}
			}
		}
	}
}
