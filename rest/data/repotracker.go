package data

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const branchRefPrefix = "refs/heads/"

// TriggerRepotracker creates an amboy job to get the commits from a
// Github Push Event
func TriggerRepotracker(ctx context.Context, q amboy.Queue, msgID string, event *github.PushEvent) error {
	branch, err := validatePushEvent(event)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source": "GitHub hook",
			"msg_id": msgID,
			"event":  "push",
		}))
		return err
	}
	if len(branch) == 0 {
		return nil
	}

	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "retrieving admin settings")
	}
	if settings.ServiceFlags.RepotrackerDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"source":  "GitHub hook",
			"msg_id":  msgID,
			"event":   "push",
			"owner":   *event.Repo.Owner.Name,
			"repo":    *event.Repo.Name,
			"ref":     *event.Ref,
			"message": "repotracker is disabled",
		})
		return errors.New("repotracker is disabled")
	}
	if len(settings.GithubOrgs) > 0 && !utility.StringSliceContains(settings.GithubOrgs, *event.Repo.Owner.Name) {
		grip.Info(message.Fields{
			"source":  "GitHub hook",
			"msg_id":  msgID,
			"event":   "push",
			"owner":   *event.Repo.Owner.Name,
			"repo":    *event.Repo.Name,
			"ref":     *event.Ref,
			"message": "owner from push event is invalid",
		})
		return errors.New("owner from push event is invalid")
	}

	refs, err := model.FindMergedEnabledProjectRefsByRepoAndBranch(*event.Repo.Owner.Name, *event.Repo.Name, branch)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "GitHub hook",
			"msg_id":  msgID,
			"event":   "push",
			"owner":   *event.Repo.Owner.Name,
			"repo":    *event.Repo.Name,
			"ref":     *event.Ref,
			"branch":  branch,
			"message": "error trying to get project refs",
		}))
		return err
	}
	if len(refs) == 0 {
		grip.Info(message.Fields{
			"source":  "github hook",
			"msg_id":  msgID,
			"event":   "push",
			"owner":   *event.Repo.Owner.Name,
			"repo":    *event.Repo.Name,
			"ref":     *event.Ref,
			"branch":  branch,
			"message": "no matching project refs found",
		})
		return nil
	}

	succeeded := []string{}
	unactionable := []string{}
	failed := []string{}
	catcher := grip.NewSimpleCatcher()
	for i := range refs {
		if !refs[i].DoesTrackPushEvents() || !refs[i].Enabled || refs[i].IsRepotrackerDisabled() {
			unactionable = append(unactionable, refs[i].Id)
			continue
		}

		err = trigger.TriggerDownstreamProjectsForPush(ctx, refs[i].Id, event, trigger.TriggerDownstreamVersion)
		catcher.Wrapf(err, "triggering downstream projects for push event for project '%s'", refs[i].Id)

		j := units.NewRepotrackerJob(fmt.Sprintf("github-push-%s", msgID), refs[i].Id)

		if err := amboy.EnqueueUniqueJob(ctx, q, j); err != nil {
			catcher.Wrap(err, "enqueueing repotracker job for GitHub push events")
			failed = append(failed, refs[i].Id)

		} else {
			succeeded = append(succeeded, refs[i].Id)
		}
	}

	grip.Error(message.WrapError(catcher.Resolve(), message.Fields{
		"source":  "GitHub hook",
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
		"source":  "GitHub hook",
		"msg_id":  msgID,
		"event":   "push",
		"owner":   *event.Repo.Owner.Name,
		"repo":    *event.Repo.Name,
		"ref":     *event.Ref,
		"message": "done processing push event",
		"project_refs": message.Fields{
			"failed":       failed,
			"succeeded":    succeeded,
			"unactionable": unactionable,
		},
	})

	if catcher.HasErrors() {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    catcher.Resolve().Error(),
		}
	}

	return nil
}

func validatePushEvent(event *github.PushEvent) (string, error) {
	if event == nil || event.Ref == nil || event.Repo == nil ||
		event.Repo.Name == nil || event.Repo.Owner == nil ||
		event.Repo.Owner.Name == nil || event.Repo.FullName == nil {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid push event from GitHub",
		}
	}

	if !strings.HasPrefix(*event.Ref, branchRefPrefix) {
		// Not an error, but we're uninterested in tag pushes
		return "", nil
	}

	refs := strings.Split(*event.Ref, "/")
	if len(refs) < 3 {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("unexpected Git ref format: %s", *event.Ref),
		}
	}
	return strings.Join(refs[2:], "/"), nil
}
