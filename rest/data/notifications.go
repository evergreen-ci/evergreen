package data

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

func GetNotificationsStats() (*restModel.APIEventStats, error) {
	stats := restModel.APIEventStats{}

	e, err := event.FindLastProcessedEvent()
	if err != nil {
		return nil, errors.Wrap(err, "fetching most recently processed event")
	}
	if e != nil {
		stats.LastProcessedAt = &e.ProcessedAt
	}

	n, err := event.CountUnprocessedEvents()
	if err != nil {
		return nil, errors.Wrap(err, "counting unprocessed events")
	}
	stats.NumUnprocessedEvents = n

	nStats, err := notification.CollectUnsentNotificationStats()
	if err != nil {
		return nil, errors.Wrap(err, "collecting unsent notification stats")
	}
	if nStats != nil {
		stats.BuildFromService(*nStats)
	}
	return &stats, nil
}
