package data

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

type NotificationConnector struct{}

func (c *NotificationConnector) GetNotificationsStats() (*restModel.APINotificationStats, error) {
	stats := restModel.APINotificationStats{}

	e, err := event.FindLastProcessedEvent()
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch most recently processed event")
	}
	stats.LastProcessedAt = e.ProcessedAt

	n, err := event.CountUnprocessedEvents()
	if err != nil {
		return nil, errors.Wrap(err, "failed to count unprocessed events")
	}
	stats.NumUnprocessedEvents = n

	nStats, err := notification.CollectUnsentNotificationStats()
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect notification stats")
	}
	stats.PendingNotificationsByType = nStats

	return &stats, nil
}

type MockNotificationConnector struct{}

func (c *MockNotificationConnector) GetNotificationsStats() (*restModel.APINotificationStats, error) {
	return nil, errors.New("not implemented")
}
