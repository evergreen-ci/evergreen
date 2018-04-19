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

func EventMetaJobQueueOperation() amboy.QueueOperation {
	return func(q amboy.Queue) error {
		t := time.Now().Truncate(EventProcessingInterval / 2)
		err := q.Put(NewEventMetaJob(q, t.Format(tsFormat)))

		return errors.Wrap(err, "failed to queue event-metajob")
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
		j := NewTaskExecutionMonitorJob(ts)

		return queue.Put(j)
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
			"id":    idleHostJobName,
			"hosts": hosts,
		}))

		grip.InfoWhen(sometimes.Percent(10), message.Fields{
			"id":    idleHostJobName,
			"op":    "dispatcher",
			"hosts": hosts,
			"num":   len(hosts),
		})

		for _, h := range hosts {
			err := queue.Put(NewIdleHostTerminationJob(env, h, ts))
			grip.Warning(message.WrapError(err, message.Fields{
				"id":   idleHostJobName,
				"host": h,
			}))
			catcher.Add(err)
		}

		return catcher.Resolve()
	}
}

func PopulateSchedulerJobs() amboy.QueueOperation {
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

		names, err := distro.FindAllNames()
		catcher.Add(err)

		grip.InfoWhen(sometimes.Percent(10), message.Fields{
			"runner":   "scheduler",
			"previous": lastPlanned,
			"distros":  names,
			"op":       "dispatcher",
		})

		ts := util.RoundPartOfMinute(20)
		for _, id := range names {
			lastRun, ok := lastPlanned[id]
			if ok && time.Since(lastRun) < 40*time.Second {
				continue
			}

			job := NewDistroSchedulerJob(id, ts)
			catcher.Add(queue.Put(job))
		}

		return catcher.Resolve()
	}
}

// used for infrequently running system alerts
func PopulateAlertingJobs() amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(addHostLongRunningTaskJobs(queue))
		catcher.Add(addHostStatsJob(queue))

		return catcher.Resolve()
	}
}

func addHostLongRunningTaskJobs(queue amboy.Queue) error {
	hosts, err := host.Find(host.IsRunningTask)
	if err != nil {
		return errors.WithStack(err)
	}

	ts := util.RoundPartOfHour(2).Format(tsFormat)
	catcher := grip.NewBasicCatcher()

	for _, host := range hosts {
		job := NewHostAlertingJob(host, ts)
		catcher.Add(queue.Put(job))
	}

	return catcher.Resolve()
}

func addHostStatsJob(queue amboy.Queue) error {
	ts := util.RoundPartOfHour(2).Format(tsFormat)
	job := NewHostStatsJob(ts)
	return queue.Put(job)
}
