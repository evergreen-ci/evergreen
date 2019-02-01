package route

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
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
}

func makeGithubHooksRoute(sc data.Connector, queue amboy.Queue, secret []byte) gimlet.RouteHandler {
	return &githubHookApi{
		sc:     sc,
		queue:  queue,
		secret: secret,
	}
}

func (gh *githubHookApi) Factory() gimlet.RouteHandler {
	return &githubHookApi{
		queue:  gh.queue,
		secret: gh.secret,
		sc:     gh.sc,
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
			"hook_id": *event.HookID,
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
					"action":  *event.Action,
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
				"action":    *event.Action,
				"message":   "pr accepted, attempting to queue",
				"repo":      *event.Repo.FullName,
				"ref":       *event.PullRequest.Base.Ref,
				"pr_number": *event.Number,
				"creator":   *event.Sender.Login,
				"hash":      *event.PullRequest.Head.SHA,
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

		owner := *event.Repo.Owner.Login
		repo := *event.Repo.Name
		PRNum := *event.Issue.Number
		pr, err := gh.sc.GetGitHubPR(ctx, owner, repo, PRNum)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  *event.Action,
				"owner":   owner,
				"repo":    repo,
				"item":    PRNum,
				"message": "get pr failed",
			}))
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't get PR from GitHub API"))
		}

		if pr == nil || pr.Base == nil || pr.Base.Label == nil {
			grip.Error(message.WrapError(err, message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  *event.Action,
				"owner":   owner,
				"repo":    repo,
				"item":    PRNum,
				"message": "PR contains no base branch label",
			}))
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "PR contains no base branch label"))
		}

		baseBranch := *pr.Base.Label
		err = gh.sc.EnqueueItem(owner, repo, baseBranch, strconv.Itoa(PRNum))
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  *event.Action,
				"owner":   owner,
				"repo":    repo,
				"item":    PRNum,
				"message": "commit queue enqueue failed",
			}))
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't enqueue item on commit queue"))
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}
