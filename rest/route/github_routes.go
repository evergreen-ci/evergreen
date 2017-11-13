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
	secret []byte
	event  interface{}

	msgId string
}

func getGithubHooksRouteManager(route string, version int) *RouteManager {
	methods := []MethodHandler{}
	if secret, err := getWebhookSecret(); err == nil && len(secret) > 0 {
		handler := &githubHookApi{
			secret: secret,
		}
		methods = append(methods, MethodHandler{
			Authenticator:  &NoAuthAuthenticator{},
			RequestHandler: handler,
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
	// TOOO ehhhhh
	secret, _ := getWebhookSecret()
	return &githubHookApi{
		secret: secret,
	}
}

func getWebhookSecret() ([]byte, error) {
	settings := evergreen.GetEnvironment().Settings()
	if settings != nil {
		secret := settings.Api.GithubWebhookSecret
		if len(secret) != 0 {
			return []byte(secret), nil
		}
	}

	return []byte{}, errors.New("No secret key")
}

func (gh *githubHookApi) ParseAndValidate(ctx context.Context, r *http.Request) error {
	eventType := r.Header.Get("X-Github-Event")
	gh.msgId = r.Header.Get("X-Github-Delivery")

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
		grip.Info("Received Github Webhook Ping")

	case *github.PullRequestEvent:
		// TODO look at all these pointers!
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
