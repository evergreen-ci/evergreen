package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	buildingContainerImageJobName = "building-container-image"
	containerBuildRetries         = 8
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
		j.parent, err = host.FindOneByIdOrTag(ctx, j.ParentID)
		j.AddError(err)
		if j.parent == nil {
			j.AddError(errors.Errorf("parent '%s' not found", j.ParentID))
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

	defer func() {
		grip.Debug(message.Fields{
			"host_id":      j.parent.Id,
			"job_id":       j.ID(),
			"runner":       "taskrunner",
			"distro":       j.parent.Distro,
			"operation":    "container build complete",
			"current_iter": j.parent.ContainerBuildAttempt,
		})
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
		j.tryRequeue(ctx)
	}()

	if j.parent.ContainerBuildAttempt >= containerBuildRetries {
		err = j.parent.SetDecommissioned(ctx, evergreen.User, true, fmt.Sprintf("exceeded max container build retries (%d)", containerBuildRetries))
		j.AddError(errors.Wrapf(err, "setting parent '%s' to decommissioned", j.parent.Id))
		err = errors.Errorf("failed %d times to build and download image '%s' on parent '%s'", containerBuildRetries, j.DockerOptions.Image, j.parent.Id)
		j.AddError(err)
		grip.Warning(message.WrapError(err, message.Fields{
			"message":   "building container image job failed",
			"job_id":    j.ID(),
			"host_id":   j.parent.Id,
			"operation": "container build",
			"num_iters": j.parent.ContainerBuildAttempt,
		}))
		return
	}

	// Get cloud manager
	mgrOpts := cloud.ManagerOpts{Provider: j.Provider}
	mgr, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting Docker manager"))
		return
	}
	containerMgr, err := cloud.ConvertContainerManager(mgr)
	if err != nil {
		j.AddError(errors.Wrap(err, "converting cloud manager to Docker manager"))
		return
	}

	err = containerMgr.GetContainerImage(ctx, j.parent, j.DockerOptions)
	if err != nil {
		j.AddError(errors.Wrap(err, "building and downloading container image"))
		return
	}
	if j.parent.ContainerImages == nil {
		j.parent.ContainerImages = make(map[string]bool)
	}
	j.parent.ContainerImages[j.DockerOptions.Image] = true
	_, err = j.parent.Upsert(ctx)
	if err != nil {
		j.AddError(errors.Wrapf(err, "upserting parent '%s'", j.parent.Id))
		return
	}
}

func (j *buildingContainerImageJob) tryRequeue(ctx context.Context) {
	if j.shouldRetry(ctx) && j.env.RemoteQueue().Info().Started {
		job := NewBuildingContainerImageJob(j.env, j.parent, j.DockerOptions, j.Provider)
		job.UpdateTimeInfo(amboy.JobTimeInfo{
			WaitUntil: time.Now().Add(time.Second * 10),
		})
		err := j.env.RemoteQueue().Put(ctx, job)
		grip.Error(message.WrapError(err, message.Fields{
			"message":   "failed to requeue setup job",
			"operation": "container build",
			"host_id":   j.ParentID,
			"job":       j.ID(),
			"attempts":  j.parent.ContainerBuildAttempt,
		}))
		j.AddError(err)
	}
}

// retry if we're under retry limit, and the image isn't built yet
func (j *buildingContainerImageJob) shouldRetry(ctx context.Context) bool {
	return j.parent.ContainerBuildAttempt <= containerBuildRetries && !j.parent.ContainerImages[j.DockerOptions.Image]
}
