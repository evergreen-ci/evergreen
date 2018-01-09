package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
)

type RepoTrackerConnector struct{}

func (c *RepoTrackerConnector) TriggerRepotracker(q amboy.Queue, msgID string, event *github.PushEvent) error {
	if err := validatePushEvent(event); err != nil {
		return err
	}
	if err := q.Put(units.NewRepotrackerJob(msgID, *event.Repo.Owner.Name,
		*event.Repo.Name)); err != nil {
		return &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to add repotracker job to queue",
		}
	}

	return nil
}

type MockRepoTrackerConnector struct{}

func (c *MockRepoTrackerConnector) TriggerRepotracker(_ amboy.Queue, _ string, event *github.PushEvent) error {
	return validatePushEvent(event)
}

func validatePushEvent(event *github.PushEvent) error {
	if event == nil || event.Ref == nil || event.Repo == nil ||
		event.Repo.Name == nil || event.Repo.Owner == nil ||
		event.Repo.Owner.Name == nil || event.Repo.FullName == nil {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid PushEvent from github",
		}
	}
	return nil
}
