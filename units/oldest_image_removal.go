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

const (
	oldestImageRemovalJobName = "oldest-image-removal"
	// if each image is around 11GB, allow 10 images at a time
	maxDiskUsage = 1024 * 1024 * 1024 * 11 * 10
)

func init() {
	registry.AddJobType(oldestImageRemovalJobName, func() amboy.Job {
		return makeOldestImageRemovalJob()
	})

}

type oldestImageRemovalJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"base" json:"base" yaml:"base"`
	Provider string `bson:"provider" json:"provider" yaml:"provider"`

	// cache
	host     *host.Host
	env      evergreen.Environment
	settings *evergreen.Settings
}

func makeOldestImageRemovalJob() *oldestImageRemovalJob {
	j := &oldestImageRemovalJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    oldestImageRemovalJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func NewOldestImageRemovalJob(h *host.Host, providerName, id string) amboy.Job {
	j := makeOldestImageRemovalJob()

	j.host = h
	j.Provider = providerName
	j.HostID = h.Id

	j.SetID(fmt.Sprintf("%s.%s.%s", oldestImageRemovalJobName, h.Id, id))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", oldestImageRemovalJobName, h.Id)})
	j.SetShouldApplyScopesOnEnqueue(true)
	return j
}

func (j *oldestImageRemovalJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	var err error
	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		j.AddError(err)
		if j.host == nil {
			j.AddError(errors.Errorf("unable to retrieve host %s", j.HostID))
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

	// get least recently used image from Docker provider
	mgr, err := cloud.GetManager(ctx, j.env, cloud.ManagerOpts{Provider: j.Provider})
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting Docker manager"))
		return
	}
	containerMgr, err := cloud.ConvertContainerManager(mgr)
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting Docker manager"))
		return
	}

	diskUsage, err := containerMgr.CalculateImageSpaceUsage(ctx, j.host)
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting Docker disk usage"))
	}

	if diskUsage >= maxDiskUsage {
		err = containerMgr.RemoveOldestImage(ctx, j.host)
		if err != nil {
			j.AddError(errors.Wrapf(err, "error removing least recently used image ID on parent %s from Docker", j.HostID))
			return
		}
	}

}
