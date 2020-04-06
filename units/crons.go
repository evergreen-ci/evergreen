package units

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
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

		ts := utility.RoundPartOfHour(part).Format(TSFormat)

		catcher := grip.NewBasicCatcher()
		for _, proj := range projects {
			// only do catchup jobs for enabled projects
			// that track push events.
			if !proj.Enabled || proj.RepotrackerDisabled || !proj.TracksPushEvents {
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

		ts := utility.RoundPartOfHour(part).Format(TSFormat)

		catcher := grip.NewBasicCatcher()
		for _, proj := range projects {
			if !proj.Enabled || proj.RepotrackerDisabled || proj.TracksPushEvents {
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

		ts := utility.RoundPartOfHour(part).Format(TSFormat)

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

func PopulateEventAlertProcessing(env evergreen.Environment, parts int) amboy.QueueOperation {
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

		ts := utility.RoundPartOfHour(parts).Format(TSFormat)

		return errors.Wrap(queue.Put(ctx, NewEventMetaJob(env, queue, ts)), "failed to queue event-metajob")
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
			catcher.Add(queue.Put(ctx, NewHostTerminationJob(env, h, true, "host is expired, decommissioned, or failed to provision")))
		}

		hosts, err = host.AllHostsSpawnedByTasksToTerminate()
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "populate hosts spawned by tasks termination jobs",
			"cron":      hostTerminationJobName,
			"impact":    "hosts termination interrupted",
		}))
		catcher.Add(err)

		for _, h := range hosts {
			catcher.Add(queue.Put(ctx, NewHostTerminationJob(env, h, true, "host spawned by task has gone out of scope")))
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
		ts := utility.RoundPartOfHour(1).Format(TSFormat)
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
			missingDistroIDs := util.GetSetDifference(distroIDsToFind, distroIDsFound)
			hosts, err := host.Find(db.Query(host.ByDistroIDs(missingDistroIDs...)))
			if err != nil {
				return errors.Wrapf(err, "could not find hosts in missing distros: %s", strings.Join(missingDistroIDs, ", "))
			}
			for _, h := range hosts {
				catcher.Wrapf(h.SetDecommissioned(evergreen.User, "distro is missing"), "could not set host '%s' as decommissioned", h.Id)
			}
			if catcher.HasErrors() {
				return errors.Wrapf(catcher.Resolve(), "could not decommission hosts from missing distros: %s", strings.Join(missingDistroIDs, ", "))
			}
		}

		distrosMap := make(map[string]distro.Distro, len(distrosFound))
		for i := range distrosFound {
			d := distrosFound[i]
			distrosMap[d.Id] = d
		}

		for _, info := range distroHosts {
			totalRunningHosts := info.RunningHostsCount
			minimumHosts := distrosMap[info.DistroID].HostAllocatorSettings.MinimumHosts
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
		ts := utility.RoundPartOfHour(1).Format(TSFormat)

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
		distros, err := distro.Find(distro.ByNeedsPlanning(env.Settings().ContainerPools.Pools))
		if err != nil {
			return errors.WithStack(err)
		}

		ts := utility.RoundPartOfMinute(0)
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

		lastPlanned, err := model.FindTaskQueueLastGenerationTimes()
		catcher.Add(err)

		lastRuntime, err := model.FindTaskQueueGenerationRuntime()
		catcher.Add(err)

		// find all active distros
		distros, err := distro.Find(distro.ByNeedsPlanning(env.Settings().ContainerPools.Pools))
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
		settings := env.Settings()

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

		lastPlanned, err := model.FindTaskAliasQueueLastGenerationTimes()
		catcher.Add(err)

		// find all active distros
		distros, err := distro.Find(distro.ByNeedsPlanning(env.Settings().ContainerPools.Pools))
		catcher.Add(err)

		lastRuntime, err := model.FindTaskQueueGenerationRuntime()
		catcher.Add(err)

		settings := env.Settings()
		ts := utility.RoundPartOfMinute(0)

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

			catcher.Add(queue.Put(ctx, NewDistroAliasSchedulerJob(d.Id, ts)))
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

// PopulateHostAlertJobs adds alerting tasks infrequently for host
// utilization monitoring.
func PopulateHostAlertJobs(parts int) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()

		ts := utility.RoundPartOfHour(parts).Format(TSFormat)

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

		// 3x / minute
		ts := utility.RoundPartOfMinute(15).Format(TSFormat)
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

		// 3x / minute
		ts := utility.RoundPartOfMinute(20).Format(TSFormat)
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

		// refersh HostInit
		if err := env.Settings().HostInit.Get(env); err != nil {
			return errors.Wrap(err, "problem getting global config")
		}
		for _, h := range hosts {
			if h.UserHost || h.SpawnOptions.SpawnedByTask {
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
						continue
					}
				}
				// only increment for task hosts, since otherwise
				// spawn hosts and hosts spawned by tasks could
				// starve task hosts
				submitted++
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

		ts := utility.RoundPartOfMinute(30).Format(TSFormat)
		catcher := grip.NewBasicCatcher()
		for _, h := range hosts {
			catcher.Add(queue.Put(ctx, NewHostSetupJob(env, h, ts)))
		}

		catcher.Add(queue.Put(ctx, NewCloudHostReadyJob(env, ts)))

		return catcher.Resolve()
	}
}

// PopulateHostJasperRestartJobs enqueues the jobs to restart the Jasper service
// on the host.
func PopulateHostJasperRestartJobs(env evergreen.Environment) amboy.QueueOperation {
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

		if err = host.UpdateAll(host.NeedsReprovisioningLocked(time.Now()), bson.M{"$unset": bson.M{
			host.ReprovisioningLockedKey: false,
		}}); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "problem updating hosts with elapsed last communication time",
				"operation": "reprovisioning hosts",
				"impact":    "hosts cannot be reprovisioned",
			}))
			return errors.WithStack(err)
		}

		expiringHosts, err := host.FindByExpiringJasperCredentials(expirationCutoff)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "Jasper service restart",
				"cron":      jasperRestartJobName,
				"impact":    "existing hosts will not have their Jasper services restarted",
			}))
			return errors.Wrap(err, "problem finding hosts with expiring credentials")
		}
		catcher := grip.NewBasicCatcher()
		for _, h := range expiringHosts {
			if err = h.SetNeedsJasperRestart(evergreen.User); err != nil {
				catcher.Add(errors.Wrapf(err, "problem marking host as needing Jasper service restarted"))
				continue
			}
		}
		if catcher.HasErrors() {
			return errors.Wrap(catcher.Resolve(), "error updating hosts with expiring credentials")
		}

		hosts, err := host.FindByNeedsJasperRestart()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "Jasper service restart",
				"cron":      jasperRestartJobName,
				"impact":    "existing hosts will not have their Jasper services restarted",
			}))
			return errors.Wrap(err, "problem finding hosts that need their Jasper service restarted")
		}

		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		for _, h := range hosts {
			expiration, err := h.JasperCredentialsExpiration(ctx, env)
			if err != nil {
				catcher.Add(errors.Wrapf(err, "problem getting expiration time on credentials for host %s", h.Id))
				continue
			}

			catcher.Add(queue.Put(ctx, NewJasperRestartJob(env, h, expiration, h.Distro.BootstrapSettings.Communication == distro.CommunicationMethodRPC, ts, 0)))
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

		if err = host.UpdateAll(host.NeedsReprovisioningLocked(time.Now()), bson.M{"$unset": bson.M{
			host.ReprovisioningLockedKey: false,
		}}); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "problem updating hosts with elapsed last communication time",
				"operation": "reprovisioning hosts",
				"impact":    "hosts cannot be reprovisioned",
			}))
			return errors.WithStack(err)
		}

		hosts, err := host.FindByShouldConvertProvisioning()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"operation": "reprovisioning hosts",
				"impact":    "existing hosts will not have their provisioning methods changed",
			}))
			return errors.Wrap(err, "problem finding hosts that need reprovisioning")
		}

		ts := utility.RoundPartOfMinute(0).Format(TSFormat)
		catcher := grip.NewBasicCatcher()
		for _, h := range hosts {
			switch h.NeedsReprovision {
			case host.ReprovisionToLegacy:
				catcher.Add(queue.Put(ctx, NewConvertHostToLegacyProvisioningJob(env, h, ts, 0)))
			case host.ReprovisionToNew:
				catcher.Add(queue.Put(ctx, NewConvertHostToNewProvisioningJob(env, h, ts, 0)))
			}
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

		ts := utility.RoundPartOfDay(part).Format(TSFormat)

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

func PopulateSpawnhostExpirationCheckJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		hosts, err := host.FindSpawnhostsWithNoExpirationToExtend()
		if err != nil {
			return err
		}

		catcher := grip.NewBasicCatcher()
		for _, h := range hosts {
			ts := utility.RoundPartOfHour(0).Format(TSFormat)
			catcher.Add(queue.Put(ctx, NewSpawnhostExpirationCheckJob(ts, &h)))
		}

		return catcher.Resolve()
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

func PopulatePeriodicBuilds(part int) amboy.QueueOperation {
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
					catcher.Add(queue.Put(ctx, NewPeriodicBuildJob(project.Identifier, definition.ID, definition.NextRunTime)))
				}
			}
		}
		return catcher.Resolve()
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
		ts := utility.RoundPartOfMinute(15).Format(TSFormat)
		for _, h := range hosts {
			catcher.Add(queue.Put(ctx, NewUserDataDoneJob(env, h, ts)))
		}
		return catcher.Resolve()
	}
}

// PopulateSSHKeyUpdates updates the remote SSH keys in the cloud providers and
// static hosts.
func PopulateSSHKeyUpdates(env evergreen.Environment) amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		settings := env.Settings()

		allRegions := map[string]bool{}
		for _, key := range settings.Providers.AWS.EC2Keys {
			allRegions[key.Region] = true
		}
		// Enqueue jobs to update SSH keys in the cloud provider.
		updateRegions := map[string]bool{}
		for _, key := range settings.SSHKeyPairs {
			for region := range allRegions {
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

func PopulateReauthorizationJobs(env evergreen.Environment) amboy.QueueOperation {
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
				"message": "background reauth is disabled",
				"impact":  "users will reauth on page loads after periodic auth expiration",
				"mode":    "degraded",
			})
			return nil
		}

		reauthAfter := time.Duration(env.Settings().AuthConfig.BackgroundReauthMinutes) * time.Minute
		if reauthAfter == 0 {
			reauthAfter = defaultBackgroundReauth
		}
		users, err := user.FindNeedsReauthorization(reauthAfter, maxReauthAttempts)
		if err != nil {
			return err
		}

		catcher := grip.NewBasicCatcher()
		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		for _, user := range users {
			catcher.Wrap(queue.Put(ctx, NewReauthorizationJob(env, &user, ts)), "could not enqueue jobs to reauthorize users")
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
		if flags.BackgroundCleanup {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "background data cleanup",
				"impact":  "data will accumulate",
				"mode":    "degraded",
			})
			return nil
		}

		return queue.Put(ctx, NewTestResultsCleanupJob(util.RoundPartOfMinute(0)))

	}
}
