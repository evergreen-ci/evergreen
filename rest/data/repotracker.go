package data

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const branchRefPrefix = "refs/heads/"

type RepoTrackerConnector struct{}

func (c *RepoTrackerConnector) TriggerRepotracker(q amboy.Queue, msgID string, event *github.PushEvent) error {
	branch, err := validatePushEvent(event)
	if err != nil {
		return err
	}
	ref, err := validateProjectRef(*event.Repo.Owner.Name, *event.Repo.Name,
		branch)
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
	branch, err := validatePushEvent(event)
	if err != nil {
		return err
	}

	_, err = validateProjectRef(*event.Repo.Owner.Name, *event.Repo.Name,
		branch)

	return err
}

func validatePushEvent(event *github.PushEvent) (string, error) {
	if event == nil || event.Ref == nil || event.Repo == nil ||
		event.Repo.Name == nil || event.Repo.Owner == nil ||
		event.Repo.Owner.Name == nil || event.Repo.FullName == nil {
		return "", &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid PushEvent from github",
		}
	}

	if !strings.HasPrefix(*event.Ref, branchRefPrefix) {
		// Not an error, we're uninterested in tag pushes
		return "", nil
	}

	refs := strings.Split(*event.Ref, "/")
	if len(refs) != 3 {
		msg := fmt.Sprintf("Unexpected Git ref format: %s", *event.Ref)
		grip.Info(message.Fields{
			"message": msg,
			"source":  "/rest/v2/hooks/github",
		})
		return "", &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    msg,
		}
	}

	return refs[2], nil
}

func validateProjectRef(owner, repo, branch string) (*model.ProjectRef, error) {
	ref, err := model.FindOneProjectRefByRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	if ref == nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "can't find project ref",
		}
	}

	return ref, nil
}
