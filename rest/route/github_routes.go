package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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

	event interface{}
	msgID string
}

func getGithubHooksRouteManager(queue amboy.Queue, secret []byte) routeManagerFactory {
	return func(route string, version int) *RouteManager {
		methods := []MethodHandler{}
		if len(secret) > 0 {
			methods = append(methods, MethodHandler{
				Authenticator: &NoAuthAuthenticator{},
				RequestHandler: &githubHookApi{
					queue:  queue,
					secret: secret,
				},
				MethodType: http.MethodPost,
			})

		} else {
			grip.Warning("Github webhook secret is empty! Github webhooks have been disabled!")
		}

		return &RouteManager{
			Route:   route,
			Methods: methods,
			Version: version,
		}
	}
}

func (gh *githubHookApi) Handler() RequestHandler {
	return &githubHookApi{
		queue:  gh.queue,
		secret: gh.secret,
	}
}

func (gh *githubHookApi) ParseAndValidate(ctx context.Context, r *http.Request) error {
	eventType := r.Header.Get("X-Github-Event")
	gh.msgID = r.Header.Get("X-Github-Delivery")

	if len(gh.secret) == 0 || gh.queue == nil {
		return rest.APIError{
			StatusCode: http.StatusInternalServerError,
		}
	}

	body, err := github.ValidatePayload(r, gh.secret)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "github hook",
			"message": "rejecting github webhook",
			"msg_id":  gh.msgID,
			"event":   eventType,
		}))
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "failed to read request body",
		}
	}

	gh.event, err = github.ParseWebHook(eventType, body)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "github hook",
			"message": "rejecting github webhook",
			"msg_id":  gh.msgID,
			"event":   eventType,
		}))
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}

	return nil
}

func (gh *githubHookApi) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	switch event := gh.event.(type) {
	case *github.PingEvent:
		if event.Hook == nil || event.Hook.URL == nil {
			return ResponseData{}, rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "malformed ping event",
			}

		}
		grip.Info(message.Fields{
			"source":   "github hook",
			"msg_id":   gh.msgID,
			"event":    "ping_event",
			"hook_url": *event.Hook.URL,
		})

	case *github.PullRequestEvent:
		if event.Action == nil {
			return ResponseData{}, rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "pull request has no action",
			}
		}

		if *event.Action == githubActionOpened || *event.Action == githubActionSynchronize ||
			*event.Action == githubActionReopened {
			ghi, err := patch.NewGithubIntent(gh.msgID, event)
			if err != nil {
				return ResponseData{}, &rest.APIError{
					StatusCode: http.StatusBadRequest,
					Message:    err.Error(),
				}
			}
			grip.Info(message.Fields{
				"source":    "github hook",
				"msg_id":    gh.msgID,
				"event":     "pull_request",
				"action":    *event.Action,
				"repo":      *event.Repo.FullName,
				"ref":       *event.PullRequest.Base.Ref,
				"pr_number": *event.Number,
				"creator":   *event.Sender.Login,
				"hash":      *event.PullRequest.Head.SHA,
			})

			if err := sc.AddPatchIntent(ghi, gh.queue); err != nil {
				return ResponseData{}, &rest.APIError{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				}
			}

		} else if *event.Action == githubActionClosed {
			grip.Info(message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   "pull_request",
				"message": "pull request closed; aborting patch",
			})
			return ResponseData{}, sc.AbortPatchesFromPullRequest(event)
		}

	case *github.PushEvent:
		return ResponseData{}, sc.TriggerRepotracker(gh.queue, gh.msgID, event)
	}

	return ResponseData{}, nil
}
