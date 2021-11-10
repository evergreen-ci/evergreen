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
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"
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

	userSettings, err := model.UpdateUserSettings(ctx, u, h.settings)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	if err = h.sc.UpdateSettings(u, *userSettings); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error saving user settings"))
	}

	if h.settings.SpruceFeedback != nil {
		h.settings.SpruceFeedback.SubmittedAt = model.ToTimePtr(time.Now())
		h.settings.SpruceFeedback.User = utility.ToStringPtr(u.Username())
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
	if dbuser == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    fmt.Sprintf("no matching DB user for '%s'", h.userID),
			StatusCode: http.StatusNotFound,
		})
	}
	if err = dbuser.AddRole(newRole.ID); err != nil {
		return gimlet.NewTextInternalErrorResponse(err.Error())
	}

	return gimlet.NewJSONResponse(struct{}{})
}

type deletePermissionsRequest struct {
	ResourceType string `json:"resource_type"`
	ResourceId   string `json:"resource_id"`
}

const allResourceType = "all"

type userPermissionsDeleteHandler struct {
	sc           data.Connector
	rm           gimlet.RoleManager
	userID       string
	resourceType string
	resourceId   string
}

func makeDeleteUserPermissions(sc data.Connector, rm gimlet.RoleManager) gimlet.RouteHandler {
	return &userPermissionsDeleteHandler{
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
	h.resourceId = request.ResourceId
	if !utility.StringSliceContains(evergreen.ValidResourceTypes, h.resourceType) && h.resourceType != allResourceType {
		return errors.New("resource_type is not a valid value")
	}
	if h.resourceType != allResourceType && h.resourceId == "" {
		return errors.New("Must specify a resource ID to delete permissions for unless deleting all permissions")
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
	if dbUser == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    fmt.Sprintf("no matching DB user for '%s'", h.userID),
			StatusCode: http.StatusNotFound,
		})
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
	rolesToCheck := []gimlet.Role{}
	// don't remove basic access, just special access
	for _, r := range roles {
		if !utility.StringSliceContains(evergreen.BasicAccessRoles, r.ID) {
			rolesToCheck = append(rolesToCheck, r)
		}
	}
	if len(rolesToCheck) == 0 {
		gimlet.NewJSONResponse(struct{}{})
	}

	rolesForResource, err := h.rm.FilterForResource(rolesToCheck, h.resourceId, h.resourceType)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(
			errors.Wrapf(err, "unable to filter user roles for resource ID '%s'", h.resourceId))
	}
	rolesToRemove := []string{}
	for _, r := range rolesForResource {
		rolesToRemove = append(rolesToRemove, r.ID)
	}

	grip.Info(message.Fields{
		"removed_roles": rolesToRemove,
		"user":          dbUser.Id,
		"resource_type": h.resourceType,
		"resource_id":   h.resourceId,
	})
	err = dbUser.DeleteRoles(rolesToRemove)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error deleting roles for user",
		}))
		return gimlet.MakeJSONInternalErrorResponder(errors.New("unable to find delete roles for user"))
	}
	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /users/permissions

type UsersPermissionsInput struct {
	ResourceId   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
}

// UserPermissionsResult is a map from userId to their highest permission for the resource
type UsersPermissionsResult map[string]gimlet.Permissions

type allUsersPermissionsGetHandler struct {
	sc    data.Connector
	rm    gimlet.RoleManager
	input UsersPermissionsInput
}

func makeGetAllUsersPermissions(sc data.Connector, rm gimlet.RoleManager) gimlet.RouteHandler {
	return &allUsersPermissionsGetHandler{
		sc: sc,
		rm: rm,
	}
}

func (h *allUsersPermissionsGetHandler) Factory() gimlet.RouteHandler {
	return &allUsersPermissionsGetHandler{
		sc: h.sc,
		rm: h.rm,
	}
}

func (h *allUsersPermissionsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	err := utility.ReadJSON(r.Body, &h.input)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("error parsing request body: %s", err.Error()),
		}
	}
	if !utility.StringSliceContains(evergreen.ValidResourceTypes, h.input.ResourceType) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("resource_type must be in %v", evergreen.ValidResourceTypes),
		}
	}
	if h.input.ResourceId == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "resource_id is required",
		}
	}
	return nil
}

func (h *allUsersPermissionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	// get roles for resource ID
	allRoles, err := h.rm.GetAllRoles()
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "error getting roles"))
	}

	roles, err := h.rm.FilterForResource(allRoles, h.input.ResourceId, h.input.ResourceType)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "error finding roles for resource"))
	}
	roleIds := []string{}
	permissionsMap := map[string]gimlet.Permissions{}
	for _, role := range roles {
		// don't include basic roles
		if !utility.StringSliceContains(evergreen.BasicAccessRoles, role.ID) {
			roleIds = append(roleIds, role.ID)
			permissionsMap[role.ID] = role.Permissions
		}
	}
	// get users with roles
	usersWithRoles, err := user.FindByRoles(roleIds)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "error finding users for roles '%v'", roleIds))
	}
	// map from users to their highest permissions
	res := UsersPermissionsResult{}
	for _, u := range usersWithRoles {
		for _, userRole := range u.SystemRoles {
			permissions, ok := permissionsMap[userRole]
			if ok {
				res[u.Username()] = getMaxPermissions(res[u.Username()], permissions)
			}
		}
	}

	return gimlet.NewJSONResponse(res)
}

func getMaxPermissions(p1, p2 gimlet.Permissions) gimlet.Permissions {
	res := gimlet.Permissions{}
	if p1 != nil {
		res = p1
	}
	for key, val := range p2 {
		if res[key] < val {
			res[key] = val
		}
	}
	return res
}

////////////////////////////////////////////////////////////////////////
//
// GET /users/{user_id}/permissions

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
	rolesToSearch, _ := utility.StringSliceSymmetricDifference(u.SystemRoles, evergreen.BasicAccessRoles)
	// filter out the roles that everybody has automatically
	permissions, err := rolemanager.PermissionSummaryForRoles(ctx, rolesToSearch, h.rm)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting permission summary",
			"route":   "userPermissionsGetHandler",
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.New("unable to get permissions for user"))
	}
	return gimlet.NewJSONResponse(permissions)
}

type rolesPostRequest struct {
	Roles      []string `json:"roles"`
	CreateUser bool     `json:"create_user"`
}

type userRolesPostHandler struct {
	sc         data.Connector
	rm         gimlet.RoleManager
	userID     string
	roles      []string
	createUser bool
}

func makeModifyUserRoles(sc data.Connector, rm gimlet.RoleManager) gimlet.RouteHandler {
	return &userRolesPostHandler{
		sc: sc,
		rm: rm,
	}
}

func (h *userRolesPostHandler) Factory() gimlet.RouteHandler {
	return &userRolesPostHandler{
		sc: h.sc,
		rm: h.rm,
	}
}

func (h *userRolesPostHandler) Parse(ctx context.Context, r *http.Request) error {
	var request rolesPostRequest
	if err := utility.ReadJSON(r.Body, &request); err != nil {
		return errors.Wrap(err, "request body is malformed")
	}
	if len(request.Roles) == 0 {
		return errors.New("must specify at least 1 role to add")
	}
	h.roles = request.Roles
	h.createUser = request.CreateUser
	vars := gimlet.GetVars(r)
	h.userID = vars["user_id"]

	return nil
}

func (h *userRolesPostHandler) Run(ctx context.Context) gimlet.Responder {
	u, err := h.sc.FindUserById(h.userID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: fmt.Sprintf("can't get user for id '%s'", h.userID)})
	}
	dbUser, valid := u.(*user.DBUser)
	if !valid {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "unexpected structure for user",
			StatusCode: http.StatusInternalServerError,
		})
	}
	if dbUser == nil {
		if h.createUser {
			um := evergreen.GetEnvironment().UserManager()
			newUser := user.DBUser{
				Id:          h.userID,
				SystemRoles: h.roles,
			}
			_, err = um.GetOrCreateUser(&newUser)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "unable to create user"))
			}
			return gimlet.NewJSONResponse(struct{}{})
		} else {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message:    fmt.Sprintf("no matching user for '%s'", h.userID),
				StatusCode: http.StatusNotFound,
			})
		}
	}
	dbRoles, err := h.rm.GetRoles(h.roles)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error finding roles",
		}))
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "no roles found",
			StatusCode: http.StatusNotFound,
		})
	}
	foundRoles := []string{}
	for _, found := range dbRoles {
		foundRoles = append(foundRoles, found.ID)
	}
	nonexistent, _ := utility.StringSliceSymmetricDifference(h.roles, foundRoles)
	if len(nonexistent) > 0 {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    fmt.Sprintf("roles not found: %v", nonexistent),
			StatusCode: http.StatusNotFound,
		})
	}
	for _, toAdd := range h.roles {
		if err = dbUser.AddRole(toAdd); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "unable to add role",
				"role":    toAdd,
			}))
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message:    "error adding role",
				StatusCode: http.StatusInternalServerError,
			})
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

type UsersWithRoleResponse struct {
	Users []*string `json:"users"`
}

type usersWithRoleGetHandler struct {
	sc   data.Connector
	role string
}

func makeGetUsersWithRole(sc data.Connector) gimlet.RouteHandler {
	return &usersWithRoleGetHandler{
		sc: sc,
	}
}

func (h *usersWithRoleGetHandler) Factory() gimlet.RouteHandler {
	return &usersWithRoleGetHandler{
		sc: h.sc,
	}
}

func (h *usersWithRoleGetHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.role = vars["role_id"]
	return nil
}

func (h *usersWithRoleGetHandler) Run(ctx context.Context) gimlet.Responder {
	users, err := user.FindByRole(h.role)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	res := []*string{}
	for idx := range users {
		res = append(res, &users[idx].Id)
	}
	return gimlet.NewJSONResponse(&UsersWithRoleResponse{Users: res})
}

type serviceUserPostHandler struct {
	sc data.Connector
	u  *model.APIDBUser
}

func makeUpdateServiceUser(sc data.Connector) gimlet.RouteHandler {
	return &serviceUserPostHandler{
		sc: sc,
		u:  &model.APIDBUser{},
	}
}

func (h *serviceUserPostHandler) Factory() gimlet.RouteHandler {
	return &serviceUserPostHandler{
		sc: h.sc,
		u:  &model.APIDBUser{},
	}
}

func (h *serviceUserPostHandler) Parse(ctx context.Context, r *http.Request) error {
	h.u = &model.APIDBUser{}
	if err := utility.ReadJSON(r.Body, h.u); err != nil {
		return errors.Wrap(err, "request body is malformed")
	}
	if h.u.UserID == nil || *h.u.UserID == "" {
		return errors.New("user_id must be specified")
	}
	return nil
}

func (h *serviceUserPostHandler) Run(ctx context.Context) gimlet.Responder {
	if h.u == nil {
		return gimlet.NewJSONInternalErrorResponse("error reading request body")
	}
	err := h.sc.AddOrUpdateServiceUser(*h.u)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(struct{}{})
}

type serviceUserDeleteHandler struct {
	sc       data.Connector
	username string
}

func makeDeleteServiceUser(sc data.Connector) gimlet.RouteHandler {
	return &serviceUserDeleteHandler{
		sc: sc,
	}
}

func (h *serviceUserDeleteHandler) Factory() gimlet.RouteHandler {
	return &serviceUserDeleteHandler{
		sc: h.sc,
	}
}

func (h *serviceUserDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.username = r.FormValue("id")
	if h.username == "" {
		return errors.New("'id' must be specified")
	}

	return nil
}

func (h *serviceUserDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	err := h.sc.DeleteServiceUser(h.username)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}

type serviceUsersGetHandler struct {
	sc data.Connector
}

func makeGetServiceUsers(sc data.Connector) gimlet.RouteHandler {
	return &serviceUsersGetHandler{
		sc: sc,
	}
}

func (h *serviceUsersGetHandler) Factory() gimlet.RouteHandler {
	return &serviceUsersGetHandler{
		sc: h.sc,
	}
}

func (h *serviceUsersGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *serviceUsersGetHandler) Run(ctx context.Context) gimlet.Responder {
	users, err := h.sc.GetServiceUsers()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse(users)
}
