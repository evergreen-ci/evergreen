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

const (
	taskHostExpirationExtendName = "task-host-expiration-extend"
)

func init() {
	registry.AddJobType(taskHostExpirationExtendName,
		func() amboy.Job { return makeTaskHostExpirationExtendJob() })
}

type taskHostExpirationExtendJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`

	host *host.Host
	env  evergreen.Environment
}

func makeTaskHostExpirationExtendJob() *taskHostExpirationExtendJob {
	j := &taskHostExpirationExtendJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskHostExpirationExtendName,
				Version: 0,
			},
		},
	}
	return j
}

func NewTaskHostExpirationExtendJob(ts string, h *host.Host) amboy.Job {
	j := makeTaskHostExpirationExtendJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", taskHostExpirationExtendName, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", taskHostExpirationExtendName, h.Id)})
	j.SetEnqueueAllScopes(true)
	j.HostID = h.Id
	return j
}

func (j *taskHostExpirationExtendJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	var err error

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.host == nil {
		j.host, err = host.FindOneId(ctx, j.HostID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding host '%s'", j.HostID))
			return
		}
		if j.host == nil {
			j.AddError(errors.Errorf("host '%s' not found", j.HostID))
			return
		}
	}

	mgrOpts, err := cloud.GetManagerOptions(j.host.Distro)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting cloud manager options for host '%s'", j.host.Id))
		return
	}
	cloudManager, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting cloud manager for host '%s'", j.HostID))
		return
	}
	if err := cloudManager.ModifyHost(ctx, j.host, host.HostModifyOptions{ExtendExpireOnByDay: true}); err != nil {
		j.AddError(errors.Wrapf(err, "extending expire-on tag for host '%s'", j.HostID))
		return
	}
}
