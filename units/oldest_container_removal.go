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
	oldestImageJobName = "oldest-image"
	// if each image is around 11GB, allow 10 images at a time
	maxDiskUsage = 1024 * 1024 * 1024 * 11 * 10
)

func init() {
	registry.AddJobType(oldestImageJobName, func() amboy.Job {
		return makeOldestImageTimeJob()
	})

}

type oldestImageJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"base" json:"base" yaml:"base"`

	// cache
	host     *host.Host
	env      evergreen.Environment
	provider string
	settings *evergreen.Settings
}

func makeOldestImageTimeJob() *oldestImageJob {
	j := &oldestImageJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    oldestImageJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func NewOldestImageJob(h *host.Host, providerName, id string) amboy.Job {
	j := makeOldestImageTimeJob()

	j.host = h
	j.provider = providerName
	j.HostID = h.Id

	j.SetID(fmt.Sprintf("%s.%s", oldestImageJobName, id))
	return j
}

func (j *oldestImageJob) Run(ctx context.Context) {
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

	// get least recently used image from Docker provider
	mgr, err := cloud.GetManager(ctx, j.provider, j.settings)
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
		err = containerMgr.RemoveOldestImageID(ctx, j.host)
		if err != nil {
			j.AddError(errors.Wrapf(err, "error removing least recently used image ID on parent %s from Docker", j.HostID))
			return
		}
	}

}
