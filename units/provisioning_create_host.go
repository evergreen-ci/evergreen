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
	maxHostCreateAttempts                 = 6
	provisioningCreateHostAttributePrefix = "evergreen.provisioning_create_host"
	createHostIntentHostIDOtelAttribute   = provisioningCreateHostAttributePrefix + ".intent_host_id"
	createHostBuildImageOtelAttribute     = provisioningCreateHostAttributePrefix + ".build_image_started"
	createHostOutcomeOtelAttribute        = provisioningCreateHostAttributePrefix + ".outcome"
	createHostStageOtelAttribute          = provisioningCreateHostAttributePrefix + ".stage"
	createHostInitialStatusOtelAttribute  = provisioningCreateHostAttributePrefix + ".initial_host_status"
	createHostFinalStatusOtelAttribute    = provisioningCreateHostAttributePrefix + ".final_host_status"
	createHostSingleTaskOtelAttribute     = provisioningCreateHostAttributePrefix + ".single_task_distro"
	createHostUserHostOtelAttribute       = provisioningCreateHostAttributePrefix + ".user_host"
	createHostSpawnedByTaskOtelAttribute  = provisioningCreateHostAttributePrefix + ".spawned_by_task"
	createHostParentIDOtelAttribute       = provisioningCreateHostAttributePrefix + ".parent_host_id"
	createHostSpawnProjectOtelAttribute   = provisioningCreateHostAttributePrefix + ".spawned_by_project_id"
	createHostSpawnTaskOtelAttribute      = provisioningCreateHostAttributePrefix + ".spawned_by_task_id"
	createHostSpawnBuildOtelAttribute     = provisioningCreateHostAttributePrefix + ".spawned_by_build_id"
	createHostRetryOtelAttribute          = provisioningCreateHostAttributePrefix + ".retry_requested"
	createHostReplacedOtelAttribute       = provisioningCreateHostAttributePrefix + ".host_replaced"
	createHostThrottleReasonOtelAttribute = provisioningCreateHostAttributePrefix + ".throttle_reason"
	createHostDistroActiveOtelAttribute   = provisioningCreateHostAttributePrefix + ".distro_active_hosts"
	createHostDistroMaxOtelAttribute      = provisioningCreateHostAttributePrefix + ".distro_max_hosts"
	createHostDistroLimitOtelAttribute    = provisioningCreateHostAttributePrefix + ".distro_host_limit_exceeded"
	createHostDynamicActiveOtelAttribute  = provisioningCreateHostAttributePrefix + ".total_dynamic_hosts"
	createHostDynamicMaxOtelAttribute     = provisioningCreateHostAttributePrefix + ".max_total_dynamic_hosts"
	createHostDynamicLimitOtelAttribute   = provisioningCreateHostAttributePrefix + ".global_host_limit_exceeded"
	createHostLowHostNumOtelAttribute     = provisioningCreateHostAttributePrefix + ".low_host_num_exception"
	createHostPendingOtelAttribute        = provisioningCreateHostAttributePrefix + ".pending_task_hosts"
	createHostThrottleOtelAttribute       = provisioningCreateHostAttributePrefix + ".host_throttle"
	createHostDistroMinOtelAttribute      = provisioningCreateHostAttributePrefix + ".distro_min_hosts"
	createHostSpawnedOtelAttribute        = provisioningCreateHostAttributePrefix + ".spawned_host"
	maxHostCreateJobTime                  = 30 * time.Minute
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
		MaxTime:    maxHostCreateJobTime,
	})
	var wait time.Duration
	var maxAttempts int

	if h.ParentID != "" {
		wait = time.Minute
		maxAttempts = maxPollAttempts
	} else if j.host.SpawnOptions.SpawnedByTask {
		wait = 10 * time.Second
		maxAttempts = j.host.SpawnOptions.Retries + 1
	} else if j.host.StartedBy == evergreen.User {
		maxAttempts = maxHostCreateAttempts
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
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String(createHostIntentHostIDOtelAttribute, j.HostID),
		attribute.Bool(createHostBuildImageOtelAttribute, j.BuildImageStarted),
		attribute.String(createHostOutcomeOtelAttribute, "failed"),
		attribute.String(createHostStageOtelAttribute, "load_service_flags"),
		attribute.Bool(createHostSpawnedOtelAttribute, false),
	)
	intentRemoved := false
	defer func() {
		attrs := []attribute.KeyValue{
			attribute.Bool(createHostBuildImageOtelAttribute, j.BuildImageStarted),
			attribute.Bool(createHostRetryOtelAttribute, j.RetryInfo().NeedsRetry),
		}
		if j.host != nil {
			attrs = append(attrs, attribute.String(evergreen.HostIDOtelAttribute, j.host.Id))
			// Once the intent document is deleted, the in-memory status no longer
			// corresponds to any stored host, so record an explicit marker instead.
			if intentRemoved {
				attrs = append(attrs, attribute.String(createHostFinalStatusOtelAttribute, "intent_removed"))
			} else {
				attrs = append(attrs, attribute.String(createHostFinalStatusOtelAttribute, j.host.Status))
			}
		}
		span.SetAttributes(attrs...)
	}()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(err)
		return
	}

	if flags.HostInitDisabled {
		span.SetAttributes(attribute.String(createHostOutcomeOtelAttribute, "disabled"))
		grip.Debug(ctx, message.Fields{
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
	span.SetAttributes(attribute.String(createHostStageOtelAttribute, "load_host_init"))
	var hostInit evergreen.HostInitConfig
	j.AddError(errors.Wrap(hostInit.Get(ctx), "refreshing hostinit settings"))

	span.SetAttributes(attribute.String(createHostStageOtelAttribute, "load_host"))
	if j.host == nil {
		j.host, err = host.FindOneId(ctx, j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			span.SetAttributes(attribute.String(createHostOutcomeOtelAttribute, "host_missing"))
			//host intent document has been removed by another evergreen process
			grip.Warning(ctx, message.Fields{
				"host_id": j.HostID,
				"attempt": j.RetryInfo().CurrentAttempt,
				"job":     j.ID(),
				"message": "host intent has been removed",
			})
			return
		}
	}

	hostAttrs := []attribute.KeyValue{
		attribute.String(evergreen.DistroIDOtelAttribute, j.host.Distro.Id),
		attribute.String(evergreen.DistroProviderOtelAttribute, j.host.Distro.Provider),
		attribute.String(createHostInitialStatusOtelAttribute, j.host.Status),
		attribute.Bool(createHostSingleTaskOtelAttribute, j.host.Distro.SingleTaskDistro),
		attribute.String(evergreen.HostStartedByOtelAttribute, j.host.StartedBy),
		attribute.Bool(createHostUserHostOtelAttribute, j.host.UserHost),
		attribute.Bool(createHostSpawnedByTaskOtelAttribute, j.host.SpawnOptions.SpawnedByTask),
	}
	if j.host.ParentID != "" {
		hostAttrs = append(hostAttrs, attribute.String(createHostParentIDOtelAttribute, j.host.ParentID))
	}
	if j.host.SpawnOptions.ProjectID != "" {
		hostAttrs = append(hostAttrs, attribute.String(createHostSpawnProjectOtelAttribute, j.host.SpawnOptions.ProjectID))
	}
	if j.host.SpawnOptions.TaskID != "" {
		hostAttrs = append(hostAttrs, attribute.String(createHostSpawnTaskOtelAttribute, j.host.SpawnOptions.TaskID))
	}
	if j.host.SpawnOptions.BuildID != "" {
		hostAttrs = append(hostAttrs, attribute.String(createHostSpawnBuildOtelAttribute, j.host.SpawnOptions.BuildID))
	}
	span.SetAttributes(hostAttrs...)

	span.SetAttributes(attribute.String(createHostStageOtelAttribute, "validate_host"))
	if j.host.Status != evergreen.HostUninitialized && j.host.Status != evergreen.HostBuilding {
		span.SetAttributes(attribute.String(createHostOutcomeOtelAttribute, "already_started"))
		grip.Notice(ctx, message.Fields{
			"message": "host has already been started",
			"status":  j.host.Status,
			"host_id": j.host.Id,
		})
		return
	}

	if j.host.IsSubjectToHostCreationThrottle() {
		span.SetAttributes(attribute.String(createHostStageOtelAttribute, "check_throttle"))
		distroActiveHosts, err := host.CountActiveHostsInDistro(ctx, j.host.Distro.Id)
		if err != nil {
			j.AddError(errors.Wrapf(err, "counting existing host pool size for distro '%s'", j.host.Distro.Id))
			return
		}
		distroLimitExceeded := distroActiveHosts > j.host.Distro.HostAllocatorSettings.MaximumHosts
		span.SetAttributes(
			attribute.Int(createHostDistroActiveOtelAttribute, distroActiveHosts),
			attribute.Int(createHostDistroMaxOtelAttribute, j.host.Distro.HostAllocatorSettings.MaximumHosts),
			attribute.Bool(createHostDistroLimitOtelAttribute, distroLimitExceeded),
		)

		removeHostIntent := false
		// When both limits are exceeded, the distro limit takes precedence in
		// throttle_reason; the *_limit_exceeded attributes record each limit independently.
		throttleReason := ""
		if distroLimitExceeded {
			throttleReason = "distro_host_limit"
			grip.Info(ctx, message.Fields{
				"host_id":            j.HostID,
				"attempt":            j.RetryInfo().CurrentAttempt,
				"distro":             j.host.Distro.Id,
				"single_task_distro": j.host.Distro.SingleTaskDistro,
				"job":                j.ID(),
				"provider":           j.host.Provider,
				"message":            "not provisioning host to respect maxhosts",
				"max_hosts":          j.host.Distro.HostAllocatorSettings.MaximumHosts,
				"active_hosts":       distroActiveHosts,
			})
			removeHostIntent = true
		}

		allActiveDynamicHosts, err := host.CountActiveDynamicHosts(ctx)
		j.AddError(err)

		lowHostNumException := false
		if distroActiveHosts < 10 {
			lowHostNumException = true
		}
		span.SetAttributes(attribute.Bool(createHostLowHostNumOtelAttribute, lowHostNumException))
		globalLimitExceeded := allActiveDynamicHosts > hostInit.MaxTotalDynamicHosts && !lowHostNumException
		// Only record the global-limit attributes if the count succeeded; otherwise the
		// span would assert a verified under-limit state based on an unknown count.
		if err == nil {
			span.SetAttributes(
				attribute.Int(createHostDynamicActiveOtelAttribute, allActiveDynamicHosts),
				attribute.Int(createHostDynamicMaxOtelAttribute, hostInit.MaxTotalDynamicHosts),
				attribute.Bool(createHostDynamicLimitOtelAttribute, globalLimitExceeded),
			)
		}

		if globalLimitExceeded {
			if throttleReason == "" {
				throttleReason = "global_host_limit"
			}
			grip.Info(ctx, message.Fields{
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
			span.SetAttributes(
				attribute.String(createHostOutcomeOtelAttribute, "throttled"),
				attribute.String(createHostThrottleReasonOtelAttribute, throttleReason),
			)
			err = errors.Wrap(j.host.Remove(ctx), "removing host intent to respect max distro hosts")
			intentRemoved = err == nil

			j.AddError(err)
			grip.Error(ctx, message.WrapError(err, message.Fields{
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
			grip.Debug(ctx, message.Fields{
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
			grip.Error(ctx, message.WrapError(j.Error(), message.Fields{
				"message":      "no attempts remaining to create host",
				"outcome":      "giving up on creating this host",
				"host_id":      j.HostID,
				"distro":       j.host.Distro.Id,
				"max_attempts": j.RetryInfo().MaxAttempts,
			}))

			if j.host.SpawnOptions.SpawnedByTask {
				if err := task.AddHostCreateDetails(ctx, j.host.StartedBy, j.host.Id, j.host.SpawnOptions.TaskExecutionNumber, j.Error()); err != nil {
					j.AddError(errors.Wrapf(err, "adding host create error details"))
				}
			}
		}
	}()

	err = j.createHost(ctx)
	if err != nil && strings.Contains(err.Error(), cloud.EC2InsufficientCapacity) {
		j.AddRetryableError(err)
	} else {
		j.AddError(err)
	}
}

func (j *createHostJob) selfThrottle(ctx context.Context, hostInit evergreen.HostInitConfig) bool {
	span := trace.SpanFromContext(ctx)
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
	// The distro and dynamic host counts are not recorded here because Run's throttle check
	// already recorded them on this span; overwriting them with these fresh, independent
	// counts could contradict the limit decisions recorded there.
	span.SetAttributes(
		attribute.Int(createHostPendingOtelAttribute, numProv),
		attribute.Int(createHostThrottleOtelAttribute, hostInit.HostThrottle),
		attribute.Int(createHostDistroMinOtelAttribute, j.host.Distro.HostAllocatorSettings.MinimumHosts),
	)

	if distroActiveHosts < allActiveDynamicHosts/100 || distroActiveHosts < j.host.Distro.HostAllocatorSettings.MinimumHosts {
		return false
	} else if numProv >= hostInit.HostThrottle {
		reason := "host creation throttle"
		span.SetAttributes(
			attribute.String(createHostOutcomeOtelAttribute, "throttled"),
			attribute.String(createHostThrottleReasonOtelAttribute, "host_creation_throttle"),
		)
		j.AddError(errors.Wrapf(j.host.SetStatusAtomically(ctx, evergreen.HostBuildingFailed, evergreen.User, reason), "getting rid of intent host '%s' for host creation throttle", j.host.Id))
		event.LogHostCreatedError(ctx, j.host.Id, reason)
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
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(createHostStageOtelAttribute, "check_context"))
	if err = ctx.Err(); err != nil {
		return errors.Wrap(err, "creating host")
	}
	grip.Info(ctx, message.Fields{
		"message":            "attempting to start host",
		"host_id":            j.host.Id,
		"single_task_distro": j.host.Distro.SingleTaskDistro,
		"job":                j.ID(),
		"attempt":            j.RetryInfo().CurrentAttempt,
		"max_attempts":       j.RetryInfo().MaxAttempts,
	})

	span.SetAttributes(attribute.String(createHostStageOtelAttribute, "get_cloud_manager"))
	mgrOpts, err := cloud.GetManagerOptions(ctx, j.host.Distro)
	if err != nil {
		return errors.Wrapf(err, "getting cloud manager options for distro '%s'", j.host.Distro.Id)
	}
	cloudManager, err = cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message": "problem getting cloud provider for host",
			"host_id": j.host.Id,
			"job":     j.ID(),
		}))
		return errors.Wrapf(errIgnorableCreateHost, "getting cloud provider for host '%s' [%s]", j.host.Id, err.Error())
	}

	span.SetAttributes(attribute.String(createHostStageOtelAttribute, "transition_host_status"))
	if j.host.Status != evergreen.HostUninitialized && j.host.Status != evergreen.HostBuilding {
		span.SetAttributes(attribute.String(createHostOutcomeOtelAttribute, "already_started"))
		return nil
	}
	// Set status temporarily to HostBuilding. Conventional hosts only stay in
	// SpawnHost for a short period of time. Containers stay in SpawnHost for
	// longer, since they may need to download container images and build them
	// with the agent. This state allows intent documents to stay around until
	// SpawnHost returns, but NOT as initializing hosts that could still be
	// spawned by Evergreen.
	if err = j.host.SetStatusAtomically(ctx, evergreen.HostBuilding, evergreen.User, ""); err != nil {
		span.SetAttributes(attribute.String(createHostOutcomeOtelAttribute, "status_transition_skipped"))
		grip.Info(ctx, message.WrapError(err, message.Fields{
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
		span.SetAttributes(attribute.String(createHostStageOtelAttribute, "wait_for_image"))
		var ready bool
		ready, err = j.isImageBuilt(ctx)
		if err != nil {
			return errors.Wrap(err, "checking if container image is built")
		}
		if !ready {
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				NeedsRetry: utility.TruePtr(),
			})
			span.SetAttributes(attribute.String(createHostOutcomeOtelAttribute, "waiting_for_image"))
			return nil
		}
	}

	span.SetAttributes(attribute.String(createHostStageOtelAttribute, "spawn_host"))
	hostReplaced, err := j.spawnAndReplaceHost(ctx, cloudManager)
	if err != nil {
		if j.host.UserHost {
			// Log a more specific event than the generic host created error
			// when the host is a spawn host. This makes subscriptions on spawn
			// host errors more efficient because the notification system can
			// process just spawn host errors rather than every single host
			// creation error.
			event.LogSpawnHostCreatedError(ctx, j.host.Id, err.Error())
		}
		event.LogHostCreatedError(ctx, j.host.Id, err.Error())
		return errors.Wrapf(err, "spawning and updating host '%s'", j.host.Id)
	}
	span.SetAttributes(
		attribute.Bool(createHostReplacedOtelAttribute, hostReplaced),
		attribute.String(evergreen.HostIDOtelAttribute, j.host.Id),
	)

	if hostReplaced {
		event.LogHostStartSucceeded(ctx, j.host.Id, evergreen.User)
	}

	grip.Info(ctx, message.Fields{
		"message":            "successfully started host",
		"host_id":            j.host.Id,
		"host_tag":           j.host.Tag,
		"distro":             j.host.Distro.Id,
		"single_task_distro": j.host.Distro.SingleTaskDistro,
		"provider":           j.host.Provider,
		"subnet":             j.host.GetSubnetID(),
		"started_by":         j.host.StartedBy,
		"job":                j.ID(),
		"runtime_secs":       time.Since(j.start).Seconds(),
		"num_attempts":       j.RetryInfo().CurrentAttempt,
	})
	span.SetAttributes(
		attribute.String(createHostOutcomeOtelAttribute, "spawned"),
		attribute.String(createHostStageOtelAttribute, "complete"),
		attribute.Bool(createHostSpawnedOtelAttribute, true),
	)

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
		grip.Warning(ctx, message.Fields{
			"message":       "parent for host not running",
			"host_id":       j.host.Id,
			"parent_status": parent.Status,
		})
		return false, nil
	}
	if ok := parent.ContainerImages[j.host.DockerOptions.Image]; ok {
		grip.Info(ctx, message.Fields{
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
		grip.Info(ctx, message.Fields{
			"message":  "image not on host, will import image",
			"host_id":  j.host.Id,
			"image":    j.host.DockerOptions.Image,
			"attempts": j.RetryInfo().CurrentAttempt,
			"job":      j.ID(),
		})
		j.BuildImageStarted = true
		buildingContainerJob := NewBuildingContainerImageJob(j.env, parent, j.host.DockerOptions, j.host.Provider)
		err = j.env.RemoteQueue().Put(ctx, buildingContainerJob)
		grip.Debug(ctx, message.WrapError(err, message.Fields{
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

	if j.host.IPAllocationID != "" {
		appCtx, _ := j.env.Context()
		hostIPAssociationQueueGroup, _ := j.env.RemoteQueueGroup().Get(appCtx, hostIPAssociationQueueGroup)
		if hostIPAssociationQueueGroup != nil {
			if err := amboy.EnqueueUniqueJob(ctx, hostIPAssociationQueueGroup, NewHostIPAssociationJob(j.env, j.host, time.Now().Format(TSFormat))); err != nil {
				grip.Warning(ctx, message.WrapError(err, message.Fields{
					"message": "could not enqueue host IP association job for host",
					"host_id": j.host.Id,
					"distro":  j.host.Distro.Id,
				}))
			}
		}
	}

	if j.host.HasContainers {
		grip.Error(ctx, message.WrapError(j.host.UpdateParentIDs(ctx), message.Fields{
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

	grip.Warning(ctx, message.WrapError(err, message.Fields{
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
	grip.Error(ctx, message.WrapError(cloudMgr.TerminateInstance(terminateCtx, j.host, evergreen.User, "hit database error trying to update host"), message.Fields{
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
