package data

import (
	"context"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/mongodb/amboy"
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

// RetryFailedWebhooks returns the failed webhook deliveries for the project.
// Unless dryRun is set, each one is also enqueued for retry.
func RetryFailedWebhooks(ctx context.Context, env evergreen.Environment, pRef *model.ProjectRef, dryRun bool) ([]restModel.APIFailedWebhook, error) {
	project := pRef.Id
	projects := []string{project}
	// Repo-level subscriptions tag failures with the repo ref ID, so include it
	// in the lookup.
	if pRef.RepoRefId != "" && pRef.RepoRefId != project {
		projects = append(projects, pRef.RepoRefId)
	}

	failedWebhooks, err := notification.FindFailedWebhooks(ctx, projects)
	if err != nil {
		return nil, errors.Wrapf(err, "finding failed webhooks for project '%s'", project)
	}

	if !dryRun && len(failedWebhooks) > 0 {
		jobs := make([]amboy.Job, len(failedWebhooks))
		for i, failedWebhook := range failedWebhooks {
			// Key the job on the failure so repeated calls for the same failure
			// dedupe, while a webhook that failed again gets a new job.
			jobs[i] = units.NewRetryEventSendJob(failedWebhook.NotificationID, strconv.FormatInt(failedWebhook.FailedAt.UnixMilli(), 10))
		}
		if err := amboy.EnqueueManyUniqueJobs(ctx, env.RemoteQueue(), jobs); err != nil {
			return nil, errors.Wrapf(err, "enqueueing webhook retries for project '%s'", project)
		}
	}

	apiFailedWebhooks := make([]restModel.APIFailedWebhook, len(failedWebhooks))
	for i := range failedWebhooks {
		apiFailedWebhooks[i].BuildFromService(failedWebhooks[i])
	}
	return apiFailedWebhooks, nil
}
