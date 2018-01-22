package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	mgo "gopkg.in/mgo.v2"
)

type RepoTrackerConnector struct{}

func (c *RepoTrackerConnector) TriggerRepotracker(q amboy.Queue, msgID string, event *github.PushEvent) error {
	if err := validatePushEvent(event); err != nil {
		return err
	}
	ref, err := validateProjectRef(*event.Repo.Owner.Name, *event.Repo.Name)
	if err != nil {
		return err
	}
	if !ref.TracksPushEvents || !ref.Enabled {
		return nil
	}
	if err := q.Put(units.NewRepotrackerJob(msgID, ref.Identifier)); err != nil {
		msg := "failed to add repotracker job to queue"
		grip.Error(message.WrapError(err, message.Fields{
			"message": msg,
		}))
		return &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    msg,
		}
	}

	return nil
}

type MockRepoTrackerConnector struct{}

func (c *MockRepoTrackerConnector) TriggerRepotracker(_ amboy.Queue, _ string, event *github.PushEvent) error {
	if err := validatePushEvent(event); err != nil {
		return err
	}

	_, err := validateProjectRef(*event.Repo.Owner.Name, *event.Repo.Name)
	if err != nil {
		return err
	}

	return nil
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

func validateProjectRef(owner, repo string) (*model.ProjectRef, error) {
	ref, err := model.FindOneProjectRefByRepo(owner, repo)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "can't find project ref",
			}
		}
		return nil, &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	return ref, nil
}
