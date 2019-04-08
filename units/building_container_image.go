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
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	buildingContainerImageJobName = "building-container-image"
	containerBuildRetries         = 5
)

func init() {
	registry.AddJobType(buildingContainerImageJobName, func() amboy.Job {
		return makeBuildingContainerImageJob()
	})
}

type buildingContainerImageJob struct {
	job.Base `bson:"base"`

	ParentID      string             `bson:"parent_id"`
	DockerOptions host.DockerOptions `bson:"docker_options"`
	Provider      string             `bson:"provider"`

	// cache
	parent   *host.Host
	env      evergreen.Environment
	settings *evergreen.Settings
}

func makeBuildingContainerImageJob() *buildingContainerImageJob {
	j := &buildingContainerImageJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    buildingContainerImageJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewBuildingContainerImageJob(env evergreen.Environment, h *host.Host, dockerOptions host.DockerOptions, providerName string) amboy.Job {
	job := makeBuildingContainerImageJob()

	job.env = env
	job.parent = h
	job.DockerOptions = dockerOptions
	job.ParentID = h.Id
	job.Provider = providerName

	job.SetID(fmt.Sprintf("%s.%s.attempt-%d.%s", buildingContainerImageJobName, job.ParentID, h.ContainerBuildAttempt, job.DockerOptions.Image))

	return job
}

func (j *buildingContainerImageJob) Run(ctx context.Context) {
	var cancel context.CancelFunc

	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	defer j.MarkComplete()

	var err error
	if j.parent == nil {
		j.parent, err = host.FindOneId(j.ParentID)
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

	defer func() {
		if err = j.parent.IncContainerBuildAttempt(); err != nil {
			j.AddError(err)
			grip.Warning(message.WrapError(err, message.Fields{
				"host_id":      j.parent.Id,
				"job_id":       j.ID(),
				"runner":       "taskrunner",
				"distro":       j.parent.Distro,
				"message":      "failed to update container build iteration",
				"current_iter": j.parent.ContainerBuildAttempt,
			}))
			return
		}
		grip.Debug(message.Fields{
			"host_id":      j.parent.Id,
			"job_id":       j.ID(),
			"runner":       "taskrunner",
			"distro":       j.parent.Distro,
			"operation":    "container build complete",
			"current_iter": j.parent.ContainerBuildAttempt,
		})
	}()

	if j.parent.ContainerBuildAttempt >= containerBuildRetries {
		j.AddError(errors.Wrapf(j.parent.SetTerminated(evergreen.User),
			"failed 5 times to build and download image '%s' on parent '%s'", j.DockerOptions.Image, j.parent.Id))
		return
	}

	// Get cloud manager
	mgr, err := cloud.GetManager(ctx, j.Provider, j.settings)
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting Docker manager"))
		return
	}
	containerMgr, err := cloud.ConvertContainerManager(mgr)
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting Docker manager"))
		return
	}

	err = containerMgr.GetContainerImage(ctx, j.parent, j.DockerOptions)
	if err != nil {
		j.AddError(errors.Wrap(err, "error building and downloading container image"))
		return
	}
	if j.parent.ContainerImages == nil {
		j.parent.ContainerImages = make(map[string]bool)
	}
	j.parent.ContainerImages[j.DockerOptions.Image] = true
	_, err = j.parent.Upsert()
	if err != nil {
		j.AddError(errors.Wrapf(err, "error upserting parent %s", j.parent.Id))
		return
	}
}
