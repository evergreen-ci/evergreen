package units

import (
	"context"
	"fmt"
	"time"

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
	createHostJobName = "provisioning-create-host"
	maxPollAttempts   = 100
)

func init() {
	registry.AddJobType(createHostJobName, func() amboy.Job {
		return makeCreateHostJob()
	})
}

type createHostJob struct {
	HostID         string `bson:"host_id" json:"host_id" yaml:"host_id"`
	CurrentAttempt int    `bson:"current_attempt" json:"current_attempt" yaml:"current_attempt"`
	MaxAttempts    int    `bson:"max_attempts" json:"max_attempts" yaml:"max_attempts"`
	job.Base       `bson:"metadata" json:"metadata" yaml:"metadata"`

	start time.Time
	host  *host.Host
	env   evergreen.Environment
}

func makeCreateHostJob() *createHostJob {
	j := &createHostJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    createHostJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostCreateJob(env evergreen.Environment, h host.Host, id string, CurrentAttempt int, MaxAttempts int) amboy.Job {
	j := makeCreateHostJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", createHostJobName, j.HostID, id))
	j.CurrentAttempt = CurrentAttempt
	if MaxAttempts > 0 {
		j.MaxAttempts = MaxAttempts
	} else if j.host.SpawnOptions.Retries > 0 {
		j.MaxAttempts = j.host.SpawnOptions.Retries
	} else {
		j.MaxAttempts = 1
	}
	return j
}

func (j *createHostJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	j.start = time.Now()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(err)
	}

	if flags.HostInitDisabled {
		grip.Debug(message.Fields{
			"mode":     "degraded",
			"host":     j.HostID,
			"job":      j.ID(),
			"job_type": j.Type().Name,
		})
		return
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.TaskID))
			return
		}
	}

	if j.host.Status != evergreen.HostUninitialized && j.host.Status != evergreen.HostBuilding {
		grip.Notice(message.Fields{
			"message": "host has already been started",
			"status":  j.host.Status,
			"host_id": j.host.Id,
		})
		return
	}

	if j.host.ParentID == "" && !j.host.SpawnOptions.SpawnedByTask {
		numHosts, err := host.CountRunningHosts(j.host.Distro.Id)
		if err != nil {
			j.AddError(errors.Wrap(err, "problem getting count of existing pool size"))
			return
		}

		if numHosts > j.host.Distro.PoolSize {
			grip.Info(message.Fields{
				"host_id":   j.HostID,
				"attempt":   j.CurrentAttempt,
				"distro":    j.host.Distro.Id,
				"job":       j.ID(),
				"provider":  j.host.Provider,
				"message":   "not provisioning host to respect maxhosts",
				"max_hosts": j.host.Distro.PoolSize,
			})

			err = errors.Wrap(j.host.Remove(), "problem removing host intent")

			j.AddError(err)
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":  j.HostID,
				"attempt":  j.CurrentAttempt,
				"distro":   j.host.Distro,
				"job":      j.ID(),
				"provider": j.host.Provider,
				"message":  "could not remove intent document",
				"outcome":  "host pool may exceed maxhost limit",
			}))

			return
		}
	}

	if j.TimeInfo().MaxTime == 0 && !j.host.SpawnOptions.TimeoutSetup.IsZero() {
		j.UpdateTimeInfo(amboy.JobTimeInfo{
			MaxTime: j.host.SpawnOptions.TimeoutSetup.Sub(j.start),
		})
	}
	err = j.createHost(ctx)
	j.AddError(err)
	if err != nil {
		grip.Info(message.Fields{
			"host_id":  j.HostID,
			"attempt":  j.CurrentAttempt,
			"distro":   j.host.Distro,
			"error":    err.Error(),
			"job":      j.ID(),
			"provider": j.host.Provider,
			"message":  "error provisioning host",
		})
	}
}

func (j *createHostJob) createHost(ctx context.Context) error {
	var cloudManager cloud.Manager
	var err error
	if err = ctx.Err(); err != nil {
		return errors.Wrap(err, "canceling create host because context is canceled")
	}
	hostStartTime := j.start
	grip.Info(message.Fields{
		"message":      "attempting to start host",
		"hostid":       j.host.Id,
		"job":          j.ID(),
		"attempt":      j.CurrentAttempt,
		"max_attempts": j.MaxAttempts,
	})

	cloudManager, err = cloud.GetManager(ctx, j.host.Provider, j.env.Settings())
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "problem getting cloud provider for host",
			"host":    j.host.Id,
			"job":     j.ID(),
		}))
		return errors.Wrapf(errIgnorableCreateHost, "problem getting cloud provider for host '%s' [%s]", j.host.Id, err.Error())
	}

	defer j.tryRequeue(ctx)

	// Set status temporarily to HostBuilding. Conventional hosts only stay in
	// SpawnHost for a short period of time. Containers stay in SpawnHost for
	// longer, since they may need to download container images and build them
	// with the agent. This state allows intent documents to stay around until
	// SpawnHost returns, but NOT as as initializing hosts that could still be
	// spawned by Evergreen.
	if err = j.host.SetStatus(evergreen.HostBuilding, evergreen.User, ""); err != nil {
		return errors.Wrapf(err, "problem setting host %s status to building", j.host.Id)
	}

	// Containers should wait on image builds, checking to see if the parent
	// already has the image. If it does not, it should download it and wait
	// on the job until it is finished downloading.
	if j.host.ParentID != "" {
		var ready bool
		j.MaxAttempts = maxPollAttempts
		ready, err = j.isImageBuilt(ctx)
		if err != nil {
			return errors.Wrap(err, "problem building container image")
		}
		if !ready {
			return nil
		}
	}
	grip.Info(message.Fields{
		"message": "image is ready",
		"job":     j.ID(),
		"host":    j.HostID,
		"image":   j.host.DockerOptions.Image,
	})
	if _, err = cloudManager.SpawnHost(ctx, j.host); err != nil {
		return errors.Wrapf(err, "error spawning host %s", j.host.Id)
	}

	// On the first attempt, remove the intent host to insert started host
	if j.CurrentAttempt == 1 {
		intentHost, err := host.FindOneId(j.HostID)
		if err != nil {
			return errors.Wrapf(err, "problem retrieving intent host '%s'", j.HostID)
		}
		if intentHost == nil {
			return errors.Wrapf(err, "no intent host '%s' found", j.HostID)
		}
		if err := intentHost.Remove(); err != nil {
			grip.Notice(message.WrapError(err, message.Fields{
				"message": "problem removing intent host",
				"job":     j.ID(),
				"host":    j.HostID,
			}))
			return errors.Wrapf(errIgnorableCreateHost, "problem removing intent host '%s' [%s]", j.HostID, err.Error())
		}
	}

	// Don't mark containers as starting. SpawnHost already marks containers as
	// running.
	if j.host.ParentID == "" {
		j.host.Status = evergreen.HostStarting
	}

	// Provisionally set j.host.StartTime to now. Cloud providers may override
	// this value with the time the host was created.
	j.host.StartTime = j.start

	if err := j.host.Insert(); err != nil {
		return errors.Wrapf(err, "error updating host %v", j.host.Id)
	}

	grip.Info(message.Fields{
		"message": "successfully started host",
		"hostid":  j.host.Id,
		"job":     j.ID(),
		"DNS":     j.host.Host,
		"runtime": time.Since(hostStartTime),
	})

	return nil
}

func (j *createHostJob) tryRequeue(ctx context.Context) {
	if j.shouldRetryCreateHost(ctx) && j.env.RemoteQueue().Started() {
		job := NewHostCreateJob(j.env, *j.host, fmt.Sprintf("attempt-%d", j.CurrentAttempt+1), j.CurrentAttempt+1, j.MaxAttempts)
		wait := time.Minute
		if j.host.ParentID != "" {
			wait = 10 * time.Second
		}
		job.UpdateTimeInfo(amboy.JobTimeInfo{
			WaitUntil: j.start.Add(wait),
			MaxTime:   j.TimeInfo().MaxTime - (time.Since(j.start)) - time.Minute,
		})
		err := j.env.RemoteQueue().Put(job)
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "failed to requeue setup job",
			"host":     j.host.Id,
			"job":      j.ID(),
			"distro":   j.host.Distro.Id,
			"attempts": j.host.ProvisionAttempts,
		}))
		j.AddError(err)
	}
}

func (j *createHostJob) shouldRetryCreateHost(ctx context.Context) bool {
	return j.CurrentAttempt < j.MaxAttempts &&
		(j.host.Status == evergreen.HostUninitialized || j.host.Status == evergreen.HostBuilding) &&
		ctx.Err() == nil
}

func (j *createHostJob) isImageBuilt(ctx context.Context) (bool, error) {
	parent, err := j.host.GetParent()
	if err != nil {
		return false, errors.Wrapf(err, "problem getting parent for '%s'", j.host.Id)
	}
	if parent == nil {
		return false, errors.Wrapf(err, "parent for '%s' does not exist", j.host.Id)
	}

	if parent.Status == evergreen.HostUninitialized || parent.Status == evergreen.HostBuilding {
		return false, errors.Errorf("parent for host '%s' not running", j.host.Id)
	}
	if ok := parent.ContainerImages[j.host.DockerOptions.Image]; ok {
		return true, nil
	}

	//  If the image is not already present on the parent, run job to build the new image
	if j.CurrentAttempt == 1 {
		buildingContainerJob := NewBuildingContainerImageJob(j.env, parent, j.host.DockerOptions, j.host.Provider)
		err = j.env.RemoteQueue().Put(buildingContainerJob)
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "Duplicate key being added to job to block building containers",
		}))
	}
	return false, nil
}
