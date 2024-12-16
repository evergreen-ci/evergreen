package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	createHostJobName                     = "provisioning-create-host"
	maxPollAttempts                       = 100
	provisioningCreateHostAttributePrefix = "evergreen.provisioning_create_host"
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
	return j
}

func NewHostCreateJob(env evergreen.Environment, h host.Host, id string, buildImageStarted bool) amboy.Job {
	j := makeCreateHostJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s.%s", createHostJobName, h.Id, id))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", createHostJobName, h.Id)})
	j.SetEnqueueAllScopes(true)
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
		maxAttempts = j.host.SpawnOptions.Retries + 1
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

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(err)
		return
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
	var hostInit evergreen.HostInitConfig
	j.AddError(errors.Wrap(hostInit.Get(ctx), "refreshing hostinit settings"))

	if j.host == nil {
		j.host, err = host.FindOneId(ctx, j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			//host intent document has been removed by another evergreen process
			grip.Warning(message.Fields{
				"host_id": j.HostID,
				"attempt": j.RetryInfo().CurrentAttempt,
				"job":     j.ID(),
				"message": "host intent has been removed",
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
		distroActiveHosts, err := host.CountActiveHostsInDistro(ctx, j.host.Distro.Id)
		if err != nil {
			j.AddError(errors.Wrapf(err, "counting existing host pool size for distro '%s'", j.host.Distro.Id))
			return
		}

		removeHostIntent := false
		if distroActiveHosts > j.host.Distro.HostAllocatorSettings.MaximumHosts {
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

		allActiveDynamicHosts, err := host.CountActiveDynamicHosts(ctx)
		j.AddError(err)

		lowHostNumException := false
		if distroActiveHosts < 10 {
			lowHostNumException = true
		}

		if allActiveDynamicHosts > hostInit.MaxTotalDynamicHosts && !lowHostNumException {
			grip.Info(message.Fields{
				"host_id":                 j.HostID,
				"attempt":                 j.RetryInfo().CurrentAttempt,
				"distro":                  j.host.Distro.Id,
				"job":                     j.ID(),
				"provider":                j.host.Provider,
				"message":                 "not provisioning host to respect max_total_dynamic_hosts",
				"total_dynamic_hosts":     allActiveDynamicHosts,
				"max_total_dynamic_hosts": hostInit.MaxTotalDynamicHosts,
			})
			removeHostIntent = true
		}

		if removeHostIntent {
			err = errors.Wrap(j.host.Remove(ctx), "removing host intent to respect max distro hosts")

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

		if j.selfThrottle(ctx, hostInit) {
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
		if j.IsLastAttempt() && j.HasErrors() && (j.host.Status == evergreen.HostUninitialized || j.host.Status == evergreen.HostBuilding) {
			grip.Error(message.WrapError(j.Error(), message.Fields{
				"message": "no attempts remaining to create host",
				"outcome": "giving up on creating this host",
				"host_id": j.HostID,
				"distro":  j.host.Distro.Id,
			}))

			if j.host.SpawnOptions.SpawnedByTask {
				if err := task.AddHostCreateDetails(j.host.StartedBy, j.host.Id, j.host.SpawnOptions.TaskExecutionNumber, j.Error()); err != nil {
					j.AddError(errors.Wrapf(err, "adding host create error details"))
				}
			}
		}
	}()

	j.AddRetryableError(j.createHost(ctx))
}

func (j *createHostJob) selfThrottle(ctx context.Context, hostInit evergreen.HostInitConfig) bool {
	numProv, err := host.CountIdleStartedTaskHosts(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "counting pending host pool size"))
		return true
	}

	distroActiveHosts, err := host.CountActiveHostsInDistro(ctx, j.host.Distro.Id)
	if err != nil {
		j.AddError(errors.Wrapf(err, "counting host pool size for distro '%s'", j.host.Distro.Id))
		return true
	}

	allActiveDynamicHosts, err := host.CountActiveDynamicHosts(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "counting size of entire host pool"))
		return true
	}

	if distroActiveHosts < allActiveDynamicHosts/100 || distroActiveHosts < j.host.Distro.HostAllocatorSettings.MinimumHosts {
		return false
	} else if numProv >= hostInit.HostThrottle {
		reason := "host creation throttle"
		j.AddError(errors.Wrapf(j.host.SetStatusAtomically(ctx, evergreen.HostBuildingFailed, evergreen.User, reason), "getting rid of intent host '%s' for host creation throttle", j.host.Id))
		event.LogHostCreatedError(j.host.Id, reason)
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
		return errors.Wrap(err, "creating host")
	}
	grip.Info(message.Fields{
		"message":      "attempting to start host",
		"host_id":      j.host.Id,
		"job":          j.ID(),
		"attempt":      j.RetryInfo().CurrentAttempt,
		"max_attempts": j.RetryInfo().MaxAttempts,
	})

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String(evergreen.DistroIDOtelAttribute, j.host.Distro.Id),
		attribute.String(evergreen.HostIDOtelAttribute, j.host.Id),
		attribute.Bool(fmt.Sprintf("%s.spawned_host", provisioningCreateHostAttributePrefix), false),
	)

	mgrOpts, err := cloud.GetManagerOptions(j.host.Distro)
	if err != nil {
		return errors.Wrapf(err, "getting cloud manager options for distro '%s'", j.host.Distro.Id)
	}
	cloudManager, err = cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "problem getting cloud provider for host",
			"host_id": j.host.Id,
			"job":     j.ID(),
		}))
		return errors.Wrapf(errIgnorableCreateHost, "getting cloud provider for host '%s' [%s]", j.host.Id, err.Error())
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
	if err = j.host.SetStatusAtomically(ctx, evergreen.HostBuilding, evergreen.User, ""); err != nil {
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
			return errors.Wrap(err, "checking if container image is built")
		}
		if !ready {
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				NeedsRetry: utility.TruePtr(),
			})
			return nil
		}
	}

	hostReplaced, err := j.spawnAndReplaceHost(ctx, cloudManager)
	if err != nil {
		if j.host.UserHost {
			// Log a more specific event than the generic host created error
			// when the host is a spawn host. This makes subscriptions on spawn
			// host errors more efficient because the notification system can
			// process just spawn host errors rather than every single host
			// creation error.
			event.LogSpawnHostCreatedError(j.host.Id, err.Error())
		}
		event.LogHostCreatedError(j.host.Id, err.Error())
		return errors.Wrapf(err, "spawning and updating host '%s'", j.host.Id)
	}

	if hostReplaced {
		event.LogHostStartSucceeded(j.host.Id, evergreen.User)
	}

	grip.Info(message.Fields{
		"message":      "successfully started host",
		"host_id":      j.host.Id,
		"host_tag":     j.host.Tag,
		"distro":       j.host.Distro.Id,
		"provider":     j.host.Provider,
		"job":          j.ID(),
		"runtime_secs": time.Since(j.start).Seconds(),
	})
	span.SetAttributes(attribute.Bool(fmt.Sprintf("%s.spawned_host", provisioningCreateHostAttributePrefix), true))

	return nil
}

func (j *createHostJob) isImageBuilt(ctx context.Context) (bool, error) {
	parent, err := j.host.GetParent(ctx)
	if err != nil {
		return false, errors.Wrapf(err, "getting parent host for container '%s'", j.host.Id)
	}
	if parent == nil {
		return false, errors.Wrapf(err, "parent for container '%s' not found", j.host.Id)
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

// spawnAndUpdateHost attempts to spawn the host and update the host document.
func (j *createHostJob) spawnAndReplaceHost(ctx context.Context, cloudMgr cloud.Manager) (replaced bool, err error) {
	if _, err = cloudMgr.SpawnHost(ctx, j.host); err != nil {
		return false, errors.Wrapf(err, "spawning host '%s'", j.host.Id)
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
		// Spawning the host did not change the ID, so we can replace the old
		// host with the new one (e.g. for Docker containers).
		if err = j.host.Replace(ctx); err != nil {
			return false, errors.Wrapf(err, "replacing host '%s'", j.host.Id)
		}
		return true, nil
	}

	// For most cases, spawning a host will change the ID, so we remove/re-insert the document
	hostReplaced, err := j.tryHostReplacement(ctx, cloudMgr)
	if err != nil {
		return hostReplaced, errors.Wrap(err, "attempting host replacement")
	}

	if j.host.HasContainers {
		grip.Error(message.WrapError(j.host.UpdateParentIDs(ctx), message.Fields{
			"message": "unable to update parent ID of containers",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
	}
	return hostReplaced, nil
}

// tryHostReplacement attempts to atomically replace the intent host with the
// real host that was created by this job. If that fails, it checks to see if
// the real host already exists. If both of those fail, it will attempt to
// terminate the cloud host.
func (j *createHostJob) tryHostReplacement(ctx context.Context, cloudMgr cloud.Manager) (replaced bool, err error) {
	if err = host.UnsafeReplace(ctx, j.env, j.HostID, j.host); err == nil {
		return true, nil
	}

	grip.Warning(message.WrapError(err, message.Fields{
		"message":        "could not perform unsafe host replacement",
		"job":            j.ID(),
		"intent_host_id": j.HostID,
		"host_id":        j.host.Id,
		"distro":         j.host.Distro.Id,
		"provider":       j.host.Provider,
	}))

	// Host replacement can fail due to timing issues, but the host might have
	// already been replaced successfully.
	//
	// It's possible for an agent to start on an instance and ask for a task
	// before this host replacement operation succeeds. In this case, the agent
	// REST route swaps the intent host for the real host. However, that would
	// conflict with this job and cause it to error here, because it cannot
	// remove the old, now-nonexistent intent host.
	//
	// In the case of the agent asking for a task before this succeeds,
	// check to see if the instance ID exists in the DB. If so,
	// the host has already been swapped successfully.
	checkHost, _ := host.FindOneId(ctx, j.host.Id)
	if checkHost != nil {
		return false, nil
	}

	terminateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	grip.Error(message.WrapError(cloudMgr.TerminateInstance(terminateCtx, j.host, evergreen.User, "hit database error trying to update host"), message.Fields{
		"message":        "could not terminate host after failed unsafe host replacement",
		"host_id":        j.host.Id,
		"intent_host_id": j.HostID,
		"distro":         j.host.Distro.Id,
		"provider":       j.host.Provider,
		"job":            j.ID(),
	}))

	return false, errors.Wrapf(err, "replacing intent host '%s' with real host '%s'", j.HostID, j.host.Id)
}

func EnqueueHostCreateJobs(ctx context.Context, env evergreen.Environment, hostIntents []host.Host) error {
	appCtx, _ := env.Context()
	queue, err := env.RemoteQueueGroup().Get(appCtx, createHostQueueGroup)
	if err != nil {
		return errors.Wrap(err, "getting host create queue")
	}

	catcher := grip.NewBasicCatcher()
	ts := utility.RoundPartOfHour(0).Format(TSFormat)
	for _, intent := range hostIntents {
		catcher.Add(errors.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewHostCreateJob(env, intent, ts, false)), "enqueueing host create job for '%s'", intent.Id))
	}

	return catcher.Resolve()
}
