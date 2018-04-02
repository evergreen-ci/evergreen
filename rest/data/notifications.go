package data

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

type NotificationConnector struct {
}

func (c *NotificationConnector) GetNotificationsStats(ctx context.Context, _ amboy.Queue) (*restModel.APINotificationStats, error) {
	stats := restModel.APINotificationStats{}

	e, err := event.FindLastProcessedEvent()
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch most recently processed event")
	}
	stats.LastProcessed = e.ProcessedAt

	n, err := event.CountUnprocessedEvents()
	if err != nil {
		return nil, errors.Wrap(err, "failed to count unprocessed events")
	}
	stats.NumUnprocessedEvents = n

	return &stats, nil
}

type MockNotificationConnector struct {
}

func (c *MockNotificationConnector) GetNotificationsStats(_ context.Context, _ amboy.Queue) (*restModel.APINotificationStats, error) {
	return nil, errors.New("not implemented")
}
