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
	"github.com/google/go-github/v70/github"
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
	ctx, span := tracer.Start(ctx, "trigger-repotracker")
	defer span.End()

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

	refs, err := model.FindMergedEnabledProjectRefsByRepoAndBranch(ctx, *event.Repo.Owner.Name, *event.Repo.Name, branch)
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
		// kim: NOTE: this checks if the branch is actually being tracked by any
		// branch projects before handling the push event. If there's no branch
		// project tracking this branch, then we ignore the push event.
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

		// kim: NOTE: this is reliable only as long as GitHub webhooks are
		// reliable, which they aren't. The repotracker handles backfilling, but
		// this solely relies on GitHub webhook best-effort delivery. Consider
		// moving to the repotracker so it can backfill commits.
		// kim: NOTE: I confirmed that GitHub webhooks suck. See the
		// head_commit.id in Splunk to see the commit that triggered the
		// webhook:
		// - For a commit that didn't trigger a downstream version, that commit
		// never sent a push event webhook: https://mongodb.splunkcloud.com/en-US/app/search/search?earliest=1756353600&latest=1756440000&q=search%20index%3Devergreen%20e1ac7309648c348989108bf7aac659f1d97f5d42%20%22GitHub%20hook%22%20event%3Dpush%20ref%3Drefs%2Fheads%2Fmaster&display.page.search.mode=verbose&dispatch.sample_ratio=1&workload_pool=&sid=1759952039.99844
		// - For a commit that did trigger a downstream version, that commit did
		// send a push event webhook: https://mongodb.splunkcloud.com/en-US/app/search/search?q=search%20index%3Devergreen%2038f3d1c7731218f8d49d7f274a2e5fa7d05b01fd%20%22GitHub%20hook%22%20event%3Dpush%20ref%3Drefs%2Fheads%2Fmaster&display.page.search.mode=verbose&dispatch.sample_ratio=1&workload_pool=&earliest=1759896000&latest=1759982400&sid=1759952123.99879
		// It appears GitHub push events are allowed to send multiple commits in
		// a single batch (see event.Commits NOT event.HeadCommit). The
		// implication is that we should handle all commits in the push event
		// individually, not just the head commit.
		// kim: NOTE: event.Commits in the list are ordered least to most
		// recent, so processing it in order should be fine.
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
