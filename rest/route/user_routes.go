package route

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

type userSettingsHandler struct {
	settings model.APIUserSettings
}

func getUserSettingsRouteManager(route string, version int) *RouteManager {
	h := userSettingsHandler{}
	userSettingsPost := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    h.Handler(),
		MethodType:        http.MethodPost,
	}

	return &RouteManager{
		Route:   route,
		Methods: []MethodHandler{userSettingsPost},
		Version: version,
	}
}

func (h *userSettingsHandler) Handler() RequestHandler {
	return &userSettingsHandler{}
}

func (h *userSettingsHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	h.settings = model.APIUserSettings{}
	return util.ReadJSONInto(r.Body, &h.settings)
}

func (h *userSettingsHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	adminSettings, err := evergreen.GetConfig()
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Error retrieving Evergreen settings")
	}
	userSettingsInterface, err := h.settings.ToService()
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Error parsing user settings")
	}
	userSettings, ok := userSettingsInterface.(user.UserSettings)
	if !ok {
		return ResponseData{}, errors.New("Unable to parse settings object")
	}

	if len(userSettings.GithubUser.LastKnownAs) == 0 {
		userSettings.GithubUser = user.GithubUser{}
	} else if u.Settings.GithubUser.LastKnownAs != userSettings.GithubUser.LastKnownAs {
		var token string
		var ghUser *github.User
		token, err = adminSettings.GetGithubOauthToken()
		if err != nil {
			return ResponseData{}, errors.Wrap(err, "Error retrieving Github token")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		ghUser, err = thirdparty.GetGithubUser(ctx, token, userSettings.GithubUser.LastKnownAs)
		if err != nil {
			return ResponseData{}, errors.Wrap(err, "Error fetching user from Github")
		}

		userSettings.GithubUser.LastKnownAs = *ghUser.Login
		userSettings.GithubUser.UID = *ghUser.ID
	} else {
		userSettings.GithubUser.UID = u.Settings.GithubUser.UID
	}

	if err = sc.UpdateSettings(u.Username(), userSettings); err != nil {
		return ResponseData{}, errors.Wrap(err, "Error saving user settings")
	}

	return ResponseData{
		Result: []model.Model{},
	}, nil
}
