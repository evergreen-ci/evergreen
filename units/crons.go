package units

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
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
	"gopkg.in/mgo.v2/bson"
)

const TSFormat = "2006-01-02.15-04-05"

func PopulateActivationJobs(part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
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
			return errors.WithStack(err)
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
			catcher.Add(queue.Put(ctx, job))
		}

		catcher.Add(queue.Put(ctx, NewStrandedTaskCleanupJob(ts)))

		return catcher.Resolve()
	}
}

func PopulateEventAlertProcessing(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.EventProcessingDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "alerts disabled",
				"impact":  "not processing alerts for notifications",
				"mode":    "degraded",
			})
			return nil
		}

		err = dispatchUnprocessedNotifications(ctx, queue, flags)
		grip.Error(message.WrapError(err, message.Fields{
			"message": "unable to dispatch unprocessed notifications",
		}))
		notifySettings := env.Settings().Notify
		if err = notifySettings.Get(env); err != nil {
			return errors.Wrap(err, "unable to get notify settings")
		}
		limit := notifySettings.EventProcessingLimit
		if limit <= 0 {
			limit = evergreen.DefaultEventProcessingLimit
		}

		events, err := event.FindUnprocessedEvents(-1)
		if err != nil {
			return errors.Wrap(err, "unable to find unprocessed events")
		}
		grip.Info(message.Fields{
			"message": "unprocessed event count",
			"pending": len(events),
			"limit":   limit,
			"source":  "events-processing",
		})
		if len(events) == 0 {
			return nil
		}

		eventIDs := []string{}
		for i, evt := range events {
			eventIDs = append(eventIDs, evt.ID)
			if (i+1)%limit == 0 || i+1 == len(events) {
				err = queue.Put(ctx, NewEventNotifierJob(env, queue, sha256sum(eventIDs), eventIDs))
				grip.Error(message.WrapError(err, message.Fields{
					"message": "unable to queue event notifier job",
				}))
				eventIDs = []string{}
			}
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
			"cron":      hostTerminationJobName,
			"impact":    "hosts termination interrupted",
		}))
		catcher.Add(err)

		for _, h := range hosts {
			if h.NoExpiration && h.UserHost && h.Status != evergreen.HostProvisionFailed && time.Now().After(h.ExpirationTime) {
				grip.Error(message.Fields{
					"message": "attempting to terminate an expired host marked as non-expiring",
					"host":    h.Id,
				})
				continue
			}
			catcher.Add(amboy.EnqueueUniqueJob(ctx, queue, NewHostTerminationJob(env, &h, true, "host is expired, decommissioned, or failed to provision")))
		}

		hosts, err = host.AllHostsSpawnedByTasksToTerminate()
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "populate hosts spawned by tasks termination jobs",
			"cron":      hostTerminationJobName,
			"impact":    "hosts termination interrupted",
		}))
		catcher.Add(err)

		for _, h := range hosts {
			catcher.Add(amboy.EnqueueUniqueJob(ctx, queue, NewHostTerminationJob(env, &h, true, "host spawned by task has gone out of scope")))
		}

		return catcher.Resolve()
	}
}

func PopulateIdleHostJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
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
		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(1).Format(TSFormat)
		err := queue.Put(ctx, NewLastContainerFinishTimeJob(ts))
		catcher.Add(err)

		return catcher.Resolve()
	}
}

func PopulateParentDecommissionJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(1).Format(TSFormat)

		settings, err := evergreen.GetConfig()
		if err != nil {
			return errors.Wrap(err, "Error finding evergreen settings")
		}
		containerPools := settings.ContainerPools.Pools

		// Create ParentDecommissionJob for each distro
		for _, c := range containerPools {
			catcher.Add(queue.Put(ctx, NewParentDecommissionJob(ts, c.Distro, c.MaxContainers)))
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
			catcher.Add(queue.Put(ctx, NewHostMonitorContainerStateJob(env, &p, evergreen.ProviderNameDocker, ts)))
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
			return errors.Wrap(err, "Error finding parent hosts")
		}

		// Create oldestImageJob when images take up too much disk space
		for _, p := range parents {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewOldestImageRemovalJob(&p, evergreen.ProviderNameDocker, ts)), "parent host '%s'", p.Id)
		}
		return errors.Wrap(catcher.Resolve(), "populating remove oldest image jobs")
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

		projectRefs, err := model.FindProjectRefsWithCommitQueueEnabled()
		if err != nil {
			return errors.Wrap(err, "can't find projectRefs with Commit Queue enabled")
		}
		for _, p := range projectRefs {
			catcher.Add(queue.Put(ctx, NewCommitQueueJob(env, p.Id, ts)))
		}
		return catcher.Resolve()
	}
}

func PopulateHostAllocatorJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
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
		distros, err := distro.Find(distro.ByNeedsHostsPlanning(config.ContainerPools.Pools))
		if err != nil {
			return errors.WithStack(err)
		}

		ts := utility.RoundPartOfMinute(0)
		catcher := grip.NewBasicCatcher()

		for _, d := range distros {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewHostAllocatorJob(env, d.Id, ts)), "distro '%s'", d.Id)
		}

		return errors.Wrap(catcher.Resolve(), "populating host allocator jobs")
	}
}

func PopulateSchedulerJobs(env evergreen.Environment) amboy.QueueOperation {
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

		lastPlanned, err := model.FindTaskQueueLastGenerationTimes()
		catcher.Add(err)

		lastRuntime, err := model.FindTaskQueueGenerationRuntime()
		catcher.Add(err)

		// find all active distros
		distros, err := distro.Find(distro.ByNeedsPlanning(config.ContainerPools.Pools))
		catcher.Add(err)

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
			// do not create scheduler jobs for parent distros
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

			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewDistroSchedulerJob(env, d.Id, ts)), "distro '%s'", d.Id)
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

		lastPlanned, err := model.FindTaskAliasQueueLastGenerationTimes()
		catcher.Add(err)

		// find all active distros
		distros, err := distro.Find(distro.ByNeedsPlanning(config.ContainerPools.Pools))
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

			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewDistroAliasSchedulerJob(d.Id, ts)), "distro 's'", d.Id)
		}

		return errors.Wrap(catcher.Resolve(), "populating distro alias scheduler jobs")
	}
}

func PopulateCheckUnmarkedBlockedTasks() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
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
			return errors.WithStack(err)
		}

		catcher := grip.NewBasicCatcher()
		// find all active distros
		distros, err := distro.Find(distro.ByNeedsPlanning(config.ContainerPools.Pools))
		catcher.Add(err)

		ts := utility.RoundPartOfMinute(0)
		for _, d := range distros {
			catcher.Add(queue.Put(ctx, NewCheckBlockedTasksJob(d.Id, ts)))
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
			return errors.WithStack(err)
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
			return errors.WithStack(err)
		}

		ts := utility.RoundPartOfMinute(15).Format(TSFormat)
		catcher := grip.NewBasicCatcher()

		// For each host, set its last communication time to now and its needs new agent
		// flag to true. This ensures a consistent state in the agent-deploy job. That job
		// uses setting the NeedsNewAgent field to false to prevent other jobs from running
		// concurrently. If we didn't set one or the other of those fields, then
		for _, h := range hosts {
			catcher.Add(amboy.EnqueueUniqueJob(ctx, queue, NewAgentDeployJob(env, h, ts)))
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
			return errors.WithStack(err)
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
			return errors.WithStack(err)
		}

		ts := utility.RoundPartOfMinute(15).Format(TSFormat)
		catcher := grip.NewBasicCatcher()

		for _, h := range hosts {
			catcher.Add(amboy.EnqueueUniqueJob(ctx, queue, NewAgentMonitorDeployJob(env, h, ts)))
		}

		return catcher.Resolve()
	}

}

// PopulateGenerateTasksJobs populates generate.tasks jobs for tasks that have started running their generate.tasks command.
func PopulateGenerateTasksJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(_ context.Context, _ amboy.Queue) error {
		ctx := context.Background()
		var q amboy.Queue
		var ok bool
		var err error

		catcher := grip.NewBasicCatcher()
		tasks, err := task.GenerateNotRun()
		if err != nil {
			return errors.Wrap(err, "problem getting tasks that need generators run")
		}

		versions := map[string]amboy.Queue{}

		ts := utility.RoundPartOfHour(1).Format(TSFormat)
		group := env.RemoteQueueGroup()
		for _, t := range tasks {
			if q, ok = versions[t.Version]; !ok {
				q, err = group.Get(ctx, t.Version)
				if err != nil {
					return errors.Wrapf(err, "problem getting queue for version %s", t.Version)
				}
				versions[t.Version] = q
			}
			catcher.Add(q.Put(ctx, NewGenerateTasksJob(t.Id, ts)))
		}
		return catcher.Resolve()
	}
}

func PopulateHostCreationJobs(env evergreen.Environment, part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
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
			return errors.Wrap(err, "error fetching uninitialized hosts")
		}
		grip.Info(message.Fields{
			"message": "uninitialized hosts",
			"number":  len(hosts),
			"runner":  "hostinit",
		})

		runningHosts, err := host.Find(db.Query(host.IsLive()))
		if err != nil {
			return errors.Wrap(err, "problem getting running hosts")
		}
		runningDistroCount := make(map[string]int)
		for _, h := range runningHosts {
			runningDistroCount[h.Distro.Id] += 1
		}

		ts := utility.RoundPartOfHour(part).Format(TSFormat)
		catcher := grip.NewBasicCatcher()
		submitted := 0

		// shuffle hosts because hosts will always come off the index in insertion order
		// and we want all of them to get an equal chance at being created on this pass
		// (Fisher-Yates shuffle)
		rand.Seed(time.Now().UnixNano())
		for i := len(hosts) - 1; i > 0; i-- {
			j := rand.Intn(i + 1)
			hosts[i], hosts[j] = hosts[j], hosts[i]
		}

		// refresh HostInit
		if err := env.Settings().HostInit.Get(env); err != nil {
			return errors.Wrap(err, "problem getting global config")
		}
		throttleCount := 0
		for _, h := range hosts {
			if !h.IsSubjectToHostCreationThrottle() {
				// pass:
				//    always start spawn hosts asap

			} else {
				num := runningDistroCount[h.Distro.Id]
				if num == 0 || num < len(runningHosts)/100 {
					// if there aren't many of these hosts up, start them even
					// if `submitted` exceeds the throttle, but increment each
					// time so we only create hosts up to the threshold
					runningDistroCount[h.Distro.Id] += 1
				} else {
					if submitted > env.Settings().HostInit.HostThrottle {
						// throttle hosts, so that we're starting very
						// few hosts on every pass. Hostinit runs very
						// frequently, lets not start too many all at
						// once.
						throttleCount++
						continue
					}
				}
				// only increment for task hosts, since otherwise
				// spawn hosts and hosts spawned by tasks could
				// starve task hosts
				submitted++
			}

			catcher.Add(queue.Put(ctx, NewHostCreateJob(env, h, ts, 0, false)))
		}

		if throttleCount > 0 {
			grip.Info(message.Fields{
				"message":           "host creation rate was throttled",
				"hosts_not_created": throttleCount,
			})
		}

		return catcher.Resolve()
	}
}

func PopulateHostSetupJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
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
			return errors.Wrap(err, "error fetching provisioning hosts")
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
			catcher.Add(err)

			jobsSubmitted++
		}

		grip.Info(message.Fields{
			"provisioning_hosts": len(hosts),
			"throttle":           hostInitSettings.ProvisioningThrottle,
			"jobs_submitted":     jobsSubmitted,
			"duplicates_seen":    collisions,
			"message":            "host provisioning setup",
		})

		catcher.Add(queue.Put(ctx, NewCloudHostReadyJob(env, utility.RoundPartOfMinute(30).Format(TSFormat))))

		return catcher.Resolve()
	}
}

// PopulateHostRestartJasperJobs enqueues the jobs to restart the Jasper service
// on the host.
func PopulateHostRestartJasperJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
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
			return errors.Wrap(err, "problem finding hosts that need their Jasper service restarted")
		}

		catcher := grip.NewBasicCatcher()
		for _, h := range hosts {
			catcher.Wrapf(EnqueueHostReprovisioningJob(ctx, env, &h), "host '%s'", h.Id)
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
			return errors.Wrap(err, "problem finding hosts that need reprovisioning")
		}

		catcher := grip.NewBasicCatcher()
		for _, h := range hosts {
			catcher.Wrapf(EnqueueHostReprovisioningJob(ctx, env, &h), "host '%s'", h.Id)
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
				"impact":  "host, task, latency, amboy, and notification stats disabled",
				"mode":    "degraded",
			})
			return nil
		}

		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfMinute(part).Format(TSFormat)

		catcher.Add(queue.Put(ctx, NewRemoteAmboyStatsCollector(env, ts)))
		catcher.Add(queue.Put(ctx, NewHostStatsCollector(ts)))
		catcher.Add(queue.Put(ctx, NewTaskStatsCollector(ts)))
		catcher.Add(queue.Put(ctx, NewLatencyStatsCollector(ts, time.Minute)))
		catcher.Add(queue.Put(ctx, NewNotificationStatsCollector(ts)))
		catcher.Add(queue.Put(ctx, NewQueueStatsCollector(ts)))

		return catcher.Resolve()
	}
}

func PopulatePeriodicNotificationJobs(parts int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}
		if flags.AlertsDisabled {
			return nil
		}

		ts := utility.RoundPartOfHour(parts).Format(TSFormat)
		catcher := grip.NewBasicCatcher()
		catcher.Add(amboy.EnqueueUniqueJob(ctx, queue, NewSpawnhostExpirationWarningsJob(ts)))
		catcher.Add(amboy.EnqueueUniqueJob(ctx, queue, NewVolumeExpirationWarningsJob(ts)))
		return errors.Wrap(catcher.Resolve(), "populating periodic notifications")
	}
}

func PopulateCacheHistoricalTestDataJob(part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}
		if flags.CacheStatsJobDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "cache stats job is disabled",
				"impact":  "pre-computed test and task stats are not updated",
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
			if !project.IsEnabled() || project.IsStatsCacheDisabled() {
				continue
			}

			catcher.Add(queue.Put(ctx, NewCacheHistoricalTestDataJob(project.Id, ts)))
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
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewSpawnhostExpirationCheckJob(ts, &h)), "host '%s'", h.Id)
		}
		grip.Info(message.Fields{
			"message": "extending spawn host expiration times",
			"hosts":   hostIds,
		})

		return errors.Wrap(catcher.Resolve(), "populating check spawn host expiration jobs")
	}
}

func PopulateVolumeExpirationCheckJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		volumes, err := host.FindVolumesWithNoExpirationToExtend()
		if err != nil {
			return err
		}

		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		for i := range volumes {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewVolumeExpirationCheckJob(ts, &volumes[i], evergreen.ProviderNameEc2OnDemand)), "volume '%s'", volumes[i].ID)
		}

		return errors.Wrap(catcher.Resolve(), "populating check volume expiration jobs")
	}
}

func PopulateVolumeExpirationJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		volumes, err := host.FindVolumesToDelete(time.Now())
		if err != nil {
			return errors.Wrap(err, "can't get volumes to delete")
		}

		catcher := grip.NewBasicCatcher()
		for _, v := range volumes {
			ts := utility.RoundPartOfHour(0).Format(TSFormat)
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewVolumeDeletionJob(ts, &v)), "volume '%s'", v.ID)
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
			grip.Alert(message.WrapError(err, message.Fields{
				"message":   "problem fetching service flags",
				"operation": "system stats",
			}))
			catcher.Add(err)
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

		catcher.Add(queue.Put(ctx, NewSysInfoStatsCollector(fmt.Sprintf("sys-info-stats-%s", utility.RoundPartOfMinute(30).Format(TSFormat)))))
		catcher.Add(queue.Put(ctx, NewLocalAmboyStatsCollector(env, fmt.Sprintf("amboy-local-stats-%s", utility.RoundPartOfMinute(0).Format(TSFormat)))))
		return catcher.Resolve()

	}
}

func PopulatePeriodicBuilds() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		projects, err := model.FindPeriodicProjects()
		if err != nil {
			return errors.Wrap(err, "error finding periodic projects")
		}
		catcher := grip.NewBasicCatcher()
		for _, project := range projects {
			for _, definition := range project.PeriodicBuilds {
				// schedule the job if we want it to start before the next time this cron runs
				if time.Now().Add(15 * time.Minute).After(definition.NextRunTime) {
					catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewPeriodicBuildJob(project.Id, definition.ID)), "project '%s' with periodic build definition '%s'", project.Id, definition.ID)
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
			return errors.Wrap(err, "error finding user data hosts that are still provisioning")
		}
		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(1)
		for _, h := range hosts {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewUserDataDoneJob(env, h.Id, ts)), "host '%s'", h.Id)
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
			catcher.Wrap(queue.Put(ctx, NewCloudUpdateSSHKeysJob(evergreen.ProviderNameEc2Fleet, region, ts)), "could not enqueue jobs to update cloud provider SSH keys")
		}

		// Enqueue jobs to update authorized keys on static hosts.
		hosts, err := host.FindStaticNeedsNewSSHKeys(settings)
		if err != nil {
			catcher.Wrap(err, "could not find hosts that need to update their SSH keys")
			return catcher.Resolve()
		}
		for _, h := range hosts {
			catcher.Wrap(queue.Put(ctx, NewStaticUpdateSSHKeysJob(h, ts)), "could not enqueue jobs to update static host SSH keys")
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
			return errors.WithStack(err)
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
			return err
		}

		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		for _, user := range users {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewReauthorizeUserJob(env, &user, ts)), "user %s", user.Id)
		}

		return catcher.Resolve()
	}
}

func PopulateDataCleanupJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}
		if flags.BackgroundCleanupDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "background data cleanup",
				"impact":  "data will accumulate",
				"mode":    "degraded",
			})
			return nil
		}

		catcher := grip.NewBasicCatcher()
		catcher.Add(queue.Put(ctx, NewTestResultsCleanupJob(utility.RoundPartOfMinute(2))))
		catcher.Add(queue.Put(ctx, NewTestLogsCleanupJob(utility.RoundPartOfMinute(2))))

		return catcher.Resolve()
	}
}

func PopulatePodCreationJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		pods, err := pod.FindByInitializing()
		if err != nil {
			return errors.Wrap(err, "error fetching initializing pods")
		}

		catcher := grip.NewBasicCatcher()
		for _, p := range pods {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewPodCreationJob(p.ID, utility.RoundPartOfMinute(0).Format(TSFormat))), "enqueueing job to create pod %s", p.ID)
		}

		return catcher.Resolve()
	}
}

func PopulatePodTerminationJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		pods, err := pod.FindByNeedsTermination()
		if err != nil {
			return errors.Wrap(err, "finding pods that have been stuck starting for too long")
		}

		catcher := grip.NewBasicCatcher()
		for _, p := range pods {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewPodTerminationJob(p.ID, "pod has been stuck starting for too long", utility.RoundPartOfMinute(0))), "pod '%s'", p.ID)
		}
		return catcher.Resolve()
	}
}

// dispatchUnprocessedNotifications gets unprocessed notifications
// leftover by previous runs and dispatches them
func dispatchUnprocessedNotifications(ctx context.Context, q amboy.Queue, flags *evergreen.ServiceFlags) error {
	unprocessedNotifications, err := notification.FindUnprocessed()
	if err != nil {
		return errors.Wrap(err, "can't find unprocessed notifications")
	}

	return dispatchNotifications(ctx, unprocessedNotifications, q, flags)
}
