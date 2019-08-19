package route

import (
	"context"
	"fmt"
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
// POST /rest/v2/user/settings

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
	return errors.Wrap(util.ReadJSONInto(r.Body, &h.settings), "error parsing request body")
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

	if h.settings.SpruceFeedback != nil {
		h.settings.SpruceFeedback.SubmittedAt = time.Now()
		h.settings.SpruceFeedback.User = model.ToAPIString(u.Username())
		if err = h.sc.SubmitFeedback(*h.settings.SpruceFeedback); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/user/settings

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

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/user/author

type userAuthorGetHandler struct {
	sc     data.Connector
	userID string
}

func makeFetchUserAuthor(sc data.Connector) gimlet.RouteHandler {
	return &userAuthorGetHandler{
		sc: sc,
	}
}

func (h *userAuthorGetHandler) Factory() gimlet.RouteHandler {
	return &userAuthorGetHandler{
		sc: h.sc,
	}
}

func (h *userAuthorGetHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.userID = vars["user_id"]
	return nil
}

func (h *userAuthorGetHandler) Run(ctx context.Context) gimlet.Responder {
	user, err := h.sc.FindUserById(h.userID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't get user for id '%s'", h.userID))
	}
	if user == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    fmt.Sprintf("no matching user for '%s'", h.userID),
			StatusCode: 404,
		})
	}

	apiAuthor := model.APIUserAuthorInformation{}
	if err := apiAuthor.BuildFromService(user); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error formatting user author information"))
	}

	return gimlet.NewJSONResponse(apiAuthor)
}

type getAllRolesHandler struct {
	sc data.Connector
}

func makeGetAllRolesHandler(sc data.Connector) gimlet.RouteHandler {
	return &getAllRolesHandler{
		sc: sc,
	}
}

func (h *getAllRolesHandler) Factory() gimlet.RouteHandler {
	return &getAllRolesHandler{
		sc: h.sc,
	}
}

func (h *getAllRolesHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *getAllRolesHandler) Run(ctx context.Context) gimlet.Responder {
	roles, err := h.sc.GetAllRoles()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse(roles)
}

type updateRoleHandler struct {
	sc   data.Connector
	role *model.APIRole
}

func makeUpdateRoleHandler(sc data.Connector) gimlet.RouteHandler {
	return &updateRoleHandler{
		sc: sc,
	}
}

func (h *updateRoleHandler) Factory() gimlet.RouteHandler {
	return &updateRoleHandler{
		sc: h.sc,
	}
}

func (h *updateRoleHandler) Parse(ctx context.Context, r *http.Request) error {
	h.role = &model.APIRole{}
	err := util.ReadJSONInto(r.Body, h.role)
	if err != nil {
		return errors.Wrap(err, "error reading request body")
	}
	if h.role.Id == model.ToAPIString("") || h.role.Id == nil {
		return errors.New("role ID must be set")
	}
	return nil
}

func (h *updateRoleHandler) Run(ctx context.Context) gimlet.Responder {
	err := h.sc.UpdateRole(h.role)
	if err != nil {
		gimlet.NewJSONErrorResponse(err)
	}

	return gimlet.NewJSONResponse(h.role)
}
