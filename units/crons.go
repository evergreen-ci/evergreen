package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const tsFormat = "2006-01-02.15-04-05"

func PopulateCatchupJobs(part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}
		if flags.RepotrackerDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "repotracker is disabled",
				"impact":  "catchup jobs disabled",
				"mode":    "degraded",
			})
			return nil
		}

		projects, err := model.FindAllTrackedProjectRefsWithRepoInfo()
		if err != nil {
			return errors.WithStack(err)
		}

		ts := util.RoundPartOfHour(part).Format(tsFormat)

		catcher := grip.NewBasicCatcher()
		for _, proj := range projects {
			// only do catchup jobs for enabled projects
			// that track push events.
			if !proj.Enabled || !proj.TracksPushEvents {
				continue
			}

			mostRecentVersion, err := model.VersionFindOne(model.VersionByMostRecentSystemRequester(proj.Identifier))
			catcher.Add(err)
			if mostRecentVersion == nil {
				grip.Warning(message.Fields{
					"project":   proj.Identifier,
					"operation": "repotracker catchup",
					"message":   "could not find a recent version for project, skipping catchup",
					"error":     err,
				})
				continue
			}

			if mostRecentVersion.CreateTime.Before(time.Now().Add(-2 * time.Hour)) {
				j := NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), proj.Identifier)
				j.SetPriority(-1)
				catcher.Add(queue.Put(ctx, j))
			}
		}

		return catcher.Resolve()
	}
}

func PopulateRepotrackerPollingJobs(part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.RepotrackerDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "repotracker is disabled",
				"impact":  "polling repos disabled",
				"mode":    "degraded",
			})
			return nil
		}

		projects, err := model.FindAllTrackedProjectRefsWithRepoInfo()
		if err != nil {
			return errors.WithStack(err)
		}

		ts := util.RoundPartOfHour(part).Format(tsFormat)

		catcher := grip.NewBasicCatcher()
		for _, proj := range projects {
			if !proj.Enabled || proj.TracksPushEvents {
				continue
			}

			j := NewRepotrackerJob(fmt.Sprintf("polling-%s", ts), proj.Identifier)
			j.SetPriority(-1)
			catcher.Add(queue.Put(ctx, j))
		}

		return catcher.Resolve()
	}
}

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

		projects, err := model.FindAllTrackedProjectRefs()
		if err != nil {
			return errors.WithStack(err)
		}

		ts := util.RoundPartOfHour(part).Format(tsFormat)

		catcher := grip.NewBasicCatcher()
		for _, proj := range projects {
			if !proj.Enabled {
				continue
			}

			catcher.Add(queue.Put(ctx, NewVersionActivationJob(proj.Identifier, ts)))
		}

		return catcher.Resolve()
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

		ts := util.RoundPartOfHour(2).Format(tsFormat)
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

func PopulateEventAlertProcessing(parts int) amboy.QueueOperation {
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

		ts := util.RoundPartOfHour(parts).Format(tsFormat)

		return errors.Wrap(queue.Put(ctx, NewEventMetaJob(queue, ts)), "failed to queue event-metajob")
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

		return queue.Put(ctx, NewTaskExecutionMonitorPopulateJob(util.RoundPartOfHour(mins).Format(tsFormat)))
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
			catcher.Add(queue.Put(ctx, NewHostTerminationJob(env, h, true)))
		}

		hosts, err = host.AllHostsSpawnedByTasksToTerminate()
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "populate hosts spawned by tasks termination jobs",
			"cron":      hostTerminationJobName,
			"impact":    "hosts termination interrupted",
		}))
		catcher.Add(err)

		for _, h := range hosts {
			catcher.Add(queue.Put(ctx, NewHostTerminationJob(env, h, true)))
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

		catcher := grip.NewBasicCatcher()
		ts := util.RoundPartOfHour(1).Format(tsFormat)
		// Each DistroID's idleHosts are sorted from oldest to newest CreationTime.
		distroHosts, err := host.IdleEphemeralGroupedByDistroID()
		if err != nil {
			return errors.Wrap(err, "database error grouping idle hosts by Distro.Id")
		}

		distroIDsToFind := make([]string, 0, len(distroHosts))
		for _, info := range distroHosts {
			distroIDsToFind = append(distroIDsToFind, info.DistroID)
		}
		distrosFound, err := distro.Find(distro.ByIds(distroIDsToFind))
		if err != nil {
			return errors.Wrapf(err, "database error for find() by distro ids in [%s]", strings.Join(distroIDsToFind, ","))
		}

		if len(distroIDsToFind) > len(distrosFound) {
			distroIDsFound := make([]string, 0, len(distrosFound))
			for _, d := range distrosFound {
				distroIDsFound = append(distroIDsFound, d.Id)
			}
			invalidDistroIDs := util.GetSetDifference(distroIDsToFind, distroIDsFound)
			return fmt.Errorf("distro ids %s not found", strings.Join(invalidDistroIDs, ","))
		}

		distrosMap := make(map[string]distro.Distro, len(distrosFound))
		for i := range distrosFound {
			d := distrosFound[i]
			distrosMap[d.Id] = d
		}

		for _, info := range distroHosts {
			totalRunningHosts := info.RunningHostsCount
			minimumHosts := distrosMap[info.DistroID].PlannerSettings.MinimumHosts
			nIdleHosts := len(info.IdleHosts)

			maxHostsToTerminate := totalRunningHosts - minimumHosts
			if maxHostsToTerminate <= 0 {
				continue
			}
			nHostsToEvaluateForTermination := nIdleHosts
			if nIdleHosts > maxHostsToTerminate {
				nHostsToEvaluateForTermination = maxHostsToTerminate
			}

			hostsToEvaluateForTermination := make([]host.Host, 0, nHostsToEvaluateForTermination)
			for j := 0; j < nHostsToEvaluateForTermination; j++ {
				hostsToEvaluateForTermination = append(hostsToEvaluateForTermination, info.IdleHosts[j])
				catcher.Add(queue.Put(ctx, NewIdleHostTerminationJob(env, info.IdleHosts[j], ts)))
			}

			grip.InfoWhen(sometimes.Percent(10), message.Fields{
				"id":                         idleHostJobName,
				"op":                         "dispatcher",
				"distro_id":                  info.DistroID,
				"minimum_hosts":              minimumHosts,
				"num_running_hosts":          totalRunningHosts,
				"num_idle_hosts":             nIdleHosts,
				"num_idle_hosts_to_evaluate": nHostsToEvaluateForTermination,
				"idle_hosts_to_evaluate":     hostsToEvaluateForTermination,
			})
		}

		return catcher.Resolve()
	}
}

func PopulateLastContainerFinishTimeJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := util.RoundPartOfHour(1).Format(tsFormat)
		err := queue.Put(ctx, NewLastContainerFinishTimeJob(ts))
		catcher.Add(err)

		return catcher.Resolve()
	}
}

func PopulateParentDecommissionJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := util.RoundPartOfHour(1).Format(tsFormat)

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
		ts := util.RoundPartOfHour(1).Format(tsFormat)

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
		ts := util.RoundPartOfHour(1).Format(tsFormat)

		parents, err := host.FindAllRunningParents()
		if err != nil {
			return errors.Wrap(err, "Error finding parent hosts")
		}

		// Create oldestImageJob when images take up too much disk space
		for _, p := range parents {
			catcher.Add(queue.Put(ctx, NewOldestImageRemovalJob(&p, evergreen.ProviderNameDocker, ts)))
		}
		return catcher.Resolve()
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
		ts := util.RoundPartOfHour(1).Format(tsFormat)

		projectRefs, err := model.FindProjectRefsWithCommitQueueEnabled()
		if err != nil {
			return errors.Wrap(err, "can't find projectRefs with Commit Queue enabled")
		}
		for _, p := range projectRefs {
			catcher.Add(queue.Put(ctx, NewCommitQueueJob(env, p.Identifier, ts)))
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

		// find all active distros
		distros, err := distro.Find(distro.ByActiveOrStatic())
		if err != nil {
			return errors.WithStack(err)
		}

		ts := util.RoundPartOfMinute(0)
		catcher := grip.NewBasicCatcher()

		for _, d := range distros {
			catcher.Add(queue.Put(ctx, NewHostAllocatorJob(env, d.Id, ts)))
		}

		return catcher.Resolve()
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

		catcher := grip.NewBasicCatcher()

		lastPlanned, err := model.FindTaskQueueGenerationTimes()
		catcher.Add(err)

		// find all active distros
		distros, err := distro.Find(distro.ByActiveOrStatic())
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

		ts := util.RoundPartOfMinute(20)
		settings := env.Settings()

		for _, d := range distros {
			// do not create scheduler jobs for parent distros
			if d.IsParent(settings) {
				continue
			}

			lastRun, ok := lastPlanned[d.Id]
			if ok && time.Since(lastRun) < 40*time.Second {
				continue
			}

			catcher.Add(queue.Put(ctx, NewDistroSchedulerJob(env, d.Id, ts)))
		}

		return catcher.Resolve()
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

		catcher := grip.NewBasicCatcher()

		lastPlanned, err := model.FindTaskAliasQueueGenerationTimes()
		catcher.Add(err)

		// find all active distros
		distros, err := distro.Find(distro.ByActiveOrStatic())
		catcher.Add(err)

		settings := env.Settings()
		ts := util.RoundPartOfMinute(30)

		for _, d := range distros {
			// do not create scheduler jobs for parent distros
			if d.IsParent(settings) {
				continue
			}

			lastRun, ok := lastPlanned[d.Id]
			if ok && time.Since(lastRun) < time.Minute {
				continue
			}

			catcher.Add(queue.Put(ctx, NewDistroAliasSchedulerJob(d.Id, ts)))
		}

		return catcher.Resolve()
	}
}

// PopulateHostAlertJobs adds alerting tasks infrequently for host
// utilization monitoring.
func PopulateHostAlertJobs(parts int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()

		ts := util.RoundPartOfHour(parts).Format(tsFormat)

		hosts, err := host.Find(host.IsRunningTask)
		grip.Warning(message.WrapError(err, message.Fields{
			"cron":      hostAlertingName,
			"operation": "background task creation",
			"impact":    "admin alerts not set",
		}))

		catcher.Add(err)
		if err == nil {
			for _, host := range hosts {
				catcher.Add(queue.Put(ctx, NewHostAlertingJob(host, ts)))
			}
		}

		catcher.Add(queue.Put(ctx, NewHostStatsJob(ts)))

		return catcher.Resolve()
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

		err = host.UpdateAll(host.AgentLastCommunicationTimeElapsed(time.Now()), bson.M{"$set": bson.M{
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

		hosts, err := host.Find(host.NeedsNewAgentFlagSet())
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "background task creation",
			"cron":      agentDeployJobName,
			"impact":    "agents cannot start",
			"message":   "problem finding hosts that need a new agent",
		}))
		if err != nil {
			return errors.WithStack(err)
		}

		// 3x / minute
		ts := util.RoundPartOfMinute(15).Format(tsFormat)
		catcher := grip.NewBasicCatcher()

		// For each host, set its last communication time to now and its needs new agent
		// flag to true. This ensures a consistent state in the agent-deploy job. That job
		// uses setting the NeedsNewAgent field to false to prevent other jobs from running
		// concurrently. If we didn't set one or the other of those fields, then
		for _, h := range hosts {
			catcher.Add(queue.Put(ctx, NewAgentDeployJob(env, h, ts)))
		}

		return catcher.Resolve()
	}

}

// PopulateGenerateTasksJobs poulates generate.tasks jobs for tasks that have started running their generate.tasks command.
func PopulateGenerateTasksJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, _ amboy.Queue) error {
		var q amboy.Queue
		var ok bool
		var err error

		catcher := grip.NewBasicCatcher()
		tasks, err := task.GenerateNotRun()
		if err != nil {
			return errors.Wrap(err, "problem getting tasks that need generators run")
		}

		versions := map[string]amboy.Queue{}

		ts := util.RoundPartOfMinute(15).Format(tsFormat)
		group := env.RemoteQueueGroup()
		for _, t := range tasks {
			if q, ok = versions[t.Version]; !ok {
				q, err = group.Get(context.TODO(), t.Version)
				if err != nil {
					return errors.Wrapf(err, "problem getting queue for version %s", t.Version)
				}
			}
			catcher.Add(q.Put(ctx, NewGenerateTasksJob(t.Id, ts)))
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
		if err = host.UpdateAll(host.AgentMonitorLastCommunicationTimeElapsed(time.Now()), bson.M{"$set": bson.M{
			host.NeedsNewAgentMonitorKey: true,
		}}); err != nil && !adb.ResultsNotFound(err) {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "background task creation",
				"cron":      agentMonitorDeployJobName,
				"impact":    "agent monitors cannot start",
				"message":   "problem updating hosts with elapsed last communication time",
			}))
			return errors.WithStack(err)
		}

		hosts, err := host.FindByNeedsNewAgentMonitor()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "background task creation",
				"cron":      agentMonitorDeployJobName,
				"impact":    "agent monitors cannot start",
				"message":   "problem finding hosts that need a new agent",
			}))
			return errors.WithStack(err)
		}

		// 3x / minute
		ts := util.RoundPartOfMinute(20).Format(tsFormat)
		catcher := grip.NewBasicCatcher()

		for _, h := range hosts {
			catcher.Add(queue.Put(ctx, NewAgentMonitorDeployJob(env, h, ts)))
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
		grip.Info(message.Fields{
			"message": "uninitialized hosts",
			"number":  len(hosts),
			"runner":  "hostinit",
		})
		if err != nil {
			return errors.Wrap(err, "error fetching uninitialized hosts")
		}
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "background task creation",
			"cron":      createHostJobName,
			"impact":    "hosts cannot start",
		}))
		if err != nil {
			return errors.WithStack(err)
		}

		ts := util.RoundPartOfHour(part).Format(tsFormat)
		catcher := grip.NewBasicCatcher()
		submitted := 0

		for _, h := range hosts {
			if h.UserHost {
				// pass:
				//    always start spawn hosts asap
			} else if submitted > 16 {
				// throttle hosts, so that we're starting very
				// few hosts on every pass. Hostinit runs very
				// frequently, lets not start too many all at
				// once.

				break
			}

			catcher.Add(queue.Put(ctx, NewHostCreateJob(env, h, ts, 1, 0, false)))
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

		hosts, err := host.FindByFirstProvisioningAttempt()
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "background host provisioning",
			"cron":      setupHostJobName,
			"impact":    "hosts cannot provision",
		}))
		if err != nil {
			return errors.Wrap(err, "error fetching provisioning hosts")
		}

		ts := util.RoundPartOfMinute(30).Format(tsFormat)
		catcher := grip.NewBasicCatcher()
		for _, h := range hosts {
			catcher.Add(queue.Put(ctx, NewHostSetupJob(env, h, ts)))
		}

		catcher.Add(queue.Put(ctx, NewCloudHostReadyJob(env, ts)))

		return catcher.Resolve()
	}
}

// PopulateJasperDeployJobs enqueues the jobs to deploy new Jasper service
// credentials to any host whose credentials are set to expire soon.
func PopulateJasperDeployJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.HostInitDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "host init disabled",
				"impact":  "existing hosts are not provisioned with new Jasper credentials, which may expire soon",
				"mode":    "degraded",
			})
			return nil
		}

		hosts, err := host.FindByExpiringJasperCredentials(expirationCutoff)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "Jasper service redeployment",
				"cron":      jasperDeployJobName,
				"impact":    "existing hosts are not provisioned with new Jasper credentials, which may expire soon",
			}))
			return errors.Wrap(err, "problem finding hosts with expiring credentials")
		}

		ts := util.RoundPartOfDay(0).Format(tsFormat)
		catcher := grip.NewBasicCatcher()
		for _, h := range hosts {
			if err := h.ResetJasperDeployAttempts(); err != nil {
				catcher.Add(errors.Wrapf(err, "problem resetting Jasper deploy attempts for host %s", h.Id))
				continue
			}
			expiration, err := h.JasperCredentialsExpiration(ctx, env)
			if err != nil {
				catcher.Add(errors.Wrapf(err, "problem getting expiration time on credentials for host %s", h.Id))
				continue
			}

			catcher.Add(queue.Put(ctx, NewJasperDeployJob(env, &h, expiration, h.Distro.BootstrapSettings.Communication == distro.CommunicationMethodRPC, ts)))
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
		ts := util.RoundPartOfMinute(part).Format(tsFormat)

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

		ts := util.RoundPartOfHour(parts).Format(tsFormat)
		catcher := grip.NewBasicCatcher()
		catcher.Add(queue.Put(ctx, NewSpawnhostExpirationWarningsJob(ts)))
		return catcher.Resolve()
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

		projects, err := model.FindAllTrackedProjectRefs()
		if err != nil {
			return errors.WithStack(err)
		}

		ts := util.RoundPartOfDay(part).Format(tsFormat)

		catcher := grip.NewBasicCatcher()
		for _, project := range projects {
			if !project.Enabled || project.DisabledStatsCache {
				continue
			}

			catcher.Add(queue.Put(ctx, NewCacheHistoricalTestDataJob(project.Identifier, ts)))
		}

		return catcher.Resolve()
	}
}

func PopulateLocalQueueJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(queue.Put(ctx, NewJasperManagerCleanup(util.RoundPartOfMinute(0).Format(tsFormat), env)))
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
				"message": "system stats ",
				"impact":  "memory, cpu, runtime stats",
				"mode":    "degraded",
			})
			return nil
		}

		catcher.Add(queue.Put(ctx, NewSysInfoStatsCollector(fmt.Sprintf("sys-info-stats-%s", util.RoundPartOfMinute(30).Format(tsFormat)))))
		catcher.Add(queue.Put(ctx, NewLocalAmboyStatsCollector(env, fmt.Sprintf("amboy-local-stats-%s", util.RoundPartOfMinute(0).Format(tsFormat)))))
		return catcher.Resolve()

	}
}

func PopulatePeriodicBuilds(part int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		projects, err := model.FindPeriodicProjects()
		if err != nil {
			return errors.Wrap(err, "error finding periodic projects")
		}
		catcher := grip.NewBasicCatcher()
		for _, project := range projects {
			catcher.Add(queue.Put(ctx, NewPeriodicBuildJob(project.Identifier, util.RoundPartOfMinute(30).Format(tsFormat))))
		}
		return nil
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
		ts := util.RoundPartOfMinute(15).Format(tsFormat)
		for _, h := range hosts {
			catcher.Add(queue.Put(ctx, NewUserDataDoneJob(env, h, ts)))
		}
		return catcher.Resolve()
	}
}
