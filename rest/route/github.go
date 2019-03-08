package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
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
			ghi, err := patch.NewGithubIntent(gh.msgID, event)
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"source":  "github hook",
					"msg_id":  gh.msgID,
					"event":   gh.eventType,
					"action":  event.Action,
					"message": "failed to create intent",
				}))
				return gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    err.Error(),
				})
			}

			grip.Info(message.Fields{
				"source":    "github hook",
				"msg_id":    gh.msgID,
				"event":     gh.eventType,
				"action":    event.Action,
				"message":   "pr accepted, attempting to queue",
				"repo":      event.Repo.FullName,
				"ref":       event.PullRequest.Base.Ref,
				"pr_number": event.Number,
				"creator":   event.Sender.Login,
				"hash":      event.PullRequest.Head.SHA,
			})

			if err := gh.sc.AddPatchIntent(ghi, gh.queue); err != nil {
				return gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				})
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
			grip.ErrorWhen(err != nil, message.WrapError(err, message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  *event.Action,
				"message": "failed to abort patches",
			}))
			if err != nil {
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
		isPullRequestComment := event.Issue.IsPullRequest()
		triggersCommitq := commitqueue.TriggersCommitQueue(*event.Action, *event.Comment.Body)
		if !(isPullRequestComment && triggersCommitq) {
			return gimlet.NewJSONResponse(struct{}{})
		}

		userRepo := data.UserRepoInfo{
			Username: *event.Comment.User.Login,
			Owner:    *event.Repo.Owner.Login,
			Repo:     *event.Repo.Name,
		}
		authorized, err := gh.sc.IsAuthorizedToPatchAndMerge(ctx, gh.settings, userRepo)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  event.Action,
				"owner":   userRepo.Owner,
				"repo":    userRepo.Repo,
				"user":    userRepo.Username,
				"message": "get authorized failed",
			}))
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't get user info from GitHub API"))
		}
		if !authorized {
			grip.Error(message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  event.Action,
				"user":    userRepo.Username,
				"message": "user is not authorized to merge",
			})
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    fmt.Sprintf("user '%s' is not authorized to merge", userRepo.Username),
			})
		}

		PRNum := *event.Issue.Number
		pr, err := gh.sc.GetGitHubPR(ctx, userRepo.Owner, userRepo.Repo, PRNum)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  event.Action,
				"owner":   userRepo.Owner,
				"repo":    userRepo.Repo,
				"item":    PRNum,
				"message": "get pr failed",
			}))
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't get PR from GitHub API"))
		}

		if pr == nil || pr.Base == nil || pr.Base.Ref == nil {
			grip.Error(message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  event.Action,
				"owner":   userRepo.Owner,
				"repo":    userRepo.Repo,
				"item":    PRNum,
				"message": "PR contains no base branch ref",
			})
			return gimlet.MakeJSONErrorResponder(errors.New("PR contains no base branch label"))
		}

		modules := model.ParseGitHubCommentModules(*event.Comment.Body)
		baseBranch := *pr.Base.Ref
		item := model.APICommitQueueItem{
			Issue:   model.ToAPIString(strconv.Itoa(PRNum)),
			Modules: modules,
		}
		err = gh.sc.EnqueueItem(userRepo.Owner, userRepo.Repo, baseBranch, item)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  event.Action,
				"owner":   userRepo.Owner,
				"repo":    userRepo.Repo,
				"item":    PRNum,
				"message": "commit queue enqueue failed",
			}))
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't enqueue item on commit queue"))
		}

		pushJob := units.NewGithubStatusUpdateJobForPushToCommitQueue(userRepo.Owner, userRepo.Repo, baseBranch, PRNum)
		q := evergreen.GetEnvironment().LocalQueue()
		grip.Error(message.WrapError(q.Put(pushJob), message.Fields{
			"source":  "github hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"action":  event.Action,
			"owner":   userRepo.Owner,
			"repo":    userRepo.Repo,
			"item":    PRNum,
			"message": "failed to queue notification for commit queue push",
		}))

		grip.Info(message.Fields{
			"source":  "github hook",
			"msg_id":  gh.msgID,
			"event":   gh.eventType,
			"action":  event.Action,
			"comment": event.Comment.Body,
			"message": "finished processing comment",
		})
	}

	return gimlet.NewJSONResponse(struct{}{})
}
