package units

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const tsFormat = "2006-01-02.15-04-05"

func PopulateCatchupJobs() amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		projects, err := model.FindAllTrackedProjectRefs()
		if err != nil {
			return errors.WithStack(err)
		}

		adminSettings, err := admin.GetSettings()
		if err != nil {
			return errors.WithStack(err)
		}
		if adminSettings.ServiceFlags.RepotrackerDisabled {
			grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
				"message": "repotracker is disabled",
			})
			return nil
		}

		now := time.Now().Add(-time.Minute).UTC()
		mins := now.Minute()
		if mins > 30 {
			mins = 30
		} else {
			mins = 0
		}

		ts := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), mins, 0, 0, time.UTC).Format(tsFormat)

		catcher := grip.NewBasicCatcher()
		for _, proj := range projects {
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

func PopulateActivationJobs(env evergreen.Environment) amboy.QueueOperation {
	return func(queue amboy.Queue) error {
		projects, err := model.FindAllTrackedProjectRefs()
		if err != nil {
			return errors.WithStack(err)
		}

		now := time.Now().Add(-time.Minute).UTC()
		ts := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, time.UTC).Format(tsFormat)

		catcher := grip.NewBasicCatcher()
		for _, proj := range projects {
			if !proj.Enabled {
				continue
			}

			catcher.Add(queue.Put(NewStepbackActiationJob(env, proj.Identifier, ts)))
		}

		return catcher.Resolve()
	}
}
