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
	"github.com/evergreen-ci/gimlet/rolemanager"
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
		userSettings.GithubUser.UID = int(*ghUser.ID)
	} else {
		userSettings.GithubUser.UID = u.Settings.GithubUser.UID
	}

	if err = h.sc.UpdateSettings(u, userSettings); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error saving user settings"))
	}

	if h.settings.SpruceFeedback != nil {
		h.settings.SpruceFeedback.SubmittedAt = model.ToTimePtr(time.Now())
		h.settings.SpruceFeedback.User = model.ToStringPtr(u.Username())
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

type userPermissionsPostHandler struct {
	sc          data.Connector
	rm          gimlet.RoleManager
	userID      string
	permissions RequestedPermissions
}

type RequestedPermissions struct {
	ResourceType string             `json:"resource_type"`
	Resources    []string           `json:"resources"`
	Permissions  gimlet.Permissions `json:"permissions"`
}

func makeModifyUserPermissions(sc data.Connector, rm gimlet.RoleManager) gimlet.RouteHandler {
	return &userPermissionsPostHandler{
		sc: sc,
		rm: rm,
	}
}

func (h *userPermissionsPostHandler) Factory() gimlet.RouteHandler {
	return &userAuthorGetHandler{
		sc: h.sc,
	}
}

func (h *userPermissionsPostHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.userID = vars["user_id"]
	if h.userID == "" {
		return errors.New("no user found")
	}
	permissions := RequestedPermissions{}
	if err := util.ReadJSONInto(r.Body, &permissions); err != nil {
		return errors.Wrap(err, "request body is not a valid Permissions request")
	}
	if !util.StringSliceContains(evergreen.ValidResourceTypes, permissions.ResourceType) {
		return errors.Errorf("'%s' is not a valid resource_type", permissions.ResourceType)
	}
	if len(permissions.Resources) == 0 {
		return errors.New("resources cannot be empty")
	}
	h.permissions = permissions

	return nil
}

func (h *userPermissionsPostHandler) Run(ctx context.Context) gimlet.Responder {
	u, err := h.sc.FindUserById(h.userID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("can't get user for id '%s'", h.userID)})
	}
	if u == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    fmt.Sprintf("no matching user for '%s'", h.userID),
			StatusCode: 404,
		})
	}

	newRole, err := rolemanager.MakeRoleWithPermissions(h.rm, h.permissions.ResourceType, h.permissions.Resources, h.permissions.Permissions)
	if err != nil {
		return gimlet.NewTextInternalErrorResponse(err.Error())
	}
	dbuser, valid := u.(*user.DBUser)
	if !valid {
		return gimlet.NewTextInternalErrorResponse("unexpected type of user found")
	}
	if err = dbuser.AddRole(newRole.ID); err != nil {
		return gimlet.NewTextInternalErrorResponse(err.Error())
	}

	return gimlet.NewJSONResponse(struct{}{})
}
