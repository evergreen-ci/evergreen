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
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
	return errors.WithStack(utility.ReadJSON(r.Body, &h.settings))
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
	return &userPermissionsPostHandler{
		sc: h.sc,
		rm: h.rm,
	}
}

func (h *userPermissionsPostHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.userID = vars["user_id"]
	if h.userID == "" {
		return errors.New("no user found")
	}
	permissions := RequestedPermissions{}
	if err := utility.ReadJSON(r.Body, &permissions); err != nil {
		return errors.Wrap(err, "request body is not a valid Permissions request")
	}
	if !utility.StringSliceContains(evergreen.ValidResourceTypes, permissions.ResourceType) {
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
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: fmt.Sprintf("can't get user for id '%s'", h.userID)})
	}
	if u == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    fmt.Sprintf("no matching user for '%s'", h.userID),
			StatusCode: http.StatusNotFound,
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

type deletePermissionsRequest struct {
	ResourceType string `json:"resource_type"`
}

const allResourceType = "all"

type userPermissionsDeleteHandler struct {
	sc           data.Connector
	rm           gimlet.RoleManager
	userID       string
	resourceType string
}

func makeDeleteUserPermissions(sc data.Connector, rm gimlet.RoleManager) gimlet.RouteHandler {
	return &userPermissionsPostHandler{
		sc: sc,
		rm: rm,
	}
}

func (h *userPermissionsDeleteHandler) Factory() gimlet.RouteHandler {
	return &userPermissionsDeleteHandler{
		sc: h.sc,
		rm: h.rm,
	}
}

func (h *userPermissionsDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.userID = vars["user_id"]
	if h.userID == "" {
		return errors.New("no user found")
	}
	request := deletePermissionsRequest{}
	if err := utility.ReadJSON(r.Body, &request); err != nil {
		return errors.Wrap(err, "request body is an invalid format")
	}
	h.resourceType = request.ResourceType
	if !utility.StringSliceContains(evergreen.ValidResourceTypes, h.resourceType) && h.resourceType != allResourceType {
		return errors.New("resource_type is not a valid value")
	}

	return nil
}

func (h *userPermissionsDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	u, err := h.sc.FindUserById(h.userID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: fmt.Sprintf("can't get user for id '%s'", h.userID)})
	}
	if u == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    fmt.Sprintf("no matching user for '%s'", h.userID),
			StatusCode: http.StatusNotFound,
		})
	}
	dbUser, valid := u.(*user.DBUser)
	if !valid {
		return gimlet.MakeJSONInternalErrorResponder(errors.New("user exists, but is of invalid type"))
	}

	if h.resourceType == allResourceType {
		err = dbUser.DeleteAllRoles()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "error deleting roles",
			}))
			return gimlet.MakeJSONInternalErrorResponder(errors.New("unable to delete roles"))
		}
		return gimlet.NewJSONResponse(struct{}{})
	}

	roles, err := h.rm.GetRoles(u.Roles())
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting roles",
		}))
		return gimlet.MakeJSONInternalErrorResponder(errors.New("unable to get roles for user"))
	}
	scopesIds := []string{}
	for _, role := range roles {
		scopesIds = append(scopesIds, role.Scope)
	}
	applicableScopes, err := h.rm.FilterScopesByResourceType(scopesIds, h.resourceType)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error filtering scopes",
		}))
		return gimlet.MakeJSONInternalErrorResponder(errors.New("unable to find applicable scopes for user"))
	}
	rolesToRemove := []string{}
	scopesToRemove := []string{}
	for _, scope := range applicableScopes {
		scopesToRemove = append(scopesToRemove, scope.ID)
	}
	for _, role := range roles {
		if utility.StringSliceContains(scopesToRemove, role.Scope) {
			rolesToRemove = append(rolesToRemove, role.ID)
		}
	}
	err = dbUser.DeleteRoles(rolesToRemove)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error deleting roles for user",
		}))
		return gimlet.MakeJSONInternalErrorResponder(errors.New("unable to find delete roles for user"))
	}
	return gimlet.NewJSONResponse(struct{}{})
}

type userPermissionsGetHandler struct {
	sc     data.Connector
	rm     gimlet.RoleManager
	userID string
}

func makeGetUserPermissions(sc data.Connector, rm gimlet.RoleManager) gimlet.RouteHandler {
	return &userPermissionsGetHandler{
		sc: sc,
		rm: rm,
	}
}

func (h *userPermissionsGetHandler) Factory() gimlet.RouteHandler {
	return &userPermissionsGetHandler{
		sc: h.sc,
		rm: h.rm,
	}
}

func (h *userPermissionsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.userID = vars["user_id"]
	if h.userID == "" {
		return errors.New("no user found")
	}
	return nil
}

func (h *userPermissionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	u, err := user.FindOneById(h.userID)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error finding user",
			"route":   "userPermissionsGetHandler",
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.New("problem finding user"))
	}
	if u == nil {
		return gimlet.NewJSONErrorResponse(errors.New("user not found"))
	}
	permissions, err := rolemanager.PermissionSummaryForRoles(ctx, u.Roles(), h.rm)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting permission summary",
			"route":   "userPermissionsGetHandler",
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.New("unable to get permissions for user"))
	}
	return gimlet.NewJSONResponse(permissions)
}
