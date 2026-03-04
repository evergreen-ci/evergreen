package units

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	TSFormat = "2006-01-02.15-04-05"

	// createHostQueueGroup is the queue group for the provisioning-create-host job.
	createHostQueueGroup = "service.host.create"
	// terminateHostQueueGroup is the queue group for host-termination-jobs.
	terminateHostQueueGroup         = "service.host.termination"
	eventNotifierQueueGroup         = "service.event.notifier"
	spawnHostModificationQueueGroup = "service.spawnhost.modify"
	hostIPAssociationQueueGroup     = "service.host.ip.associate"
)

type cronJobFactory func(context.Context, evergreen.Environment, time.Time) ([]amboy.Job, error)

func PopulateActivationJobs(part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.SchedulerDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "scheduler is disabled",
				"impact":  "skipping batch time activation",
				"mode":    "degraded",
			})
			return nil
		}

		return queue.Put(ctx, NewVersionActivationJob(utility.RoundPartOfHour(part).Format(TSFormat)))
	}
}

func hostMonitoringJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}

	if flags.MonitorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "monitor is disabled",
			"impact":  "not detecting externally terminated hosts",
			"mode":    "degraded",
		})
		return nil, nil
	}

	const reachabilityCheckInterval = 10 * time.Minute
	threshold := time.Now().Add(-reachabilityCheckInterval)
	hosts, err := host.Find(ctx, host.ByNotMonitoredSince(threshold))
	if err != nil {
		return nil, errors.Wrap(err, "finding hosts")
	}

	grip.InfoWhen(len(hosts) > 0, message.Fields{
		"runner":    "monitor",
		"operation": "host reachability monitor",
		"num_hosts": len(hosts),
	})

	jobs := []amboy.Job{NewStrandedTaskCleanupJob(ts.Format(TSFormat))}
	for _, host := range hosts {
		jobs = append(jobs, NewHostMonitoringCheckJob(env, &host, ts.Format(TSFormat)))
	}
	return jobs, nil
}

func sendNotificationJobs(ctx context.Context, _ evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if flags.EventProcessingDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "notifications disabled",
			"impact":  "not sending notifications",
			"mode":    "degraded",
		})
		return nil, nil
	}

	unprocessedNotifications, err := notification.FindUnprocessed(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding unprocessed notifications")
	}
	return notificationJobs(ctx, unprocessedNotifications, flags, ts)
}

func eventNotifierJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}

	if flags.EventProcessingDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "event processing disabled",
			"impact":  "not processing events",
			"mode":    "degraded",
		})
		return nil, nil
	}

	events, err := event.FindUnprocessedEvents(ctx, -1)
	if err != nil {
		return nil, errors.Wrap(err, "finding all unprocessed events")
	}
	grip.Info(message.Fields{
		"message": "unprocessed event count",
		"pending": len(events),
		"source":  "events-processing",
	})

	var jobs []amboy.Job
	for _, evt := range events {
		jobs = append(jobs, NewEventNotifierJob(env, evt.ID, ts.Format(TSFormat)))
	}
	return jobs, nil
}

func PopulateTaskMonitoring(mins int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.MonitorDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "monitor is disabled",
				"impact":  "not detecting task heartbeat/dispatching timeouts",
				"mode":    "degraded",
			})
			return nil
		}

		return queue.Put(ctx, NewTaskExecutionMonitorPopulateJob(utility.RoundPartOfHour(mins).Format(TSFormat)))
	}
}

func hostTerminationJobs(ctx context.Context, env evergreen.Environment, _ time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}

	if flags.MonitorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "monitor is disabled",
			"impact":  "not submitting termination jobs for dead/killable hosts",
			"mode":    "degraded",
		})
		return nil, nil
	}

	catcher := grip.NewBasicCatcher()
	hosts, err := host.FindHostsToTerminate(ctx)
	grip.Error(message.WrapError(err, message.Fields{
		"operation": "populate host termination jobs",
		"cron":      HostTerminationJobName,
		"impact":    "hosts termination interrupted",
	}))
	catcher.Wrap(err, "finding hosts to terminate")

	var jobs []amboy.Job
	for _, h := range hosts {
		if h.NoExpiration && h.UserHost && h.Status != evergreen.HostProvisionFailed && time.Now().After(h.ExpirationTime) {
			grip.Error(message.Fields{
				"message": "attempting to terminate an expired host marked as non-expiring",
				"host":    h.Id,
			})
			continue
		}
		jobs = append(jobs, NewHostTerminationJob(env, &h, HostTerminationOptions{
			TerminateIfBusy:   true,
			TerminationReason: "host is expired, decommissioned, or failed to provision (this is not an error)",
		}))
	}

	hosts, err = host.AllHostsSpawnedByTasksToTerminate(ctx)
	grip.Error(message.WrapError(err, message.Fields{
		"operation": "populate hosts spawned by tasks termination jobs",
		"cron":      HostTerminationJobName,
		"impact":    "hosts termination interrupted",
	}))
	catcher.Wrap(err, "finding hosts spawned by tasks to terminate")
	for _, h := range hosts {
		jobs = append(jobs, NewHostTerminationJob(env, &h, HostTerminationOptions{
			TerminateIfBusy:   true,
			TerminationReason: "host spawned by task has gone out of scope",
		}))
	}
	return jobs, catcher.Resolve()
}

func lastContainerFinishTimeJobs(ctx context.Context, _ evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	return []amboy.Job{NewLastContainerFinishTimeJob(ts.Format(TSFormat))}, nil
}

func parentDecommissionJobs(ctx context.Context, _ evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting admin settings")
	}
	containerPools := settings.ContainerPools.Pools

	// Create a ParentDecommissionJob for each distro.
	var jobs []amboy.Job
	for _, c := range containerPools {
		jobs = append(jobs, NewParentDecommissionJob(ts.Format(TSFormat), c.Distro, c.MaxContainers))
	}

	return jobs, nil
}

func containerStateJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	parents, err := host.FindAllRunningParents(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding parent hosts")
	}

	var jobs []amboy.Job
	// Create a job to check container state consistency for each parent.
	for _, p := range parents {
		jobs = append(jobs, NewHostMonitorContainerStateJob(&p, evergreen.ProviderNameDocker, ts.Format(TSFormat)))
	}
	return jobs, nil
}

func oldestImageRemovalJobs(ctx context.Context, _ evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	parents, err := host.FindAllRunningParents(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding all parent hosts")
	}

	var jobs []amboy.Job
	// Create an oldestImageJob when images take up too much disk space.
	for _, p := range parents {
		jobs = append(jobs, NewOldestImageRemovalJob(&p, evergreen.ProviderNameDocker, ts.Format(TSFormat)))
	}
	return jobs, nil
}

func hostAllocatorJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if config.ServiceFlags.HostAllocatorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "host allocation is disabled",
			"impact":  "new hosts cannot be allocated",
			"mode":    "degraded",
		})
		return nil, nil
	}

	// find all active distros
	distros, err := distro.Find(ctx, distro.ByNeedsHostsPlanning(config.ContainerPools.Pools))
	if err != nil {
		return nil, errors.Wrap(err, "finding distros that need planning")
	}

	jobs := make([]amboy.Job, 0, len(distros))
	for _, d := range distros {
		jobs = append(jobs, NewHostAllocatorJob(env, d.Id, ts))
	}

	return jobs, nil
}

func schedulerJobs(ctx context.Context, _ evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting admin settings")
	}

	if config.ServiceFlags.SchedulerDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "scheduler is disabled",
			"impact":  "new tasks are not enqueued",
			"mode":    "degraded",
		})
		return nil, nil
	}

	// find all active distros
	distros, err := distro.Find(ctx, distro.ByNeedsPlanning(config.ContainerPools.Pools))
	if err != nil {
		return nil, errors.Wrap(err, "finding distros that need planning")
	}

	jobs := make([]amboy.Job, 0, len(distros))
	for _, d := range distros {
		if d.IsParent(config) {
			continue
		}
		jobs = append(jobs, NewDistroSchedulerJob(d.Id, ts.Format(TSFormat)))
	}
	return jobs, nil
}

func aliasSchedulerJobs(ctx context.Context, _ evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting admin settings")
	}

	if config.ServiceFlags.SchedulerDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "scheduler is disabled",
			"impact":  "new tasks are not enqueued",
			"mode":    "degraded",
		})
		return nil, nil
	}

	// find all active distros
	distros, err := distro.Find(ctx, distro.ByNeedsPlanning(config.ContainerPools.Pools))
	if err != nil {
		return nil, errors.Wrap(err, "finding distros that need planning")
	}

	jobs := make([]amboy.Job, 0, len(distros))
	for _, d := range distros {
		if d.IsParent(config) {
			continue
		}
		jobs = append(jobs, NewDistroAliasSchedulerJob(d.Id, ts.Format(TSFormat)))
	}

	return jobs, nil
}

func idleHostJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}

	if flags.MonitorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "monitor is disabled",
			"impact":  "not submitting detecting idle hosts",
			"mode":    "degraded",
		})
		return nil, nil
	}

	return []amboy.Job{NewIdleHostTerminationJob(env, ts.Format(TSFormat))}, nil
}

func PopulateCheckUnmarkedBlockedTasks() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}
		if flags.CheckBlockedTasksDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "CheckBlockedTasks job is disabled",
				"impact":  "new tasks are not enqueued",
				"mode":    "degraded",
			})
			return nil
		}

		config, err := evergreen.GetConfig(ctx)
		if err != nil {
			return errors.Wrap(err, "getting admin settings")
		}

		catcher := grip.NewBasicCatcher()
		// find all active distros
		distros, err := distro.Find(ctx, distro.ByNeedsPlanning(config.ContainerPools.Pools))
		catcher.Wrap(err, "getting distros that need planning")

		ts := utility.RoundPartOfMinute(0)
		for _, d := range distros {
			catcher.Wrapf(queue.Put(ctx, NewCheckBlockedTasksJob(d.Id, ts)), "enqueueing check blocked tasks job for distro '%s'", d.Id)
		}
		// Passing an empty string as the distro id will enqueue a job for all container tasks
		catcher.Wrap(queue.Put(ctx, NewCheckBlockedTasksJob("", ts)), "enqueueing check blocked tasks job for container tasks")
		return catcher.Resolve()
	}
}

func PopulateDuplicateTaskCheckJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		return queue.Put(ctx, NewDuplicateTaskCheckJob(ts))
	}
}

// PopulateHostStatJobs adds host stats jobs.
func PopulateHostStatJobs(parts int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		ts := utility.RoundPartOfHour(parts).Format(TSFormat)
		return queue.Put(ctx, NewHostStatsJob(ts))
	}
}

func agentDeployJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}

	if flags.AgentStartDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "agent start disabled",
			"impact":  "agents are not deployed",
			"mode":    "degraded",
		})
		return nil, nil
	}

	// For each host, set its last communication time to now and its needs new agent
	// flag to true. This ensures a consistent state in the agent-deploy job. That job
	// uses setting the NeedsNewAgent field to false to prevent other jobs from running
	// concurrently.
	err = host.UpdateAll(ctx, host.NeedsAgentDeploy(time.Now()), bson.M{"$set": bson.M{
		host.NeedsNewAgentKey: true,
	}})
	if err != nil && !adb.ResultsNotFound(err) {
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "background task creation",
			"cron":      agentDeployJobName,
			"impact":    "agents cannot start",
			"message":   "problem updating hosts with elapsed last communication time",
		}))
		return nil, errors.WithStack(err)
	}

	hosts, err := host.Find(ctx, host.ShouldDeployAgent())
	grip.Error(message.WrapError(err, message.Fields{
		"operation": "background task creation",
		"cron":      agentDeployJobName,
		"impact":    "agents cannot start",
		"message":   "problem finding hosts that need a new agent",
	}))
	if err != nil {
		return nil, errors.Wrap(err, "finding hosts that should be deployed new agents")
	}

	jobs := make([]amboy.Job, 0, len(hosts))
	for _, h := range hosts {
		jobs = append(jobs, NewAgentDeployJob(env, h, ts.Format(TSFormat)))
	}

	return jobs, nil
}

// agentMonitorDeployJobs enqueues the jobs to deploy the agent monitor
// to any host in which: (1) the agent monitor has not been deployed yet, (2)
// the agent's last communication time has exceeded the threshold or (3) has
// already been marked as needing to redeploy a new agent monitor.
func agentMonitorDeployJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}

	if flags.AgentStartDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "agent start disabled",
			"impact":  "agents are not deployed",
			"mode":    "degraded",
		})
		return nil, nil
	}

	// The agent monitor deploy job will atomically clear the
	// NeedsNewAgentMonitor field to prevent other jobs from running
	// concurrently.
	if err = host.UpdateAll(ctx, host.NeedsAgentMonitorDeploy(time.Now()), bson.M{"$set": bson.M{
		host.NeedsNewAgentMonitorKey: true,
	}}); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "background task creation",
			"cron":      agentMonitorDeployJobName,
			"impact":    "agent monitors cannot start",
			"message":   "problem updating hosts with elapsed last communication time",
		}))
		return nil, errors.WithStack(err)
	}

	hosts, err := host.Find(ctx, host.ShouldDeployAgentMonitor())
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "background task creation",
			"cron":      agentMonitorDeployJobName,
			"impact":    "agent monitors cannot start",
			"message":   "problem finding hosts that need a new agent",
		}))
		return nil, errors.Wrap(err, "finding hosts that should be deployed new agent monitors")
	}

	jobs := make([]amboy.Job, 0, len(hosts))
	for _, h := range hosts {
		jobs = append(jobs, NewAgentMonitorDeployJob(env, h, ts.Format(TSFormat)))
	}

	return jobs, nil
}

// enqueueFallbackGenerateTasksJobs populates generate.tasks jobs for tasks that have started running their generate.tasks command.
// Since the original generate.tasks request kicks off a job immediately, this function only serves as a fallback in case the
// original job fails to run. If the original job has already completed, the job will not be created here. If the original job is in flight,
// the job will be created here, but will no-op unless the original job fails to complete.
func enqueueFallbackGenerateTasksJobs(ctx context.Context, env evergreen.Environment, ts time.Time) error {
	tasks, err := task.GenerateNotRun(ctx)
	if err != nil {
		return errors.Wrap(err, "getting tasks that need generators run")
	}

	return CreateAndEnqueueGenerateTasks(ctx, env, tasks, ts.Format(TSFormat))
}

func hostCreationJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}

	if flags.HostInitDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "host init disabled",
			"impact":  "new hosts are not created in cloud providers",
			"mode":    "degraded",
		})
		return nil, nil
	}

	hosts, err := host.Find(ctx, host.IsUninitialized)
	if err != nil {
		return nil, errors.Wrap(err, "finding uninitialized hosts")
	}
	grip.Info(message.Fields{
		"message": "uninitialized hosts",
		"number":  len(hosts),
		"runner":  "hostinit",
		"source":  "PopulateHostCreationJobs",
	})

	var jobs []amboy.Job
	for _, h := range hosts {
		jobs = append(jobs, NewHostCreateJob(env, h, ts.Format(TSFormat), false))
	}
	return jobs, nil
}

func enqueueHostSetupJobs(ctx context.Context, env evergreen.Environment, queue amboy.Queue, ts time.Time) error {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "getting service flags")
	}

	if flags.HostInitDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "host init disabled",
			"impact":  "new hosts are not setup or provisioned",
			"mode":    "degraded",
		})
		return nil
	}

	hostInitSettings := env.Settings().HostInit
	if err = hostInitSettings.Get(ctx); err != nil {
		hostInitSettings = env.Settings().HostInit
	}

	hosts, err := host.FindByProvisioning(ctx)
	grip.Error(message.WrapError(err, message.Fields{
		"operation": "background host provisioning",
		"cron":      setupHostJobName,
		"impact":    "hosts cannot provision",
	}))
	if err != nil {
		return errors.Wrap(err, "finding hosts that need provisioning")
	}

	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].StartTime.Before(hosts[j].StartTime)
	})

	catcher := grip.NewBasicCatcher()
	collisions := 0
	jobsSubmitted := 0
	for _, h := range hosts {
		if !h.IsContainer() {
			if time.Since(h.StartTime) < 40*time.Second {
				// empirically no hosts are
				// ready in less than 40
				// seconds, so it doesn't seem
				// worth trying.
				continue
			}

			if jobsSubmitted >= hostInitSettings.ProvisioningThrottle {
				continue
			}
		}

		err := queue.Put(ctx, NewSetupHostJob(env, &h, ts.Format(TSFormat)))
		if amboy.IsDuplicateJobError(err) || amboy.IsDuplicateJobScopeError(err) {
			collisions++
			continue
		}
		catcher.Wrapf(err, "enqueueing host setup job for host '%s'", h.Id)

		jobsSubmitted++
	}

	grip.Info(message.Fields{
		"provisioning_hosts": len(hosts),
		"throttle":           hostInitSettings.ProvisioningThrottle,
		"jobs_submitted":     jobsSubmitted,
		"duplicates_seen":    collisions,
		"message":            "host provisioning setup",
	})

	return catcher.Resolve()
}

func hostReadyJob(ctx context.Context, _ evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}

	if flags.HostInitDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "host init disabled",
			"impact":  "new hosts are not setup or provisioned",
			"mode":    "degraded",
		})
		return nil, nil
	}

	return []amboy.Job{NewCloudHostReadyJob(nil, ts.Format(TSFormat))}, nil
}

// PopulateHostRestartJasperJobs enqueues the jobs to restart the Jasper service
// on the host.
func PopulateHostRestartJasperJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.HostInitDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "host init disabled",
				"impact":  "existing hosts are not provisioned with new Jasper services",
				"mode":    "degraded",
			})
			return nil
		}

		hosts, err := host.FindByNeedsToRestartJasper(ctx)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "Jasper service restart",
				"cron":      restartJasperJobName,
				"impact":    "existing hosts will not have their Jasper services restarted",
			}))
			return errors.Wrap(err, "finding hosts that need their Jasper service restarted")
		}

		catcher := grip.NewBasicCatcher()
		for _, h := range hosts {
			catcher.Wrapf(EnqueueHostReprovisioningJob(ctx, env, &h), "enqueueing Jasper restart job for host '%s'", h.Id)
		}

		return catcher.Resolve()
	}
}

// PopulateHostProvisioningConversionJobs enqueues the jobs to convert the host
// provisioning type.
func PopulateHostProvisioningConversionJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.HostInitDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "host init disabled",
				"impact":  "existing hosts that need to be converted to a different provisioning type are not being reprovisioned",
				"mode":    "degraded",
			})
			return nil
		}

		hosts, err := host.FindByShouldConvertProvisioning(ctx)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "reprovisioning hosts",
				"impact":    "existing hosts will not have their provisioning methods changed",
			}))
			return errors.Wrap(err, "finding hosts that need reprovisioning")
		}

		catcher := grip.NewBasicCatcher()
		for _, h := range hosts {
			catcher.Wrapf(EnqueueHostReprovisioningJob(ctx, env, &h), "enqueueing host reprovisioning job for host '%s'", h.Id)
		}

		return catcher.Resolve()
	}
}

func backgroundStatsJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"message":   "problem fetching service flags",
			"operation": "background stats",
		}))
		return nil, err
	}

	if flags.BackgroundStatsDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "background stats collection disabled",
			"impact":  "host, task, latency, amboy, and notification stats disabled",
			"mode":    "degraded",
		})
		return nil, nil
	}

	return []amboy.Job{
		NewRemoteAmboyStatsCollector(env, ts.Format(TSFormat)),
		NewHostStatsCollector(ts.Format(TSFormat)),
		NewTaskStatsCollector(ts.Format(TSFormat)),
		NewNotificationStatsCollector(ts.Format(TSFormat)),
		NewQueueStatsCollector(ts.Format(TSFormat)),
	}, nil
}

func periodicNotificationJobs(ctx context.Context, _ evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}
	if flags.AlertsDisabled {
		return nil, nil
	}

	return []amboy.Job{
		NewSpawnhostExpirationWarningsJob(ts.Format(TSFormat)),
		NewVolumeExpirationWarningsJob(ts.Format(TSFormat)),
		NewAlertableInstanceTypeNotifyJob(ts.Format(TSFormat)),
	}, nil
}

func PopulateCacheHistoricalTaskDataJob(part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}
		if flags.CacheStatsJobDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "cache historical task data job is disabled",
				"impact":  "pre-computed task stats are not updated",
				"mode":    "degraded",
			})
			return nil
		}

		projects, err := model.FindAllMergedTrackedProjectRefs(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		// Although we don't run this hourly, we still queue hourly to improve resiliency.
		ts := utility.RoundPartOfDay(part).Format(TSFormat)

		catcher := grip.NewBasicCatcher()
		for _, project := range projects {
			if !project.Enabled || project.IsStatsCacheDisabled() {
				continue
			}

			catcher.Wrapf(queue.Put(ctx, NewCacheHistoricalTaskDataJob(ts, project.Id)), "enqueueing cache historical task data job for project '%s'", project.Identifier)
		}

		return catcher.Resolve()
	}
}

func PopulateSpawnhostExpirationCheckJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		hosts, err := host.FindSpawnhostsWithNoExpirationToExtend(ctx)
		if err != nil {
			return err
		}

		catcher := grip.NewBasicCatcher()
		hostIds := []string{}
		for _, h := range hosts {
			hostIds = append(hostIds, h.Id)
			ts := utility.RoundPartOfHour(0).Format(TSFormat)
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewSpawnhostExpirationCheckJob(ts, &h)), "enqueueing spawn host expiration check job for host '%s'", h.Id)
		}
		grip.Info(message.Fields{
			"message": "extending spawn host expiration times",
			"hosts":   hostIds,
		})

		return errors.Wrap(catcher.Resolve(), "populating check spawn host expiration jobs")
	}
}

// PopulateCloudCleanupJob returns a QueueOperation to enqueue a CloudCleanup job for Fleet in the default EC2 region.
func PopulateCloudCleanupJob(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		return errors.Wrap(
			amboy.EnqueueUniqueJob(
				ctx,
				queue,
				NewCloudCleanupJob(env, utility.RoundPartOfHour(0).Format(TSFormat), evergreen.ProviderNameEc2Fleet, evergreen.DefaultEC2Region)),
			"enqueueing cloud cleanup job")
	}

}

func PopulateVolumeExpirationCheckJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		volumes, err := host.FindVolumesWithNoExpirationToExtend(ctx)
		if err != nil {
			return errors.Wrap(err, "finding expired volumes")
		}

		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		for i := range volumes {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewVolumeExpirationCheckJob(ts, &volumes[i], evergreen.ProviderNameEc2OnDemand)), "enqueueing volume expiration check job for volume '%s'", volumes[i].ID)
		}

		return errors.Wrap(catcher.Resolve(), "populating check volume expiration jobs")
	}
}

func PopulateVolumeExpirationJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		volumes, err := host.FindVolumesToDelete(ctx, time.Now())
		if err != nil {
			return errors.Wrap(err, "finding volumes to delete")
		}

		catcher := grip.NewBasicCatcher()
		for _, v := range volumes {
			ts := utility.RoundPartOfHour(0).Format(TSFormat)
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewVolumeDeletionJob(ts, &v)), "enqueueing volume deletion job for volume '%s'", v.ID)
		}

		return errors.Wrap(catcher.Resolve(), "populating expire volume jobs")
	}
}

// PopulateUnstickVolumesJob looks for volumes that are marked as attached to terminated hosts in our DB,
// and enqueues jobs to mark them unattached.
func PopulateUnstickVolumesJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		volumes, err := host.FindVolumesWithTerminatedHost(ctx)
		if err != nil {
			return errors.Wrap(err, "finding volumes to delete")

		}
		for _, v := range volumes {
			ts := utility.RoundPartOfHour(0).Format(TSFormat)
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewVolumeUnstickJob(ts, &v)), "enqueueing volume deletion job for volume '%s'", v.ID)
		}

		return errors.Wrap(catcher.Resolve(), "populating expire volume jobs")
	}
}

func PopulateLocalQueueJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(queue.Put(ctx, NewJasperManagerCleanup(utility.RoundPartOfMinute(0).Format(TSFormat), env)))
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			catcher.Wrap(err, "getting service flags")
			return catcher.Resolve()
		}

		if flags.BackgroundStatsDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "system stats",
				"impact":  "memory, cpu, runtime stats",
				"mode":    "degraded",
			})
			return nil
		}

		catcher.Wrap(queue.Put(ctx, NewSysInfoStatsCollector(fmt.Sprintf("sys-info-stats-%s", utility.RoundPartOfMinute(30).Format(TSFormat)))), "enqueueing system info stats job")
		catcher.Wrap(queue.Put(ctx, NewLocalAmboyStatsCollector(env, fmt.Sprintf("amboy-local-stats-%s", utility.RoundPartOfMinute(0).Format(TSFormat)))), "enqueueing Amboy local queue stats collector job")
		return catcher.Resolve()

	}
}

func PopulatePeriodicBuilds() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		projects, err := model.FindPeriodicProjects(ctx)
		if err != nil {
			return errors.Wrap(err, "finding periodic projects")
		}
		catcher := grip.NewBasicCatcher()
		for _, project := range projects {
			for _, definition := range project.PeriodicBuilds {
				// schedule the job if we want it to start before the next time this cron runs
				if time.Now().Add(15 * time.Minute).After(definition.NextRunTime) {
					catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewPeriodicBuildJob(project.Id, definition.ID)), "enqueueing periodic build job for project '%s' with periodic build definition '%s'", project.Id, definition.ID)
				}
			}
		}
		return errors.Wrap(catcher.Resolve(), "populating periodic build jobs")
	}
}

// userDataDoneJobs enqueues the jobs to check whether a spawn host
// provisioning with user data is done running its user data script yet.
func userDataDoneJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	hosts, err := host.FindUserDataSpawnHostsProvisioning(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding user data spawn hosts that are still provisioning")
	}
	var jobs []amboy.Job
	for _, h := range hosts {
		jobs = append(jobs, NewUserDataDoneJob(env, h.Id, ts))
	}
	return jobs, nil
}

func PopulateReauthorizeUserJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		if !env.UserManagerInfo().CanReauthorize {
			return nil
		}

		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}
		if flags.BackgroundReauthDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "background user reauth is disabled",
				"impact":  "users will need to manually log in again once their login session expires",
				"mode":    "degraded",
			})
			return nil
		}

		reauthAfter := time.Duration(env.Settings().AuthConfig.BackgroundReauthMinutes) * time.Minute
		if reauthAfter == 0 {
			reauthAfter = defaultBackgroundReauth
		}
		users, err := user.FindNeedsReauthorization(ctx, reauthAfter)
		if err != nil {
			return errors.Wrap(err, "finding users that need to reauthorize")
		}

		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		for _, user := range users {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewReauthorizeUserJob(env, &user, ts)), "enqueueing reauthorization job for user '%s'", user.Id)
		}

		return catcher.Resolve()
	}
}

func hostIPAssociationJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}

	if flags.ElasticIPsDisabled {
		return nil, nil
	}

	hosts, err := host.FindByNeedsIPAssociation(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding hosts that need to be associated with their IP address")
	}

	jobs := make([]amboy.Job, 0, len(hosts))
	for _, h := range hosts {
		jobs = append(jobs, NewHostIPAssociationJob(env, &h, ts.Format(TSFormat)))
	}
	return jobs, nil
}

// PopulateUnexpirableSpawnHostStatsJob populates jobs to collect statistics on
// unexpirable spawn host usage.
func PopulateUnexpirableSpawnHostStatsJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		return amboy.EnqueueUniqueJob(ctx, queue, NewUnexpirableSpawnHostStatsJob(utility.RoundPartOfHour(0).Format(TSFormat)))
	}
}

func sleepSchedulerJobs(ctx context.Context, env evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	return []amboy.Job{NewSleepSchedulerJob(env, ts.Format(TSFormat))}, nil
}

func PopulateDistroAutoTuneJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		distrosToAutoTune, err := distro.FindByCanAutoTune(ctx)
		if err != nil {
			return errors.Wrap(err, "finding distros that can be auto-tuned")
		}
		catcher := grip.NewBasicCatcher()
		for _, d := range distrosToAutoTune {
			ts := utility.RoundPartOfDay(0).Format(TSFormat)
			if err := amboy.EnqueueUniqueJob(ctx, queue, NewDistroAutoTuneJob(d.Id, ts)); err != nil {
				catcher.Wrapf(err, "enqueueing auto-tune job for distro '%s'", d.Id)
			}
		}
		return catcher.Resolve()
	}
}

func populateQueueGroup(ctx context.Context, env evergreen.Environment, queueGroupName string, factory cronJobFactory, ts time.Time) error {
	appCtx, _ := env.Context()
	queueGroup, err := env.RemoteQueueGroup().Get(appCtx, queueGroupName)
	if err != nil {
		return errors.Wrapf(err, "getting '%s' queue", queueGroupName)
	}
	jobs, err := factory(ctx, env, ts)
	if err != nil {
		return errors.Wrapf(err, "getting '%s' jobs", queueGroupName)
	}

	return errors.Wrapf(amboy.EnqueueManyUniqueJobs(ctx, queueGroup, jobs), "populating '%s' queue", queueGroupName)
}

// PopulateGithubAPILimitJob enqueues a job to log GitHub API rate limit
// information.
func PopulateGithubAPILimitJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		ts := utility.RoundPartOfMinute(0).Format(TSFormat)
		return amboy.EnqueueUniqueJob(ctx, queue, NewGitHubAPILimitJob(ts))
	}
}
