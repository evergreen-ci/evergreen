package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/google/go-github/github"
	"github.com/k0kubun/pp"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

type githubHookApi struct {
	secret []byte
	event  interface{}

	eventType string
	uid       string
}

func getGithubHooksRouteManager(route string, version int) *RouteManager {
	settings := evergreen.GetEnvironment().Settings()

	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator: &NoAuthAuthenticator{},
				RequestHandler: &githubHookApi{
					secret: settings.AuthConfig.Github.HookSecret,
				},
				MethodType: http.MethodPost,
			},
		},
		Version: version,
	}
}

func (gh *githubHookApi) Handler() RequestHandler {
	return &githubHookApi{}
}

func (gh *githubHookApi) ParseAndValidate(ctx context.Context, r *http.Request) error {
	// Reject a request which is not json (hook misconfiguration)
	if r.Header.Get("Content-type") != "application/json" {
		return rest.APIError{
			StatusCode: http.StatusUnsupportedMediaType,
			Message:    "invalid content type",
		}
	}

	gh.eventType = r.Header.Get("X-Github-Event")

	gh.uid = r.Header.Get("X-Github-Delivery")

	body, err := github.ValidatePayload(r, gh.secret)
	if err != nil {
		grip.Error(message.Fields{
			"message": "invalid data from Github; either auth or API change",
			"error":   err,
		})
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "failed to read request body",
		}
	}

	gh.event, err = github.ParseWebHook(gh.eventType, body)
	if err != nil {
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}

	return nil
}

func (gh *githubHookApi) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	pp.Println(gh.event)

	switch event := gh.event.(type) {
	case github.PullRequestEvent:
	case github.MembershipEvent:
	case github.PingEvent:

	}

	return ResponseData{}, nil
}
