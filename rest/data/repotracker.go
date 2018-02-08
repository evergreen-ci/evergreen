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
			"source":  "github hook",
			"msg_id":  msgID,
			"event":   "push",
			"owner":   *event.Repo.Owner.Name,
			"repo":    *event.Repo.Name,
			"ref":     *event.Ref,
			"message": "repotracker is disabled",
		})
		return errors.New("repotracker is disabled")
	}

	refs, err := validateProjectRefs(*event.Repo.Owner.Name, *event.Repo.Name, branch)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "github hook",
			"msg_id":  msgID,
			"event":   "push",
			"owner":   *event.Repo.Owner.Name,
			"repo":    *event.Repo.Name,
			"ref":     *event.Ref,
			"message": "error occurred while trying to match match push event to project refs",
		}))
		return err
	}

	succeeded := []string{}
	unactionable := []string{}
	failed := []string{}
	catcher := grip.NewSimpleCatcher()
	for i := range refs {
		if !refs[i].TracksPushEvents || !refs[i].Enabled {
			unactionable = append(unactionable, refs[i].Identifier)
			continue
		}

		if err := q.Put(units.NewRepotrackerJob(fmt.Sprintf("github-push-%s", msgID), refs[i].Identifier)); err != nil {
			catcher.Add(errors.Errorf("failed to add repotracker job to queue for project: '%s'", refs[i].Identifier))
			failed = append(failed, refs[i].Identifier)

		} else {
			succeeded = append(succeeded, refs[i].Identifier)
		}
	}

	grip.Error(message.WrapError(catcher.Resolve(), message.Fields{
		"source":  "github hook",
		"msg_id":  msgID,
		"event":   "push",
		"owner":   *event.Repo.Owner.Name,
		"repo":    *event.Repo.Name,
		"ref":     *event.Ref,
		"message": "errors occurred while triggering repotracker",
		"project_refs": message.Fields{
			"failed":       failed,
			"succeeded":    succeeded,
			"unactionable": unactionable,
		},
	}))

	grip.Info(message.Fields{
		"source":  "github hook",
		"msg_id":  msgID,
		"event":   "push",
		"owner":   *event.Repo.Owner.Name,
		"repo":    *event.Repo.Name,
		"ref":     *event.Ref,
		"message": "done processing PushEvent",
		"project_refs": message.Fields{
			"failed":       failed,
			"succeeded":    succeeded,
			"unactionable": unactionable,
		},
	})

	if catcher.HasErrors() {
		return &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    catcher.Resolve().Error(),
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
	if len(branch) == 0 {
		return nil
	}

	_, err = validateProjectRefs(*event.Repo.Owner.Name, *event.Repo.Name, branch)

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
		// Not an error, but we're uninterested in tag pushes
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

func validateProjectRefs(owner, repo, branch string) ([]model.ProjectRef, error) {
	refs, err := model.FindProjectRefsByRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	if len(refs) == 0 {
		return nil, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "no project refs found",
		}
	}

	return refs, nil
}
