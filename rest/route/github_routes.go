package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type githubHookApi struct {
	event interface{}

	msgId string
}

func getGithubHooksRouteManager(route string, version int) *RouteManager {
	methods := []MethodHandler{}
	if secret, err := getWebhookSecret(); err == nil && len(secret) > 0 {
		methods = append(methods, MethodHandler{
			Authenticator:  &NoAuthAuthenticator{},
			RequestHandler: &githubHookApi{},
			MethodType:     http.MethodPost,
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

func (gh *githubHookApi) Handler() RequestHandler {
	return &githubHookApi{}
}

func getWebhookSecret() ([]byte, error) {
	settings := evergreen.GetEnvironment().Settings()
	if settings != nil {
		secret := settings.Api.GithubWebhookSecret
		if len(secret) != 0 {
			return []byte(secret), nil
		}
	}

	return nil, errors.New("No secret key")
}

func (gh *githubHookApi) ParseAndValidate(ctx context.Context, r *http.Request) error {
	eventType := r.Header.Get("X-Github-Event")
	gh.msgId = r.Header.Get("X-Github-Delivery")

	secret, err := getWebhookSecret()
	if err != nil {
		return rest.APIError{
			StatusCode: http.StatusInternalServerError,
		}
	}

	body, err := github.ValidatePayload(r, secret)
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
		grip.Infof("Received Github Webhook Ping for hook: %s", event.Hook.URL)

	case *github.PullRequestEvent:
		if !validatePullRequestEvent(event) {
			return ResponseData{}, rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "bad pull request",
			}

		}
		if *event.Action == "opened" || *event.Action == "synchronize" {
			ghi, err := patch.NewGithubIntent(gh.msgId, *event.Repo.FullName, *event.Number, *event.Sender.Login, *event.PullRequest.Base.SHA, *event.PullRequest.PatchURL)
			if err != nil {
				return ResponseData{}, rest.APIError{
					StatusCode: http.StatusBadRequest,
					Message:    "unexpected pr event data",
				}
			}

			if err := sc.AddPatchIntent(ghi); err != nil {
				return ResponseData{}, rest.APIError{
					StatusCode: http.StatusInternalServerError,
					Message:    "failed to created patch intent",
				}
			}
		}
	}

	return ResponseData{}, nil
}

func validatePullRequestEvent(event *github.PullRequestEvent) bool {
	// crying
	if event.Action == nil || event.Number == nil ||
		event.Repo == nil || event.Repo.FullName == nil ||
		event.Sender == nil || event.Sender.Login == nil ||
		event.PullRequest == nil || event.PullRequest.PatchURL == nil ||
		event.PullRequest.Base == nil || event.PullRequest.Base.SHA == nil {
		return false
	}

	return true
}
