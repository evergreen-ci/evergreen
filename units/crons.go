package units

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const tsFormat = "2006-01-02.15-04-05"

func PopulateCatchupJobs(part int) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
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

		projects, err := model.FindAllTrackedProjectRefs()
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

			mostRecentVersion, err := version.FindOne(version.ByMostRecentForRequester(proj.Identifier, evergreen.RepotrackerVersionRequester))
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
				catcher.Add(queue.Put(j))
			}
		}

		return catcher.Resolve()
	}
}

func PopulateRepotrackerPollingJobs(part int) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
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

		projects, err := model.FindAllTrackedProjectRefs()
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
			catcher.Add(queue.Put(j))
		}

		return catcher.Resolve()
	}
}

func PopulateActivationJobs(part int) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.TaskDispatchDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "task dispatching disabled",
				"mode":    "degraded",
				"impact":  "skipping stepback activation",
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

			catcher.Add(queue.Put(NewVersionActiationJob(proj.Identifier, ts)))
		}

		return catcher.Resolve()
	}
}

func PopulateHostMonitoring(env evergreen.Environment) amboy.QueueOperation {
	const reachabilityCheckInterval = 10 * time.Minute

	return func(queue amboy.Queue) error {
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
			catcher.Add(queue.Put(job))
		}

		return catcher.Resolve()
	}
}

func PopulateEventAlertProcessing(parts int) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.AlertsDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "alerts disabled",
				"impact":  "not processing alerts for notifications",
				"mode":    "degraded",
			})
			return nil
		}

		ts := util.RoundPartOfHour(parts).Format(tsFormat)

		return errors.Wrap(queue.Put(NewEventMetaJob(queue, ts)), "failed to queue event-metajob")
	}
}

func PopulateTaskMonitoring() amboy.QueueOperation {
	return func(queue amboy.Queue) error {
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

		ts := util.RoundPartOfHour(2).Format(tsFormat)

		return queue.Put(NewTaskExecutionMonitorJob(ts))
	}
}

func PopulateHostTerminationJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
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
			catcher.Add(queue.Put(NewHostTerminationJob(env, h)))
		}

		hosts, err = host.AllHostsSpawnedByTasksToTerminate()
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "populate hosts spawned by tasks termination jobs",
			"cron":      hostTerminationJobName,
			"impact":    "hosts termination interrupted",
		}))
		catcher.Add(err)

		for _, h := range hosts {
			catcher.Add(queue.Put(NewHostTerminationJob(env, h)))
		}

		return catcher.Resolve()
	}
}

func PopulateIdleHostJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
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
		hosts, err := host.AllIdleEphemeral()
		catcher.Add(err)
		grip.Warning(message.WrapError(err, message.Fields{
			"cron":      idleHostJobName,
			"operation": "background task creation",
			"hosts":     hosts,
			"impact":    "idle hosts termination",
		}))

		grip.InfoWhen(sometimes.Percent(10), message.Fields{
			"id":    idleHostJobName,
			"op":    "dispatcher",
			"hosts": hosts,
			"num":   len(hosts),
		})

		for _, h := range hosts {
			err := queue.Put(NewIdleHostTerminationJob(env, h, ts))
			catcher.Add(err)
		}

		return catcher.Resolve()
	}
}

func PopulateLastContainerFinishTimeJobs() amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := util.RoundPartOfHour(1).Format(tsFormat)
		err := queue.Put(NewLastContainerFinishTimeJob(ts))
		catcher.Add(err)

		return catcher.Resolve()
	}
}

func PopulateParentDecommissionJobs() amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := util.RoundPartOfHour(1).Format(tsFormat)

		settings, err := evergreen.GetConfig()
		if err != nil {
			return errors.Wrap(err, "Error finding evergreen settings")
		}
		containerPools := settings.ContainerPools.Pools

		// Create ParentDecommissionJob for each distro
		for _, c := range containerPools {
			d, err := distro.FindOne(distro.ById(c.Distro))
			if err != nil {
				return errors.Wrapf(err, "Could not find parent distro %s", c.Distro)
			}
			catcher.Add(queue.Put(NewParentDecommissionJob(ts, d, c.MaxContainers)))
		}

		return catcher.Resolve()
	}
}

func PopulateSchedulerJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
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
		}

		catcher := grip.NewBasicCatcher()

		lastPlanned, err := model.FindTaskQueueGenerationTimes()
		catcher.Add(err)

		names, err := distro.FindActive()
		catcher.Add(err)

		grip.InfoWhen(sometimes.Percent(10), message.Fields{
			"runner":   "scheduler",
			"previous": lastPlanned,
			"distros":  names,
			"op":       "dispatcher",
		})

		grip.Error(message.WrapError(err, message.Fields{
			"cron":      schedulerJobName,
			"impact":    "new task scheduling non-operative",
			"operation": "background task creation",
		}))

		ts := util.RoundPartOfMinute(20)
		for _, id := range names {
			lastRun, ok := lastPlanned[id]
			if ok && time.Since(lastRun) < 40*time.Second {
				continue
			}

			catcher.Add(queue.Put(NewDistroSchedulerJob(env, id, ts)))
		}

		return catcher.Resolve()
	}
}

// PopulateHostAlertJobs adds alerting tasks infrequently for host
// utilization monitoring.
func PopulateHostAlertJobs(parts int) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
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
				catcher.Add(queue.Put(NewHostAlertingJob(host, ts)))
			}
		}

		catcher.Add(queue.Put(NewHostStatsJob(ts)))

		return catcher.Resolve()
	}
}

func PopulateAgentDeployJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.TaskrunnerDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "taskrunner disabled",
				"impact":  "agents are not deployed",
				"mode":    "degraded",
			})
			return nil
		}

		hosts, err := host.Find(host.NeedsNewAgent(time.Now()))
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "background task creation",
			"cron":      agentDeployJobName,
			"impact":    "agents cannot start",
		}))
		if err != nil {
			return errors.WithStack(err)
		}

		// don't do this more than once a minute:
		ts := util.RoundPartOfMinute(30).Format(tsFormat)
		catcher := grip.NewBasicCatcher()

		for _, h := range hosts {
			catcher.Add(queue.Put(NewAgentDeployJob(env, h, ts)))
		}

		return catcher.Resolve()
	}

}

func PopulateHostCreationJobs(env evergreen.Environment, part int) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.HostinitDisabled {
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

			catcher.Add(queue.Put(NewHostCreateJob(env, h, ts)))
		}

		return catcher.Resolve()
	}
}

func PopulateHostSetupJobs(env evergreen.Environment, part int) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.HostinitDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "host init disabled",
				"impact":  "new hosts are not setup or provisioned",
				"mode":    "degraded",
			})
			return nil
		}

		hosts, err := host.Find(host.Provisioning())
		grip.Error(message.WrapError(err, message.Fields{
			"operation": "background task creation",
			"cron":      setupHostJobName,
			"impact":    "hosts cannot provision",
		}))
		if err != nil {
			return errors.Wrap(err, "error fetching provisioning hosts")
		}

		ts := util.RoundPartOfMinute(part).Format(tsFormat)
		catcher := grip.NewBasicCatcher()
		for _, h := range hosts {
			catcher.Add(queue.Put(NewHostSetupJob(env, h, ts)))
		}

		catcher.Add(queue.Put(NewCloudHostReadyJob(env, ts)))

		return catcher.Resolve()
	}
}

func PopulateBackgroundStatsJobs(env evergreen.Environment, part int) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
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

		catcher.Add(queue.Put(NewAmboyStatsCollector(env, ts)))
		catcher.Add(queue.Put(NewHostStatsCollector(ts)))
		catcher.Add(queue.Put(NewTaskStatsCollector(ts)))
		catcher.Add(queue.Put(NewLatencyStatsCollector(ts, time.Minute)))
		catcher.Add(queue.Put(NewNotificationStatsCollector(ts)))
		catcher.Add(queue.Put(NewQueueStatsCollector(ts)))

		return catcher.Resolve()
	}
}

func PopulateLegacyRunnerJobs(env evergreen.Environment, part int) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags()
		if err != nil {
			return errors.WithStack(err)
		}

		catcher := grip.NewBasicCatcher()
		ts := util.RoundPartOfHour(part).Format(tsFormat)

		if !flags.AlertsDisabled {
			catcher.Add(queue.Put(NewLegacyAlertsRunnerJob(env, ts)))
		}

		if !flags.MonitorDisabled {
			catcher.Add(queue.Put(NewLegacyMonitorRunnerJob(env, ts)))
		}

		return catcher.Resolve()
	}
}
