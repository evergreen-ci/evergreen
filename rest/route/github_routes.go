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

	event     interface{}
	eventType string
	msgID     string
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
	gh.eventType = r.Header.Get("X-Github-Event")
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
			"event":   gh.eventType,
		}))
		return rest.APIError{
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
			"event":    gh.eventType,
			"hook_url": *event.Hook.URL,
		})

	case *github.PullRequestEvent:
		if event.Action == nil {
			err := rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "pull request has no action",
			}
			grip.Error(message.WrapError(err, message.Fields{
				"source": "github hook",
				"msg_id": gh.msgID,
				"event":  gh.eventType,
			}))

			return ResponseData{}, err
		}

		if *event.Action == githubActionOpened || *event.Action == githubActionSynchronize ||
			*event.Action == githubActionReopened {
			ghi, err := patch.NewGithubIntent(gh.msgID, event)
			if err != nil {
				grip.Info(message.WrapError(err, message.Fields{
					"source":  "github hook",
					"msg_id":  gh.msgID,
					"event":   gh.eventType,
					"action":  *event.Action,
					"message": "failed to create intent",
				}))
				return ResponseData{}, &rest.APIError{
					StatusCode: http.StatusBadRequest,
					Message:    err.Error(),
				}
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
				"event":   gh.eventType,
				"action":  *event.Action,
				"message": "pull request closed; aborting patch",
			})

			err := sc.AbortPatchesFromPullRequest(event)
			grip.ErrorWhen(err != nil, message.WrapError(err, message.Fields{
				"source":  "github hook",
				"msg_id":  gh.msgID,
				"event":   gh.eventType,
				"action":  *event.Action,
				"message": "failed to abort patches",
			}))

			return ResponseData{}, err
		}

	case *github.PushEvent:
		return ResponseData{}, sc.TriggerRepotracker(gh.queue, gh.msgID, event)
	}

	return ResponseData{}, nil
}
