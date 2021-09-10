package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
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
	HostID            string `bson:"host_id" json:"host_id" yaml:"host_id"`
	BuildImageStarted bool   `bson:"build_image_started" json:"build_image_started" yaml:"build_image_started"`
	job.Base          `bson:"metadata" json:"metadata" yaml:"metadata"`

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

func NewHostCreateJob(env evergreen.Environment, h host.Host, id string, currentAttempt int, buildImageStarted bool) amboy.Job {
	j := makeCreateHostJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", createHostJobName, h.Id, id))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", createHostJobName, h.Id)})
	j.SetShouldApplyScopesOnEnqueue(true)
	j.BuildImageStarted = buildImageStarted
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		DispatchBy: j.host.SpawnOptions.TimeoutSetup,
	})
	var wait time.Duration
	var maxAttempts int
	if h.ParentID != "" {
		wait = time.Minute
		maxAttempts = maxPollAttempts
	} else {
		wait = 10 * time.Second
		maxAttempts = j.host.SpawnOptions.Retries
	}
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(maxAttempts),
		WaitUntil:   utility.ToTimeDurationPtr(wait),
	})
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
			"host_id":  j.HostID,
			"job":      j.ID(),
			"job_type": j.Type().Name,
		})
		return
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	j.AddError(errors.Wrap(j.env.Settings().HostInit.Get(j.env), "problem refreshing hostinit settings"))

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			//host intent document has been removed by another evergreen process
			grip.Warning(message.Fields{
				"host_id": j.HostID,
				"task_id": j.TaskID,
				"attempt": j.RetryInfo().CurrentAttempt,
				"job":     j.ID(),
				"message": "could not find host",
			})
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

	if j.host.IsSubjectToHostCreationThrottle() {
		var numHosts int
		numHosts, err = host.CountRunningHosts(j.host.Distro.Id)
		if err != nil {
			j.AddError(errors.Wrap(err, "problem getting count of existing pool size"))
			return
		}

		removeHostIntent := false
		if numHosts > j.host.Distro.HostAllocatorSettings.MaximumHosts {
			grip.Info(message.Fields{
				"host_id":   j.HostID,
				"attempt":   j.RetryInfo().CurrentAttempt,
				"distro":    j.host.Distro.Id,
				"job":       j.ID(),
				"provider":  j.host.Provider,
				"message":   "not provisioning host to respect maxhosts",
				"max_hosts": j.host.Distro.HostAllocatorSettings.MaximumHosts,
			})
			removeHostIntent = true

		}

		allRunningDynamicHosts, err := host.CountAllRunningDynamicHosts()
		j.AddError(err)
		lowHostNumException := false
		if numHosts < 10 {
			lowHostNumException = true
		}

		if allRunningDynamicHosts > j.env.Settings().HostInit.MaxTotalDynamicHosts && !lowHostNumException {

			grip.Info(message.Fields{
				"host_id":                 j.HostID,
				"attempt":                 j.RetryInfo().CurrentAttempt,
				"distro":                  j.host.Distro.Id,
				"job":                     j.ID(),
				"provider":                j.host.Provider,
				"message":                 "not provisioning host to respect max_total_dynamic_hosts",
				"total_dynamic_hosts":     allRunningDynamicHosts,
				"max_total_dynamic_hosts": j.env.Settings().HostInit.MaxTotalDynamicHosts,
			})
			removeHostIntent = true

		}

		if removeHostIntent {
			err = errors.Wrap(j.host.Remove(), "problem removing host intent")

			j.AddError(err)
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":  j.HostID,
				"attempt":  j.RetryInfo().CurrentAttempt,
				"distro":   j.host.Distro.Id,
				"job":      j.ID(),
				"provider": j.host.Provider,
				"message":  "could not remove intent document",
				"outcome":  "host pool may exceed maxhost limit",
			}))

			return
		}

		if j.selfThrottle() {
			grip.Debug(message.Fields{
				"host_id":  j.HostID,
				"attempt":  j.RetryInfo().CurrentAttempt,
				"distro":   j.host.Distro.Id,
				"job":      j.ID(),
				"provider": j.host.Provider,
				"outcome":  "skipping provisioning",
				"message":  "throttling host creation",
			})
			return
		}
	}

	defer func() {
		if j.RetryInfo().GetRemainingAttempts() == 0 && j.HasErrors() && (j.host.Status == evergreen.HostUninitialized || j.host.Status == evergreen.HostBuilding) && j.host.SpawnOptions.SpawnedByTask {
			if err := task.AddHostCreateDetails(j.host.StartedBy, j.host.Id, j.host.SpawnOptions.TaskExecutionNumber, j.Error()); err != nil {
				j.AddError(errors.Wrapf(err, "error adding host create error details"))
			}
		}
	}()

	j.AddRetryableError(j.createHost(ctx))
}

func (j *createHostJob) selfThrottle() bool {
	var (
		numProv            int
		runningHosts       int
		distroRunningHosts int
		err                error
	)

	numProv, err = host.CountStartedTaskHosts()
	if err != nil {
		j.AddError(errors.Wrap(err, "problem getting count of pending pool size"))
		return true
	}

	distroRunningHosts, err = host.CountRunningHosts(j.host.Distro.Id)
	if err != nil {
		j.AddError(errors.Wrap(err, "problem getting count of pending pool size"))
		return true
	}

	runningHosts, err = host.CountAllRunningDynamicHosts()
	if err != nil {
		j.AddError(errors.Wrap(err, "problem getting count of pending pool size"))
		return true
	}

	if distroRunningHosts < runningHosts/100 || distroRunningHosts < j.host.Distro.HostAllocatorSettings.MinimumHosts {
		return false
	} else if numProv >= j.env.Settings().HostInit.HostThrottle {
		j.AddError(errors.Wrapf(j.host.Remove(), "problem removing host intent for %s", j.host.Id))
		return true
	}

	return false
}

var (
	errIgnorableCreateHost = errors.New("host.create encountered internal error")
)

func (j *createHostJob) createHost(ctx context.Context) error {
	var cloudManager cloud.Manager
	var err error
	if err = ctx.Err(); err != nil {
		return errors.Wrap(err, "canceling create host because context is canceled")
	}
	grip.Info(message.Fields{
		"message":      "attempting to start host",
		"host_id":      j.host.Id,
		"job":          j.ID(),
		"attempt":      j.RetryInfo().CurrentAttempt,
		"max_attempts": j.RetryInfo().MaxAttempts,
	})

	mgrOpts, err := cloud.GetManagerOptions(j.host.Distro)
	if err != nil {
		return errors.Wrapf(err, "can't get ManagerOpts for '%s'", j.host.Id)
	}
	cloudManager, err = cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "problem getting cloud provider for host",
			"host_id": j.host.Id,
			"job":     j.ID(),
		}))
		return errors.Wrapf(errIgnorableCreateHost, "problem getting cloud provider for host '%s' [%s]", j.host.Id, err.Error())
	}

	if j.host.Status != evergreen.HostUninitialized && j.host.Status != evergreen.HostBuilding {
		return nil
	}
	// Set status temporarily to HostBuilding. Conventional hosts only stay in
	// SpawnHost for a short period of time. Containers stay in SpawnHost for
	// longer, since they may need to download container images and build them
	// with the agent. This state allows intent documents to stay around until
	// SpawnHost returns, but NOT as initializing hosts that could still be
	// spawned by Evergreen.
	if err = j.host.SetStatusAtomically(evergreen.HostBuilding, evergreen.User, ""); err != nil {
		grip.Info(message.WrapError(err, message.Fields{
			"message": "host could not be transitioned from initializing to building, so it may already be building",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		return nil
	}

	// Containers should wait on image builds, checking to see if the parent
	// already has the image. If it does not, it should download it and wait
	// on the job until it is finished downloading.
	if j.host.ParentID != "" {
		var ready bool
		ready, err = j.isImageBuilt(ctx)
		if err != nil {
			return errors.Wrap(err, "problem building container image")
		}
		if !ready {
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				NeedsRetry: utility.TruePtr(),
			})
			return nil
		}
	}

	if _, err = cloudManager.SpawnHost(ctx, j.host); err != nil {
		if strings.Contains(err.Error(), cloud.EC2InsufficientCapacity) && j.host.ShouldFallbackToOnDemand() {
			event.LogHostFallback(j.host.Id)
			// create a new cloud manager for on demand, and re-attempt to spawn
			j.host.Provider = evergreen.ProviderNameEc2OnDemand
			j.host.Distro.Provider = evergreen.ProviderNameEc2OnDemand
			mgrOpts.Provider = j.host.Provider
			cloudManager, err = cloud.GetManager(ctx, j.env, mgrOpts)
			if err != nil {
				grip.Warning(message.WrapError(err, message.Fields{
					"message":   "problem getting cloud provider for host",
					"operation": "fallback to EC2 on-demand",
					"host_id":   j.host.Id,
					"job":       j.ID(),
				}))
				return errors.Wrapf(errIgnorableCreateHost, "problem getting cloud provider for host '%s' [%s]", j.host.Id, err.Error())
			}
			if _, err = cloudManager.SpawnHost(ctx, j.host); err != nil {
				return errors.Wrapf(err, "error falling back to on-demand for host '%s'", j.host.Id)
			}
		} else {
			return errors.Wrapf(err, "error spawning host '%s'", j.host.Id)
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

	if j.HostID == j.host.Id {
		// spawning the host did not change the ID, so we can replace the old host with the new one (ie. for docker containers)
		if err = j.host.Replace(); err != nil {
			return errors.Wrapf(err, "unable to replace host %s", j.host.Id)
		}
	} else {
		// for most cases, spawning a host with change the ID, so we remove/re-insert the document
		if err = host.RemoveStrict(j.HostID); err != nil {
			terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			grip.Error(message.WrapError(cloudManager.TerminateInstance(terminateCtx, j.host, evergreen.User, "hit database error trying to update host"), message.Fields{
				"message":     "problem terminating instance after cloud host was spawned",
				"host_id":     j.host.Id,
				"intent_host": j.HostID,
				"distro":      j.host.Distro.Id,
				"provider":    j.host.Provider,
				"job":         j.ID(),
			}))
			grip.Warning(message.WrapError(err, message.Fields{
				"message": "problem removing intent host",
				"job":     j.ID(),
				"host_id": j.HostID,
				"error":   err.Error(),
			}))
			return errors.Wrapf(err, "problem removing intent host '%s' [%s]", j.HostID, err.Error())
		}

		if err = j.host.Insert(); err != nil {
			terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			grip.Error(message.WrapError(cloudManager.TerminateInstance(terminateCtx, j.host, evergreen.User, "hit database error trying to update host"), message.Fields{
				"message":  "problem terminating instance after cloud host was spawned",
				"host_id":  j.host.Id,
				"distro":   j.host.Distro.Id,
				"provider": j.host.Provider,
				"job":      j.ID(),
			}))
			return errors.Wrapf(err, "error inserting host %s", j.host.Id)
		}
		if j.host.HasContainers {
			grip.Error(message.WrapError(j.host.UpdateParentIDs(), message.Fields{
				"message": "unable to update parent ID of containers",
				"host_id": j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
		}
	}

	event.LogHostStartFinished(j.host.Id, true)
	grip.Info(message.Fields{
		"message":      "successfully started host",
		"host_id":      j.host.Id,
		"distro":       j.host.Distro.Id,
		"provider":     j.host.Provider,
		"job":          j.ID(),
		"runtime_secs": time.Since(j.start).Seconds(),
	})

	return nil
}

func (j *createHostJob) isImageBuilt(ctx context.Context) (bool, error) {
	parent, err := j.host.GetParent()
	if err != nil {
		return false, errors.Wrapf(err, "problem getting parent for '%s'", j.host.Id)
	}
	if parent == nil {
		return false, errors.Wrapf(err, "parent for '%s' does not exist", j.host.Id)
	}

	if parent.Status != evergreen.HostRunning {
		grip.Warning(message.Fields{
			"message":       "parent for host not running",
			"host_id":       j.host.Id,
			"parent_status": parent.Status,
		})
		return false, nil
	}
	if ok := parent.ContainerImages[j.host.DockerOptions.Image]; ok {
		grip.Info(message.Fields{
			"message":  "image already exists, will start container",
			"host_id":  j.host.Id,
			"image":    j.host.DockerOptions.Image,
			"attempts": j.RetryInfo().CurrentAttempt,
			"job":      j.ID(),
		})
		return true, nil
	}

	//  If the image is not already present on the parent, run job to build the new image
	if !j.BuildImageStarted {
		grip.Info(message.Fields{
			"message":  "image not on host, will import image",
			"host_id":  j.host.Id,
			"image":    j.host.DockerOptions.Image,
			"attempts": j.RetryInfo().CurrentAttempt,
			"job":      j.ID(),
		})
		j.BuildImageStarted = true
		buildingContainerJob := NewBuildingContainerImageJob(j.env, parent, j.host.DockerOptions, j.host.Provider)
		err = j.env.RemoteQueue().Put(ctx, buildingContainerJob)
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "Duplicate key being added to job to block building containers",
		}))
	}
	return false, nil
}
