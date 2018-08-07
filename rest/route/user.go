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
	"github.com/evergreen-ci/gimlet"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/users/settings

type userSettingsPostHandler struct {
	settings model.APIUserSettings
	sc       data.Connector
}

func makeSetUserConfig(sc data.Connector) gimlet.RouteHandler {
	return &userSettingsPostHandler{
		sc: sc,
	}
}

func (h *userSettingsPostHandler) Factory() gimlet.RouteHandler {
	return &userSettingsPostHandler{
		sc: h.sc,
	}
}

func (h *userSettingsPostHandler) Parse(ctx context.Context, r *http.Request) error {
	h.settings = model.APIUserSettings{}
	return errors.WithStack(util.ReadJSONInto(r.Body, &h.settings))
}

func (h *userSettingsPostHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	adminSettings, err := evergreen.GetConfig()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error retrieving Evergreen settings"))
	}
	changedSettings, err := model.ApplyUserChanges(u.Settings, h.settings)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "problem applying user settings"))
	}
	userSettingsInterface, err := changedSettings.ToService()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error parsing user settings"))
	}
	userSettings, ok := userSettingsInterface.(user.UserSettings)
	if !ok {
		return gimlet.MakeJSONErrorResponder(errors.New("Unable to parse settings object"))
	}

	if len(userSettings.GithubUser.LastKnownAs) == 0 {
		userSettings.GithubUser = user.GithubUser{}
	} else if u.Settings.GithubUser.LastKnownAs != userSettings.GithubUser.LastKnownAs {
		var token string
		var ghUser *github.User
		token, err = adminSettings.GetGithubOauthToken()
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error retrieving Github token"))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		ghUser, err = thirdparty.GetGithubUser(ctx, token, userSettings.GithubUser.LastKnownAs)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error fetching user from Github"))
		}

		userSettings.GithubUser.LastKnownAs = *ghUser.Login
		userSettings.GithubUser.UID = *ghUser.ID
	} else {
		userSettings.GithubUser.UID = u.Settings.GithubUser.UID
	}

	if err = h.sc.UpdateSettings(u, userSettings); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error saving user settings"))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/users/settings

type userSettingsGetHandler struct{}

func makeFetchUserConfig() gimlet.RouteHandler {
	return &userSettingsGetHandler{}
}

func (h *userSettingsGetHandler) Factory() gimlet.RouteHandler                     { return h }
func (h *userSettingsGetHandler) Parse(ctx context.Context, r *http.Request) error { return nil }

func (h *userSettingsGetHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	apiSettings := model.APIUserSettings{}
	if err := apiSettings.BuildFromService(u.Settings); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error formatting user settings"))
	}

	return gimlet.NewJSONResponse(apiSettings)
}
