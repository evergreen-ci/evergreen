package data

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const branchRefPrefix = "refs/heads/"

type RepoTrackerConnector struct{}

func (c *RepoTrackerConnector) TriggerRepotracker(q amboy.Queue, msgID string, event *github.PushEvent) error {
	branch, err := validatePushEvent(event)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source": "github hook",
			"msg_id": msgID,
			"event":  "push",
		}))
		return err
	}
	if len(branch) == 0 {
		return nil
	}
	adminSettings, err := evergreen.GetConfig()
	if err != nil {
		return errors.Wrap(err, "error retrieving admin settings")
	}
	if adminSettings.ServiceFlags.RepotrackerDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "repotracker is disabled",
			"source":  "github hooks",
		})
		return errors.New("repotracker is disabled")
	}

	ref, err := validateProjectRef(*event.Repo.Owner.Name, *event.Repo.Name,
		branch)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "github hook",
			"msg_id":  msgID,
			"event":   "push",
			"owner":   *event.Repo.Owner.Name,
			"repo":    *event.Repo.Name,
			"ref":     *event.Ref,
			"message": "can't match push event to project ref",
		}))
		return err
	}
	fields := message.Fields{
		"source":      "github hook",
		"msg_id":      msgID,
		"event":       "push",
		"owner":       *event.Repo.Owner.Name,
		"repo":        *event.Repo.Name,
		"ref":         *event.Ref,
		"project_ref": ref.Identifier,
		"message":     "actionable push acknowledged",
	}
	if !ref.TracksPushEvents || !ref.Enabled {
		fields["tracks_push_events"] = ref.TracksPushEvents
		fields["enabled"] = ref.Enabled
		fields["message"] = "unactionable push acknowledged"
		grip.Info(fields)
		return nil
	}
	grip.Info(fields)

	if err := q.Put(units.NewRepotrackerJob(fmt.Sprintf("github-push-%s", msgID), ref.Identifier)); err != nil {
		msg := "failed to add repotracker job to queue"
		grip.Error(message.WrapError(err, message.Fields{
			"source":      "github hook",
			"msg_id":      msgID,
			"event":       "push",
			"owner":       *event.Repo.Owner.Name,
			"repo":        *event.Repo.Name,
			"ref":         *event.Ref,
			"project_ref": ref.Identifier,
			"message":     msg,
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
		return "", &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Unexpected Git ref format: %s", *event.Ref),
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
