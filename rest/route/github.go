package route

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	githubActionClosed      = "closed"
	githubActionOpened      = "opened"
	githubActionSynchronize = "synchronize"
	githubActionReopened    = "reopened"

	retryComment = "evergreen retry"
)

type githubHookApi struct {
	queue  amboy.Queue
	secret []byte

	event     interface{}
	eventType string
	msgID     string
	sc        data.Connector
	settings  *evergreen.Settings
}

func makeGithubHooksRoute(sc data.Connector, queue amboy.Queue, secret []byte, settings *evergreen.Settings) gimlet.RouteHandler {
	return &githubHookApi{
		sc:       sc,
		settings: settings,
		queue:    queue,
		secret:   secret,
	}
}

func (gh *githubHookApi) Factory() gimlet.RouteHandler {
	return &githubHookApi{
		queue:    gh.queue,
		secret:   gh.secret,
		sc:       gh.sc,
		settings: gh.settings,
	}
}

func (gh *githubHookApi) Parse(ctx context.Context, r *http.Request) error {
	gh.eventType = r.Header.Get("X-Github-Event")
	gh.msgID = r.Header.Get("X-Github-Delivery")

	if len(gh.secret) == 0 || gh.queue == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "webhooks are not configured and therefore disabled",
		}
	}

	body, err := github.ValidatePayload(r, gh.secret)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "github hook",
			"message": "rejecting github webhook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
		}))
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "failed to read request body",
		}
	}

	gh.event, err = github.ParseWebHook(gh.eventType, body)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "github hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"message": "rejecting github webhook",
		}))
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}

	return nil
}

func (gh *githubHookApi) Run(ctx context.Context) gimlet.Responder {
	switch event := gh.event.(type) {
	case *github.PingEvent:
		if event.HookID == nil {
			return gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "malformed ping event",
			})
		}
		grip.Info(message.Fields{
			"source":  "github hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"hook_id": event.HookID,
		})

	case *github.PullRequestEvent:
		if event.Action == nil {
			err := gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "pull request has no action",
			}
			grip.Error(message.WrapError(err, message.Fields{
				"source": "github hook",
				"msg_id": gh.msgID,
				"event":  gh.eventType,
			}))

			return gimlet.NewJSONErrorResponse(err)
		}

		if *event.Action == githubActionOpened || *event.Action == githubActionSynchronize ||
			*event.Action == githubActionReopened {
			grip.Info(message.Fields{
				"source":    "github hook",
				"msg_id":    gh.msgID,
				"event":     gh.eventType,
				"repo":      *event.PullRequest.Base.Repo.FullName,
				"ref":       *event.PullRequest.Base.Ref,
				"pr_number": *event.PullRequest.Number,
				"hash":      *event.PullRequest.Head.SHA,
				"message":   "pr accepted, attempting to queue",
			})
			if err := gh.AddIntentForPR(event.PullRequest); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":    "github hook",
					"msg_id":    gh.msgID,
					"event":     gh.eventType,
					"action":    event.Action,
					"repo":      *event.PullRequest.Base.Repo.FullName,
					"ref":       *event.PullRequest.Base.Ref,
					"pr_number": *event.PullRequest.Number,
					"message":   "can't add intent",
				}))
				return gimlet.NewJSONErrorResponse(err)
			}
		} else if *event.Action == githubActionClosed {
			grip.Info(message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  *event.Action,
				"message": "pull request closed; aborting patch",
			})

			err := gh.sc.AbortPatchesFromPullRequest(event)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":  "github hook",
					"msg_id":  gh.msgID,
					"event":   gh.eventType,
					"action":  *event.Action,
					"message": "failed to abort patches",
				}))
				return gimlet.MakeJSONErrorResponder(err)
			}

			// if the item is on a commit queue, remove it
			if err = gh.tryDequeueCommitQueueItemForPR(event.PullRequest); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":  "github hook",
					"msg_id":  gh.msgID,
					"event":   gh.eventType,
					"action":  *event.Action,
					"message": "commit queue item not dequeued",
				}))
				return gimlet.MakeJSONErrorResponder(err)
			}

			return gimlet.NewJSONResponse(struct{}{})
		}

	case *github.PushEvent:
		if err := gh.sc.TriggerRepotracker(gh.queue, gh.msgID, event); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		return gimlet.NewJSONResponse(struct{}{})

	case *github.IssueCommentEvent:
		if event.Issue.IsPullRequest() {
			if commitqueue.TriggersCommitQueue(*event.Action, *event.Comment.Body) {
				grip.Info(message.Fields{
					"source":    "github hook",
					"msg_id":    gh.msgID,
					"event":     gh.eventType,
					"repo":      *event.Repo.FullName,
					"pr_number": *event.Issue.Number,
					"user":      *event.Sender.Login,
					"message":   "commit queue triggered",
				})
				if err := gh.commitQueueEnqueue(ctx, event); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"source":    "github hook",
						"msg_id":    gh.msgID,
						"event":     gh.eventType,
						"action":    event.Action,
						"repo":      *event.Repo.FullName,
						"pr_number": *event.Issue.Number,
						"user":      *event.Sender.Login,
						"message":   "can't enqueue on commit queue",
					}))
					return gimlet.MakeJSONErrorResponder(err)
				}
			}
			if triggersRetry(*event.Action, *event.Comment.Body) {
				grip.Info(message.Fields{
					"source":    "github hook",
					"msg_id":    gh.msgID,
					"event":     gh.eventType,
					"repo":      *event.Repo.FullName,
					"pr_number": *event.Issue.Number,
					"user":      *event.Sender.Login,
					"message":   "retry triggered",
				})
				if err := gh.retryPRPatch(ctx, event.Repo.Owner.GetLogin(), event.Repo.GetName(), event.Issue.GetNumber()); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"source":    "github hook",
						"msg_id":    gh.msgID,
						"event":     gh.eventType,
						"action":    event.Action,
						"repo":      *event.Repo.FullName,
						"pr_number": *event.Issue.Number,
						"user":      *event.Sender.Login,
						"message":   "can't retry PR",
					}))
					return gimlet.MakeJSONErrorResponder(err)
				}
			}
		}

	case *github.MetaEvent:
		if event.GetAction() == "deleted" {
			hookID := event.GetHookID()
			if hookID == 0 {
				msg := "invalid hook ID for deleted hook"
				grip.Error(message.Fields{
					"source":  "github hook",
					"msg_id":  gh.msgID,
					"event":   gh.eventType,
					"action":  event.Action,
					"hook":    event.Hook,
					"message": msg,
				})
				return gimlet.MakeJSONErrorResponder(errors.New(msg))
			}
			if err := model.RemoveGithubHook(int(hookID)); err != nil {
				return gimlet.MakeJSONErrorResponder(err)
			}
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func (gh *githubHookApi) retryPRPatch(ctx context.Context, owner, repo string, prNumber int) error {
	settings, err := gh.sc.GetEvergreenSettings()
	if err != nil {
		return errors.Wrap(err, "can't get Evergreen settings")
	}
	githubToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return errors.Wrap(err, "can't get GitHub token from settings")
	}

	pr, err := thirdparty.GetGithubPullRequest(ctx, githubToken, owner, repo, prNumber)
	if err != nil {
		return errors.Wrapf(err, "can't get PR for repo %s:%s, PR #%d", owner, repo, prNumber)
	}

	return gh.AddIntentForPR(pr)
}

func (gh *githubHookApi) AddIntentForPR(pr *github.PullRequest) error {
	ghi, err := patch.NewGithubIntent(gh.msgID, pr)
	if err != nil {
		return errors.Wrap(err, "failed to create intent")
	}

	if err := gh.sc.AddPatchIntent(ghi, gh.queue); err != nil {
		return errors.Wrap(err, "error saving intent")
	}

	return nil
}

func (gh *githubHookApi) commitQueueEnqueue(ctx context.Context, event *github.IssueCommentEvent) error {
	userRepo := data.UserRepoInfo{
		Username: *event.Comment.User.Login,
		Owner:    *event.Repo.Owner.Login,
		Repo:     *event.Repo.Name,
	}
	authorized, err := gh.sc.IsAuthorizedToPatchAndMerge(ctx, gh.settings, userRepo)
	if err != nil {
		return errors.Wrap(err, "can't get user info from GitHub API")
	}
	if !authorized {
		return errors.Errorf("user '%s' is not authorized to merge", userRepo.Username)
	}

	PRNum := *event.Issue.Number
	pr, err := gh.sc.GetGitHubPR(ctx, userRepo.Owner, userRepo.Repo, PRNum)
	if err != nil {
		return errors.Wrap(err, "can't get PR from GitHub API")
	}

	if pr == nil || pr.Base == nil || pr.Base.Ref == nil {
		return errors.New("PR contains no base branch label")
	}

	modules := restModel.ParseGitHubCommentModules(*event.Comment.Body)
	baseBranch := *pr.Base.Ref
	projectRef, err := gh.sc.GetProjectWithCommitQueueByOwnerRepoAndBranch(userRepo.Owner, userRepo.Repo, baseBranch)
	if err != nil {
		return errors.Wrapf(err, "can't get project for '%s:%s' tracking branch '%s'", userRepo.Owner, userRepo.Repo, baseBranch)
	}
	if projectRef == nil {
		return errors.Errorf("no project with commit queue enabled for '%s:%s' tracking branch '%s'", userRepo.Owner, userRepo.Repo, baseBranch)
	}
	item := restModel.APICommitQueueItem{
		Issue:   restModel.ToAPIString(strconv.Itoa(PRNum)),
		Modules: modules,
	}
	_, err = gh.sc.EnqueueItem(projectRef.Identifier, item)
	if err != nil {
		return errors.Wrap(err, "can't enqueue item on commit queue")
	}

	if pr == nil || pr.Head == nil || pr.Head.SHA == nil {
		return errors.New("PR contains no head branch SHA")
	}
	pushJob := units.NewGithubStatusUpdateJobForPushToCommitQueue(userRepo.Owner, userRepo.Repo, *pr.Head.SHA, PRNum)
	q := evergreen.GetEnvironment().LocalQueue()
	grip.Error(message.WrapError(q.Put(ctx, pushJob), message.Fields{
		"source":  "github hook",
		"msg_id":  gh.msgID,
		"event":   gh.eventType,
		"action":  event.Action,
		"owner":   userRepo.Owner,
		"repo":    userRepo.Repo,
		"item":    PRNum,
		"message": "failed to queue notification for commit queue push",
	}))

	return nil
}

// Because the PR isn't necessarily on a commit queue, we only error if item is on the queue and can't be removed correctly
func (gh *githubHookApi) tryDequeueCommitQueueItemForPR(pr *github.PullRequest) error {
	err := thirdparty.ValidatePR(pr)
	if err != nil {
		return errors.Wrap(err, "GitHub sent an incomplete PR")
	}

	projRef, err := gh.sc.GetProjectWithCommitQueueByOwnerRepoAndBranch(*pr.Base.Repo.Owner.Login, *pr.Base.Repo.Name, *pr.Base.Ref)
	if err != nil {
		return errors.Wrapf(err, "can't find valid project for %s/%s, branch %s", *pr.Base.Repo.Owner.Login, *pr.Base.Repo.Name, *pr.Base.Ref)
	}
	if projRef == nil {
		return nil
	}

	exists, err := gh.sc.IsItemOnCommitQueue(projRef.Identifier, strconv.Itoa(*pr.Number))
	if err != nil {
		return errors.Wrapf(err, "can't determine if item is on commit queue %s", projRef.Identifier)
	}
	if !exists {
		return nil
	}

	_, err = gh.sc.CommitQueueRemoveItem(projRef.Identifier, strconv.Itoa(*pr.Number))
	if err != nil {
		return errors.Wrapf(err, "can't remove item %d from commit queue %s", *pr.Number, projRef.Identifier)
	}
	return nil
}

func triggersRetry(action, comment string) bool {
	if action == "deleted" {
		return false
	}

	return comment == retryComment
}
