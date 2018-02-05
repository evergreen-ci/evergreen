package units

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const tsFormat = "2006-01-02.15-04-05"

func PopulateCatchupJobs() amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		adminSettings, err := admin.GetSettings()
		if err != nil {
			return errors.WithStack(err)
		}
		if adminSettings.ServiceFlags.RepotrackerDisabled {
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

		ts := util.RoundPartOfHour(30).Format(tsFormat)

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
				catcher.Add(queue.Put(NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), proj.Identifier)))
			}
		}

		return catcher.Resolve()
	}
}

func PopulateRepotrackerPollingJobs() amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		adminSettings, err := admin.GetSettings()
		if err != nil {
			return errors.WithStack(err)
		}

		if adminSettings.ServiceFlags.RepotrackerDisabled {
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

		ts := util.RoundPartOfHour(5).Format(tsFormat)

		catcher := grip.NewBasicCatcher()
		for _, proj := range projects {
			if !proj.Enabled || proj.TracksPushEvents {
				continue
			}

			catcher.Add(queue.Put(NewRepotrackerJob(fmt.Sprintf("polling-%s", ts), proj.Identifier)))
		}

		return catcher.Resolve()
	}
}

func PopulateActivationJobs() amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		adminSettings, err := admin.GetSettings()
		if err != nil {
			return errors.WithStack(err)
		}

		if adminSettings.ServiceFlags.TaskDispatchDisabled {
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

		ts := util.RoundPartOfHour(2).Format(tsFormat)

		catcher := grip.NewBasicCatcher()
		for _, proj := range projects {
			if !proj.Enabled {
				continue
			}

			catcher.Add(queue.Put(NewStepbackActiationJob(proj.Identifier, ts)))
		}

		return catcher.Resolve()
	}
}
