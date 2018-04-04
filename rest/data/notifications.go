package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/rest"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

type NotificationConnector struct{}

func (c *NotificationConnector) GetNotificationsStats() (*restModel.APIEventStats, error) {
	stats := restModel.APIEventStats{}

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

	if err = stats.BuildFromService(nStats); err != nil {
		return nil, &rest.APIError{
			Message:    "failed to build stats response",
			StatusCode: http.StatusInternalServerError,
		}
	}

	return &stats, nil
}

type MockNotificationConnector struct{}

func (c *MockNotificationConnector) GetNotificationsStats() (*restModel.APIEventStats, error) {
	return nil, errors.New("not implemented")
}
