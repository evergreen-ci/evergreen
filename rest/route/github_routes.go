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
)

type githubHookApi struct {
	queue  amboy.Queue
	secret []byte

	event interface{}
	msgId string
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
	gh.msgId = r.Header.Get("X-Github-Delivery")

	if len(gh.secret) == 0 || gh.queue == nil {
		return rest.APIError{
			StatusCode: http.StatusInternalServerError,
		}
	}

	body, err := github.ValidatePayload(r, gh.secret)
	if err != nil {
		grip.Errorf("Rejecting Github webhook POST: %+v", err)
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "failed to read request body",
		}
	}

	gh.event, err = github.ParseWebHook(eventType, body)
	if err != nil {
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
				Message:    "bad ping",
			}

		}
		grip.Infof("Received Github Webhook Ping for hook: %s", *event.Hook.URL)

	case *github.PullRequestEvent:
		if !validatePullRequestEvent(event) {
			return ResponseData{}, rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "bad pull request",
			}

		}
		if *event.Action == "opened" || *event.Action == "synchronize" {
			ghi, err := patch.NewGithubIntent(gh.msgId,
				*event.Number,
				*event.Repo.FullName,
				*event.PullRequest.Head.Repo.FullName,
				*event.PullRequest.Head.SHA,
				*event.Sender.Login,
				*event.PullRequest.DiffURL)
			if err != nil {
				return ResponseData{}, rest.APIError{
					StatusCode: http.StatusBadRequest,
					Message:    "unexpected pr event data",
				}
			}

			if err := sc.AddPatchIntent(ghi, gh.queue); err != nil {
				return ResponseData{}, rest.APIError{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				}
			}
		}
	}

	return ResponseData{}, nil
}

func validatePullRequestEvent(event *github.PullRequestEvent) bool {
	if event.Action == nil || event.Number == nil ||
		event.Repo == nil || event.Repo.FullName == nil ||
		event.Sender == nil || event.Sender.Login == nil ||
		event.PullRequest == nil || event.PullRequest.DiffURL == nil ||
		event.PullRequest.Head == nil || event.PullRequest.Head.SHA == nil ||
		event.PullRequest.Head.Repo == nil || event.PullRequest.Head.Repo.FullName == nil {
		return false
	}

	return true
}
