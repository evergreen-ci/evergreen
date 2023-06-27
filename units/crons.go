package units

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	TSFormat = "2006-01-02.15-04-05"

	// CreateHostQueueGroup is the queue group for the provisioning-create-host job.
	CreateHostQueueGroup            = "service.host.create"
	commitQueueQueueGroup           = "service.commitqueue"
	eventNotifierQueueGroup         = "service.event.notifier"
	podAllocationQueueGroup         = "service.pod.allocate"
	podDefinitionCreationQueueGroup = "service.pod.definition.create"
	podCreationQueueGroup           = "service.pod.create"
)

func PopulateActivationJobs(part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
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

func PopulateHostMonitoring(env evergreen.Environment) amboy.QueueOperation {
	const reachabilityCheckInterval = 10 * time.Minute

	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.MonitorDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "monitor is disabled",
				"impact":  "not detecting externally terminated hosts",
				"mode":    "degraded",
			})
			return nil
		}

		threshold := time.Now().Add(-reachabilityCheckInterval)
		hosts, err := host.Find(host.ByNotMonitoredSince(threshold))
		if err != nil {
			return errors.WithStack(err)
		}

		ts := utility.RoundPartOfHour(2).Format(TSFormat)
		catcher := grip.NewBasicCatcher()

		grip.InfoWhen(len(hosts) > 0, message.Fields{
			"runner":    "monitor",
			"operation": "host reachability monitor",
			"num_hosts": len(hosts),
		})

		for _, host := range hosts {
			job := NewHostMonitorExternalStateJob(env, &host, ts)
			catcher.Wrapf(queue.Put(ctx, job), "enqueueing host monitor external state job for host '%s'", host.Id)
		}

		catcher.Wrap(queue.Put(ctx, NewStrandedTaskCleanupJob(ts)), "enqueueing stranded task cleanup job")

		return catcher.Resolve()
	}
}

// PopulatePodHealthCheckJobs enqueues the jobs to check pods that have not
// checked in recently to determine if they are still healthy.
func PopulatePodHealthCheckJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.MonitorDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "monitor is disabled",
				"impact":  "not detecting pods that have not communicated recently",
				"mode":    "degraded",
			})
			return nil
		}

		pods, err := pod.FindByLastCommunicatedBefore(time.Now().Add(-podReachabilityThreshold))
		if err != nil {
			return errors.Wrap(err, "finding pods that have not communicated recently")
		}

		catcher := grip.NewBasicCatcher()
		for _, p := range pods {
			j := NewPodHealthCheckJob(p.ID, utility.RoundPartOfMinute(0))
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, j), "enqueueing pod health check job for pod '%s'", p.ID)
		}

		return catcher.Resolve()
	}
}

func PopulateEventSendJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.EventProcessingDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "notifications disabled",
				"impact":  "not processing alerts for notifications",
				"mode":    "degraded",
			})
			return nil
		}

		return dispatchUnprocessedNotifications(ctx, queue, flags)
	}
}

func PopulateEventNotifierJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.EventProcessingDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "event processing disabled",
				"impact":  "not processing events",
				"mode":    "degraded",
			})
			return nil
		}

		events, err := event.FindUnprocessedEvents(-1)
		if err != nil {
			return errors.Wrap(err, "finding all unprocessed events")
		}
		grip.Info(message.Fields{
			"message": "unprocessed event count",
			"pending": len(events),
			"source":  "events-processing",
		})

		catcher := grip.NewBasicCatcher()
		for _, evt := range events {
			if err := amboy.EnqueueUniqueJob(ctx, queue, NewEventNotifierJob(env, env.RemoteQueue(), evt.ID, utility.RoundPartOfMinute(0).Format(TSFormat))); err != nil {
				catcher.Wrapf(err, "enqueueing event notifier job for event '%s'", evt.ID)
			}
		}

		if catcher.HasErrors() {
			return message.WrapError(catcher.Resolve(), message.Fields{
				"message":  "unable to queue event notifier jobs",
				"count":    len(catcher.Errors()),
				"job_type": eventNotifierName,
			})
		}

		return nil
	}
}

func PopulateTaskMonitoring(mins int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
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

func PopulateHostTerminationJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.MonitorDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "monitor is disabled",
				"impact":  "not submitting termination flags for dead/killable hosts",
				"mode":    "degraded",
			})
			return nil
		}

		catcher := grip.NewBasicCatcher()
		hosts, err := host.FindHostsToTerminate()
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "populate host termination jobs",
			"cron":      HostTerminationJobName,
			"impact":    "hosts termination interrupted",
		}))
		catcher.Wrap(err, "finding hosts to terminate")

		for _, h := range hosts {
			if h.NoExpiration && h.UserHost && h.Status != evergreen.HostProvisionFailed && time.Now().After(h.ExpirationTime) {
				grip.Error(message.Fields{
					"message": "attempting to terminate an expired host marked as non-expiring",
					"host":    h.Id,
				})
				continue
			}
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewHostTerminationJob(env, &h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "host is expired, decommissioned, or failed to provision",
			})), "enqueueing termination job for host '%s'", h.Id)
		}

		hosts, err = host.AllHostsSpawnedByTasksToTerminate()
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "populate hosts spawned by tasks termination jobs",
			"cron":      HostTerminationJobName,
			"impact":    "hosts termination interrupted",
		}))
		catcher.Wrap(err, "finding hosts spawned by tasks to terminate")

		for _, h := range hosts {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewHostTerminationJob(env, &h, HostTerminationOptions{
				TerminateIfBusy:   true,
				TerminationReason: "host spawned by task has gone out of scope",
			})), "enqueueing termination job for host '%s'", h.Id)
		}

		return catcher.Resolve()
	}
}

func PopulateIdleHostJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.MonitorDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "monitor is disabled",
				"impact":  "not submitting detecting idle hosts",
				"mode":    "degraded",
			})
			return nil
		}

		return queue.Put(ctx, NewIdleHostTerminationJob(env, utility.RoundPartOfHour(1).Format(TSFormat)))
	}
}

func PopulateLastContainerFinishTimeJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		ts := utility.RoundPartOfHour(1).Format(TSFormat)
		return queue.Put(ctx, NewLastContainerFinishTimeJob(ts))
	}
}

func PopulateParentDecommissionJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(1).Format(TSFormat)

		settings, err := evergreen.GetConfig()
		if err != nil {
			return errors.Wrap(err, "getting admin settings")
		}
		containerPools := settings.ContainerPools.Pools

		// Create ParentDecommissionJob for each distro
		for _, c := range containerPools {
			catcher.Wrapf(queue.Put(ctx, NewParentDecommissionJob(ts, c.Distro, c.MaxContainers)), "enqueueing parent decommission job for distro '%s'", c.Distro)
		}

		return catcher.Resolve()
	}
}

func PopulateContainerStateJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(1).Format(TSFormat)

		parents, err := host.FindAllRunningParents()
		if err != nil {
			return errors.Wrap(err, "Error finding parent hosts")
		}

		// create job to check container state consistency for each parent
		for _, p := range parents {
			catcher.Wrapf(queue.Put(ctx, NewHostMonitorContainerStateJob(env, &p, evergreen.ProviderNameDocker, ts)), "enqueueing host monitor container state job for parent '%s'", p.Id)
		}

		return catcher.Resolve()
	}
}

func PopulateOldestImageRemovalJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(1).Format(TSFormat)

		parents, err := host.FindAllRunningParents()
		if err != nil {
			return errors.Wrap(err, "finding all parent hosts")
		}

		// Create oldestImageJob when images take up too much disk space
		for _, p := range parents {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewOldestImageRemovalJob(&p, evergreen.ProviderNameDocker, ts)), "enqueueing oldest image removal job for parent host '%s'", p.Id)
		}
		return errors.Wrap(catcher.Resolve(), "populating oldest image removal jobs")
	}
}

func PopulateCommitQueueJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.CommitQueueDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "commit queue is disabled",
				"impact":  "commit queue items are not processed",
				"mode":    "degraded",
			})
			return nil
		}

		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(1).Format(TSFormat)

		projectIds, err := model.FindProjectRefIdsWithCommitQueueEnabled()
		if err != nil {
			return errors.Wrap(err, "finding project refs with commit queue enabled")
		}
		for _, id := range projectIds {
			catcher.Wrapf(queue.Put(ctx, NewCommitQueueJob(env, id, ts)), "enqueueing commit queue job for project '%s'", id)
		}
		return catcher.Resolve()
	}
}

func PopulateHostAllocatorJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.HostAllocatorDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "host allocation is disabled",
				"impact":  "new hosts cannot be allocated",
				"mode":    "degraded",
			})
			return nil
		}

		config, err := evergreen.GetConfig()
		if err != nil {
			return errors.WithStack(err)
		}
		// find all active distros
		distros, err := distro.FindWithContext(ctx, distro.ByNeedsHostsPlanning(config.ContainerPools.Pools))
		if err != nil {
			return errors.Wrap(err, "finding distros that need planning")
		}

		ts := utility.RoundPartOfMinute(0)
		catcher := grip.NewBasicCatcher()

		for _, d := range distros {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewHostAllocatorJob(env, d.Id, ts)), "enqueueing host allocator job for distro '%s'", d.Id)
		}

		return errors.Wrap(catcher.Resolve(), "populating host allocator jobs")
	}
}

func PopulateSchedulerJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.SchedulerDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "scheduler is disabled",
				"impact":  "new tasks are not enqueued",
				"mode":    "degraded",
			})
			return nil
		}
		config, err := evergreen.GetConfig()
		if err != nil {
			return errors.Wrap(err, "getting admin settings")
		}

		catcher := grip.NewBasicCatcher()

		lastPlanned, err := model.FindTaskQueueLastGenerationTimes()
		catcher.Wrap(err, "finding task queue last generation time")

		lastRuntime, err := model.FindTaskQueueGenerationRuntime()
		catcher.Wrap(err, "finding task queue generation runtime")

		// find all active distros
		distros, err := distro.FindWithContext(ctx, distro.ByNeedsPlanning(config.ContainerPools.Pools))
		catcher.Wrap(err, "finding distros that need planning")

		grip.InfoWhen(sometimes.Percent(10), message.Fields{
			"runner":   "scheduler",
			"previous": lastPlanned,
			"distros":  distro.DistroGroup(distros).GetDistroIds(),
			"op":       "dispatcher",
		})

		grip.Error(message.WrapError(err, message.Fields{
			"cron":      schedulerJobName,
			"impact":    "new task scheduling non-operative",
			"operation": "background task creation",
		}))

		ts := utility.RoundPartOfMinute(0)
		settings, err := evergreen.GetConfig()
		if err != nil {
			grip.Critical(message.WrapError(err, message.Fields{
				"cron":      schedulerJobName,
				"operation": "retrieving settings object",
			}))
			return catcher.Resolve()
		}
		for _, d := range distros {
			if d.IsParent(settings) {
				continue
			}

			lastRun, ok := lastPlanned[d.Id]
			if ok && time.Since(lastRun) < 2*time.Minute {
				continue
			}

			if ok && time.Since(lastRun)+10*time.Second < lastRuntime[d.Id] {
				continue
			}

			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewDistroSchedulerJob(env, d.Id, ts)), "enqueueing scheduler job for distro '%s'", d.Id)
		}

		return errors.Wrap(catcher.Resolve(), "populating distro scheduler jobs")
	}
}

func PopulateAliasSchedulerJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.SchedulerDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "scheduler is disabled",
				"impact":  "new tasks are not enqueued",
				"mode":    "degraded",
			})
			return nil
		}
		config, err := evergreen.GetConfig()
		if err != nil {
			return errors.WithStack(err)
		}
		catcher := grip.NewBasicCatcher()

		lastPlanned, err := model.FindTaskSecondaryQueueLastGenerationTimes()
		catcher.Add(err)

		// find all active distros
		distros, err := distro.FindWithContext(ctx, distro.ByNeedsPlanning(config.ContainerPools.Pools))
		catcher.Add(err)

		lastRuntime, err := model.FindTaskQueueGenerationRuntime()
		catcher.Add(err)

		ts := utility.RoundPartOfMinute(0)
		for _, d := range distros {
			// do not create scheduler jobs for parent distros
			if d.IsParent(config) {
				continue
			}

			lastRun, ok := lastPlanned[d.Id]
			if ok && time.Since(lastRun) < 2*time.Minute {
				continue
			}
			if ok && time.Since(lastRun)+10*time.Second < lastRuntime[d.Id] {
				continue
			}

			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewDistroAliasSchedulerJob(d.Id, ts)), "enqueueing secondary scheduler for distro '%s'", d.Id)
		}

		return errors.Wrap(catcher.Resolve(), "populating distro secondary scheduler jobs")
	}
}

func PopulateCheckUnmarkedBlockedTasks() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
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

		config, err := evergreen.GetConfig()
		if err != nil {
			return errors.Wrap(err, "getting admin settings")
		}

		catcher := grip.NewBasicCatcher()
		// find all active distros
		distros, err := distro.FindWithContext(ctx, distro.ByNeedsPlanning(config.ContainerPools.Pools))
		catcher.Wrap(err, "getting distros that need planning")

		ts := utility.RoundPartOfMinute(0)
		for _, d := range distros {
			catcher.Wrapf(queue.Put(ctx, NewCheckBlockedTasksJob(d.Id, ts)), "enqueueing check blocked tasks job for distro '%s'", d.Id)
		}
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

func PopulateAgentDeployJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.AgentStartDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "agent start disabled",
				"impact":  "agents are not deployed",
				"mode":    "degraded",
			})
			return nil
		}

		err = host.UpdateAll(host.NeedsAgentDeploy(time.Now()), bson.M{"$set": bson.M{
			host.NeedsNewAgentKey: true,
		}})
		if err != nil && !adb.ResultsNotFound(err) {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "background task creation",
				"cron":      agentDeployJobName,
				"impact":    "agents cannot start",
				"message":   "problem updating hosts with elapsed last communication time",
			}))
			return errors.WithStack(err)
		}

		hosts, err := host.Find(host.ShouldDeployAgent())
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "background task creation",
			"cron":      agentDeployJobName,
			"impact":    "agents cannot start",
			"message":   "problem finding hosts that need a new agent",
		}))
		if err != nil {
			return errors.Wrap(err, "finding hosts that should be deployed new agents")
		}

		ts := utility.RoundPartOfMinute(15).Format(TSFormat)
		catcher := grip.NewBasicCatcher()

		// For each host, set its last communication time to now and its needs new agent
		// flag to true. This ensures a consistent state in the agent-deploy job. That job
		// uses setting the NeedsNewAgent field to false to prevent other jobs from running
		// concurrently. If we didn't set one or the other of those fields, then
		for _, h := range hosts {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewAgentDeployJob(env, h, ts)), "enqueueing agent deploy job for host '%s'", h.Id)
		}

		return catcher.Resolve()
	}

}

// PopulateAgentMonitorDeployJobs enqueues the jobs to deploy the agent monitor
// to any host in which: (1) the agent monitor has not been deployed yet, (2)
// the agent's last communication time has exceeded the threshold or (3) has
// already been marked as needing to redeploy a new agent monitor.
func PopulateAgentMonitorDeployJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.AgentStartDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "agent start disabled",
				"impact":  "agents are not deployed",
				"mode":    "degraded",
			})
			return nil
		}

		// The agent monitor deploy job will atomically clear the
		// NeedsNewAgentMonitor field to prevent other jobs from running
		// concurrently.
		if err = host.UpdateAll(host.NeedsAgentMonitorDeploy(time.Now()), bson.M{"$set": bson.M{
			host.NeedsNewAgentMonitorKey: true,
		}}); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "background task creation",
				"cron":      agentMonitorDeployJobName,
				"impact":    "agent monitors cannot start",
				"message":   "problem updating hosts with elapsed last communication time",
			}))
			return errors.WithStack(err)
		}

		hosts, err := host.Find(host.ShouldDeployAgentMonitor())
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "background task creation",
				"cron":      agentMonitorDeployJobName,
				"impact":    "agent monitors cannot start",
				"message":   "problem finding hosts that need a new agent",
			}))
			return errors.Wrap(err, "finding hosts that should be deployed new agent monitors")
		}

		ts := utility.RoundPartOfMinute(15).Format(TSFormat)
		catcher := grip.NewBasicCatcher()

		for _, h := range hosts {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewAgentMonitorDeployJob(env, h, ts)), "enqueueing agent monitor deploy job for host '%s'", h.Id)
		}

		return catcher.Resolve()
	}

}

// PopulateGenerateTasksJobs populates generate.tasks jobs for tasks that have started running their generate.tasks command.
func PopulateGenerateTasksJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(_ context.Context, _ amboy.Queue) error {
		ctx := context.Background()

		catcher := grip.NewBasicCatcher()
		tasks, err := task.GenerateNotRun()
		if err != nil {
			return errors.Wrap(err, "getting tasks that need generators run")
		}

		ts := utility.RoundPartOfHour(1).Format(TSFormat)
		for _, t := range tasks {
			queueName := fmt.Sprintf("service.generate.tasks.version.%s", t.Version)
			queue, err := env.RemoteQueueGroup().Get(ctx, queueName)
			if err != nil {
				catcher.Wrapf(err, "getting generate tasks queue '%s' for version '%s'", queueName, t.Version)
				continue
			}
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewGenerateTasksJob(t.Version, t.Id, ts)), "enqueueing generate tasks job for task '%s'", t.Id)
		}

		return catcher.Resolve()
	}
}

func PopulateHostCreationJobs(env evergreen.Environment, part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.HostInitDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "host init disabled",
				"impact":  "new hosts are not created in cloud providers",
				"mode":    "degraded",
			})
			return nil
		}

		hosts, err := host.Find(host.IsUninitialized)
		if err != nil {
			return errors.Wrap(err, "finding uninitialized hosts")
		}
		grip.Info(message.Fields{
			"message": "uninitialized hosts",
			"number":  len(hosts),
			"runner":  "hostinit",
			"source":  "PopulateHostCreationJobs",
		})

		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(part).Format(TSFormat)
		for _, h := range hosts {
			catcher.Wrapf(queue.Put(ctx, NewHostCreateJob(env, h, ts, 0, false)), "enqueueing host creation job for host '%s'", h.Id)
		}

		return catcher.Resolve()
	}
}

func PopulateHostSetupJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
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
		if err = hostInitSettings.Get(env); err != nil {
			hostInitSettings = env.Settings().HostInit
		}

		hosts, err := host.FindByProvisioning()
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

		jobsSubmitted := 0
		collisions := 0
		catcher := grip.NewBasicCatcher()
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

			err := queue.Put(ctx, NewSetupHostJob(env, &h, utility.RoundPartOfMinute(0).Format(TSFormat)))
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

		catcher.Wrap(queue.Put(ctx, NewCloudHostReadyJob(env, utility.RoundPartOfMinute(30).Format(TSFormat))), "enqueueing cloud host ready job")

		return catcher.Resolve()
	}
}

// PopulateHostRestartJasperJobs enqueues the jobs to restart the Jasper service
// on the host.
func PopulateHostRestartJasperJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
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

		hosts, err := host.FindByNeedsToRestartJasper()
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
		flags, err := evergreen.GetServiceFlags()
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

		hosts, err := host.FindByShouldConvertProvisioning()
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

func PopulateBackgroundStatsJobs(env evergreen.Environment, part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			grip.Alert(message.WrapError(err, message.Fields{
				"message":   "problem fetching service flags",
				"operation": "background stats",
			}))
			return err
		}

		if flags.BackgroundStatsDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "background stats collection disabled",
				"impact":  "host, pod, task, latency, amboy, and notification stats disabled",
				"mode":    "degraded",
			})
			return nil
		}

		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfMinute(part).Format(TSFormat)

		catcher.Wrap(queue.Put(ctx, NewRemoteAmboyStatsCollector(env, ts)), "enqueueing remote Amboy stats collector job")
		catcher.Wrap(queue.Put(ctx, NewHostStatsCollector(ts)), "enqueueing host stats collector job")
		catcher.Wrap(queue.Put(ctx, NewPodStatsCollector(ts)), "enqueueing pod stats collector job")
		catcher.Wrap(queue.Put(ctx, NewTaskStatsCollector(ts)), "enqueueing task stats collector job")
		catcher.Wrap(queue.Put(ctx, NewNotificationStatsCollector(ts)), "enqueueing notification stats collector job")
		catcher.Wrap(queue.Put(ctx, NewQueueStatsCollector(ts)), "enqueueing task queue stats collector job")

		return catcher.Resolve()
	}
}

func PopulatePeriodicNotificationJobs(parts int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}
		if flags.AlertsDisabled {
			return nil
		}

		ts := utility.RoundPartOfHour(parts).Format(TSFormat)
		catcher := grip.NewBasicCatcher()
		catcher.Wrap(amboy.EnqueueUniqueJob(ctx, queue, NewSpawnhostExpirationWarningsJob(ts)), "enqueueing spawn host expiration warning job")
		catcher.Wrap(amboy.EnqueueUniqueJob(ctx, queue, NewVolumeExpirationWarningsJob(ts)), "enqueueing volume expiration warning job")
		return errors.Wrap(catcher.Resolve(), "populating periodic notifications")
	}
}

func PopulateCacheHistoricalTaskDataJob(part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
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

		projects, err := model.FindAllMergedTrackedProjectRefs()
		if err != nil {
			return errors.WithStack(err)
		}

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
		hosts, err := host.FindSpawnhostsWithNoExpirationToExtend()
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
		volumes, err := host.FindVolumesWithNoExpirationToExtend()
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
		volumes, err := host.FindVolumesToDelete(time.Now())
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

func PopulateLocalQueueJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(queue.Put(ctx, NewJasperManagerCleanup(utility.RoundPartOfMinute(0).Format(TSFormat), env)))
		flags, err := evergreen.GetServiceFlags()
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
		projects, err := model.FindPeriodicProjects()
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

// PopulateUserDataDoneJobs enqueues the jobs to check whether a spawn host
// provisioning with user data is done running its user data script yet.
func PopulateUserDataDoneJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		hosts, err := host.FindUserDataSpawnHostsProvisioning()
		if err != nil {
			return errors.Wrap(err, "finding user data spawn hosts that are still provisioning")
		}
		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(1)
		for _, h := range hosts {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewUserDataDoneJob(env, h.Id, ts)), "enqueueing user data done job for host '%s'", h.Id)
		}
		return errors.Wrap(catcher.Resolve(), "populating check user data done jobs")
	}
}

// PopulateSSHKeyUpdates updates the remote SSH keys in the cloud providers and
// static hosts.
func PopulateSSHKeyUpdates(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		settings := env.Settings()

		allRegions := settings.Providers.AWS.AllowedRegions
		// Enqueue jobs to update SSH keys in the cloud provider.
		updateRegions := map[string]bool{}
		for _, key := range settings.SSHKeyPairs {
			for _, region := range allRegions {
				if utility.StringSliceContains(key.EC2Regions, region) {
					continue
				}
				updateRegions[region] = true
			}
		}
		for region := range updateRegions {
			catcher.Wrapf(queue.Put(ctx, NewCloudUpdateSSHKeysJob(evergreen.ProviderNameEc2Fleet, region, ts)), "enqueueing jobs to update SSH keys for EC2 region '%s'", region)
		}

		// Enqueue jobs to update authorized keys on static hosts.
		hosts, err := host.FindStaticNeedsNewSSHKeys(settings)
		if err != nil {
			catcher.Wrap(err, "finding static hosts that need to update their SSH keys")
			return catcher.Resolve()
		}
		for _, h := range hosts {
			catcher.Wrapf(queue.Put(ctx, NewStaticUpdateSSHKeysJob(h, ts)), "enqueueing jobs to update SSH keys for static host '%s'", h.Id)
		}

		return catcher.Resolve()
	}
}

func PopulateReauthorizeUserJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		if !env.UserManagerInfo().CanReauthorize {
			return nil
		}

		flags, err := evergreen.GetServiceFlags()
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
		users, err := user.FindNeedsReauthorization(reauthAfter)
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

// PopulatePodAllocatorJobs returns the queue operation to enqueue jobs to
// allocate pods to tasks and disable container tasks that exceed the stale
// undispatched threshold.
func PopulatePodAllocatorJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}

		if flags.PodAllocatorDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "pod allocation disabled",
				"impact":  "container tasks will not be allocated any pods to run them",
				"mode":    "degraded",
			})
			return nil
		}

		if err := model.DisableStaleContainerTasks(evergreen.StaleContainerTaskMonitor); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not disable stale container tasks",
				"context": "pod allocation",
			}))
		}

		numInitializing, err := pod.CountByInitializing()
		if err != nil {
			return errors.Wrap(err, "counting initializing pods")
		}

		grip.Info(message.Fields{
			"message":          "tracking statistics for number of parallel pods being requested",
			"num_initializing": numInitializing,
		})

		settings, err := evergreen.GetConfig()
		if err != nil {
			return errors.Wrap(err, "getting admin settings")
		}
		remaining := settings.PodLifecycle.MaxParallelPodRequests - numInitializing

		ctq, err := model.NewContainerTaskQueue()
		if err != nil {
			return errors.Wrap(err, "getting container task queue")
		}

		catcher := grip.NewBasicCatcher()

		for ctq.HasNext() && remaining > 0 {
			t := ctq.Next()
			if t == nil {
				break
			}

			j := NewPodAllocatorJob(t.Id, utility.RoundPartOfMinute(0).Format(TSFormat))
			if err := amboy.EnqueueUniqueJob(ctx, queue, j); err != nil {
				catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, j), "enqueueing pod allocator job for task '%s'", t.Id)
				continue
			}

			remaining--
		}

		grip.InfoWhen(remaining <= 0 && ctq.Len() > 0, message.Fields{
			"message":             "reached max parallel pod request limit, not allocating any more",
			"included_on":         evergreen.ContainerHealthDashboard,
			"context":             "pod allocation",
			"num_remaining_tasks": ctq.Len(),
		})

		return catcher.Resolve()
	}
}

func PopulatePodCreationJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		pods, err := pod.FindByInitializing()
		if err != nil {
			return errors.Wrap(err, "finding initializing pods")
		}

		catcher := grip.NewBasicCatcher()
		for _, p := range pods {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewPodCreationJob(p.ID, utility.RoundPartOfMinute(0).Format(TSFormat))), "enqueueing pod creation job for pod '%s'", p.ID)
		}

		return catcher.Resolve()
	}
}

func PopulatePodTerminationJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		pods, err := pod.FindByNeedsTermination()
		if err != nil {
			return errors.Wrap(err, "finding pods that need to be terminated")
		}

		catcher := grip.NewBasicCatcher()
		for _, p := range pods {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewPodTerminationJob(p.ID, "system indicates pod should be terminated", utility.RoundPartOfMinute(0))), "enqueueing pod termination job for pod '%s'", p.ID)
		}
		return catcher.Resolve()
	}
}

// PopulatePodDefinitionCreationJobs populates the jobs to create pod
// definitions.
func PopulatePodDefinitionCreationJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		pods, err := pod.FindByInitializing()
		if err != nil {
			return errors.Wrap(err, "finding initializing pods")
		}

		catcher := grip.NewBasicCatcher()
		for _, p := range pods {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewPodDefinitionCreationJob(env.Settings().Providers.AWS.Pod.ECS, p.TaskContainerCreationOpts, utility.RoundPartOfMinute(0).Format(TSFormat))), "pod '%s'", p.ID)
		}

		return errors.Wrap(catcher.Resolve(), "enqueueing pod definition creation jobs")
	}
}

// PopulatePodResourceCleanupJobs populates the jobs to clean up pod
// resources.
func PopulatePodResourceCleanupJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		catcher.Wrap(amboy.EnqueueUniqueJob(ctx, queue, NewPodDefinitionCleanupJob(utility.RoundPartOfHour(0).Format(TSFormat))), "enqueueing pod definition cleanup job")
		catcher.Wrap(amboy.EnqueueUniqueJob(ctx, queue, NewContainerSecretCleanupJob(utility.RoundPartOfHour(0).Format(TSFormat))), "enqueueing container secret cleanup job")
		return catcher.Resolve()
	}
}

// dispatchUnprocessedNotifications gets unprocessed notifications
// leftover by previous runs and dispatches them
func dispatchUnprocessedNotifications(ctx context.Context, q amboy.Queue, flags *evergreen.ServiceFlags) error {
	unprocessedNotifications, err := notification.FindUnprocessed()
	if err != nil {
		return errors.Wrap(err, "finding unprocessed notifications")
	}

	return dispatchNotifications(ctx, unprocessedNotifications, q, flags)
}
