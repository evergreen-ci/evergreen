package data

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

func GetNotificationsStats(ctx context.Context) (*restModel.APIEventStats, error) {
	stats := restModel.APIEventStats{}

	e, err := event.FindLastProcessedEvent(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetching most recently processed event")
	}
	if e != nil {
		stats.LastProcessedAt = &e.ProcessedAt
	}

	n, err := event.CountUnprocessedEvents(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "counting unprocessed events")
	}
	stats.NumUnprocessedEvents = n

	nStats, err := notification.CollectUnsentNotificationStats(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "collecting unsent notification stats")
	}
	if nStats != nil {
		stats.BuildFromService(*nStats)
	}
	return &stats, nil
}

// FindFailedWebhooks returns failed webhook deliveries for the given projects,
// optionally filtered by URL, newest first.
func FindFailedWebhooks(ctx context.Context, projects []string, url string, limit int) ([]restModel.APIFailedWebhook, error) {
	failedWebhooks, err := notification.FindFailedWebhooks(ctx, projects, url, limit)
	if err != nil {
		return nil, errors.Wrap(err, "finding failed webhook notifications")
	}

	apiFailedWebhooks := make([]restModel.APIFailedWebhook, len(failedWebhooks))
	for i := range failedWebhooks {
		apiFailedWebhooks[i].BuildFromService(failedWebhooks[i])
	}

	return apiFailedWebhooks, nil
}
