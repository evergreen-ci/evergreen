package route

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
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
}

func makeSetUserConfig() gimlet.RouteHandler {
	return &userSettingsPostHandler{}
}

func (h *userSettingsPostHandler) Factory() gimlet.RouteHandler {
	return &userSettingsPostHandler{}
}

func (h *userSettingsPostHandler) Parse(ctx context.Context, r *http.Request) error {
	h.settings = model.APIUserSettings{}
	return errors.Wrap(utility.ReadJSON(r.Body, &h.settings), "reading user settings from JSON request body")
}

func (h *userSettingsPostHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	userSettings, err := model.UpdateUserSettings(ctx, u, h.settings)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "updating user settings for user '%s'", u.Username()))
	}

	if err = data.UpdateSettings(u, *userSettings); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "saving updated settings for user '%s'", u.Username()))
	}

	if h.settings.SpruceFeedback != nil {
		h.settings.SpruceFeedback.SubmittedAt = model.ToTimePtr(time.Now())
		h.settings.SpruceFeedback.User = utility.ToStringPtr(u.Username())
		if err = data.SubmitFeedback(*h.settings.SpruceFeedback); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "submitting Spruce feedback"))
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
	apiSettings.BuildFromService(u.Settings)
	return gimlet.NewJSONResponse(apiSettings)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/users/{user_id}

type getUserHandler struct {
	userId string
}

func makeGetUserHandler() gimlet.RouteHandler {
	return &getUserHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get user
//	@Description	Get information about the given user
//	@Tags			users
//	@Router			/users/{user_id} [get]
//	@Security		Api-User || Api-Key
//	@Param			user_id	path		string			true	"User ID"
//	@Success		200		{object}	model.APIDBUser	"the requested user"
func (h *getUserHandler) Factory() gimlet.RouteHandler { return h }
func (h *getUserHandler) Parse(ctx context.Context, r *http.Request) error {
	h.userId = gimlet.GetVars(r)["user_id"]
	return nil
}

func (h *getUserHandler) Run(ctx context.Context) gimlet.Responder {
	usr, err := user.FindOneById(h.userId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding user by ID"))
	}
	if usr == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("user '%s' not found", h.userId),
		})
	}

	return gimlet.NewJSONResponse(model.APIDBUserBuildFromService(*usr))
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/users/{user_id}/permissions

type userPermissionsPostHandler struct {
	rm          gimlet.RoleManager
	userID      string
	permissions RequestedPermissions
}

type RequestedPermissions struct {
	// resource_type - the type of resources for which permission is granted. Must be one of "project", "distro", or "superuser"
	ResourceType string `json:"resource_type"`
	// resources - an array of strings representing what resources the access is for. For a resource_type of project, this will be a list of projects. For a resource_type of distro, this will be a list of distros.
	Resources []string `json:"resources"`
	// permissions - an object whose keys are the permission keys returned by the /permissions endpoint above, and whose values are the levels of access to grant for that permission (also returned by the /permissions endpoint)
	Permissions gimlet.Permissions `json:"permissions"`
}

// Factory creates an instance of the handler.
//
//	@Summary		Give permissions to user
//	@Description	Grants the user specified by user_id the permissions in the request body.
//	@Tags			users
//	@Router			/users/{user_id}/permissions [post]
//	@Security		Api-User || Api-Key
//	@Param			user_id		path	string					true	"the user's ID"
//	@Param			{object}	body	RequestedPermissions	true	"parameters"
//	@Success		200
func makeModifyUserPermissions(rm gimlet.RoleManager) gimlet.RouteHandler {
	return &userPermissionsPostHandler{
		rm: rm,
	}
}

func (h *userPermissionsPostHandler) Factory() gimlet.RouteHandler {
	return &userPermissionsPostHandler{
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
		return errors.Wrap(err, "reading permissions from JSON request body")
	}
	if !utility.StringSliceContains(evergreen.ValidResourceTypes, permissions.ResourceType) {
		return errors.Errorf("invalid resource type '%s'", permissions.ResourceType)
	}
	if len(permissions.Resources) == 0 {
		return errors.New("resources cannot be empty")
	}
	h.permissions = permissions

	return nil
}

func (h *userPermissionsPostHandler) Run(ctx context.Context) gimlet.Responder {
	u, err := user.FindOneById(h.userID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting user '%s'", h.userID))
	}
	if u == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    fmt.Sprintf("user '%s' not found", h.userID),
			StatusCode: http.StatusNotFound,
		})
	}

	newRole, err := rolemanager.MakeRoleWithPermissions(h.rm, h.permissions.ResourceType, h.permissions.Resources, h.permissions.Permissions)
	if err != nil {
		return gimlet.NewTextInternalErrorResponse(err.Error())
	}
	if err = u.AddRole(newRole.ID); err != nil {
		return gimlet.NewTextInternalErrorResponse(err.Error())
	}

	// This is unfortunately very special-casey, but if the user has been granted permission to view/edit
	// project settings, they also need to be given view access for the repo project, if applicable.
	if h.permissions.ResourceType == evergreen.ProjectResourceType &&
		h.permissions.Permissions[evergreen.PermissionProjectSettings] >= evergreen.ProjectSettingsView.Value {
		repoProjectsUpdated := map[string]bool{}
		pRefs, err := serviceModel.FindMergedEnabledProjectRefsByIds(h.permissions.Resources...)
		if err != nil {
			return gimlet.NewTextInternalErrorResponse(fmt.Sprintf(
				"problem checking for repos: %s", err))
		}

		for _, pRef := range pRefs {
			fmt.Println("repo ref ", pRef.RepoRefId)
			if pRef.RepoRefId != "" && !repoProjectsUpdated[pRef.RepoRefId] {
				if err = u.AddRole(serviceModel.GetViewRepoRole(pRef.RepoRefId)); err != nil {
					return gimlet.NewTextInternalErrorResponse(fmt.Sprintf(
						"problem updating repo view permission: %s", err.Error()))
				}
				repoProjectsUpdated[pRef.RepoRefId] = true
			}
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

type deletePermissionsRequest struct {
	//   resource_type - the type of resources for which to delete permissions. Must
	//   be one of "project", "distro", "superuser", or "all". "all" will revoke all
	//   permissions for the user.
	ResourceType string `json:"resource_type"`
	//   resource_id - the resource ID for which to delete permissions.
	//   Required unless deleting all permissions.
	ResourceId string `json:"resource_id"`
}

const allResourceType = "all"

type userPermissionsDeleteHandler struct {
	rm           gimlet.RoleManager
	userID       string
	resourceType string
	resourceId   string
}

func makeDeleteUserPermissions(rm gimlet.RoleManager) gimlet.RouteHandler {
	return &userPermissionsDeleteHandler{
		rm: rm,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Delete user permissions
//	@Description	Deletes all permissions of a given type for a user by deleting their roles of that type for that resource ID. This ignores the Basic Project/Distro Access that is given to all MongoDB employees.
//	@Tags			users
//	@Router			/users/{user_id}/permissions [delete]
//	@Security		Api-User || Api-Key
//	@Param			user_id		path	string						true	"the user's ID"
//	@Param			{object}	body	deletePermissionsRequest	true	"parameters"
//	@Success		200
func (h *userPermissionsDeleteHandler) Factory() gimlet.RouteHandler {
	return &userPermissionsDeleteHandler{
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
		return errors.Wrap(err, "reading delete request from JSON request body")
	}
	h.resourceType = request.ResourceType
	h.resourceId = request.ResourceId
	if !utility.StringSliceContains(evergreen.ValidResourceTypes, h.resourceType) && h.resourceType != allResourceType {
		return errors.Errorf("invalid resource type '%s'", h.resourceType)
	}
	if h.resourceType != allResourceType && h.resourceId == "" {
		return errors.New("must specify a resource ID to delete permissions for unless deleting all permissions")
	}

	return nil
}

func (h *userPermissionsDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	u, err := user.FindOneById(h.userID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding user '%s'", h.userID))
	}
	if u == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    fmt.Sprintf("user '%s' not found", h.userID),
			StatusCode: http.StatusNotFound,
		})
	}

	if h.resourceType == allResourceType {
		err = u.DeleteAllRoles()
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "deleting all roles for user '%s'", u.Username()))
		}
		return gimlet.NewJSONResponse(struct{}{})
	}

	roles, err := h.rm.GetRoles(u.Roles())
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting current roles for user '%s'", u.Username()))
	}
	rolesToCheck := []gimlet.Role{}
	// We don't check basic/superuser access since those are internally maintained.
	for _, r := range roles {
		if !utility.StringSliceContains(evergreen.GeneralAccessRoles, r.ID) {
			rolesToCheck = append(rolesToCheck, r)
		}
	}
	if len(rolesToCheck) == 0 {
		return gimlet.NewJSONResponse(struct{}{})
	}

	rolesForResource, err := h.rm.FilterForResource(rolesToCheck, h.resourceId, h.resourceType)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "filtering user roles for resource '%s'", h.resourceId))
	}
	rolesToRemove := []string{}
	for _, r := range rolesForResource {
		rolesToRemove = append(rolesToRemove, r.ID)
	}

	grip.Info(message.Fields{
		"removed_roles": rolesToRemove,
		"user":          u.Id,
		"resource_type": h.resourceType,
		"resource_id":   h.resourceId,
	})
	err = u.DeleteRoles(rolesToRemove)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "deleting roles for user '%s'", u.Username()))
	}
	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /users/permissions

type UsersPermissionsInput struct {
	// The resource ID
	ResourceId string `json:"resource_id"`
	// The resource type
	ResourceType string `json:"resource_type"`
}

// UserPermissionsResult is a map from userId to their highest permission for the resource
type UsersPermissionsResult map[string]gimlet.Permissions

// Swagger-only type, included because this API route returns an external type
// nolint:all
type swaggerUsersPermissionsResult map[string]swaggerPermissions

type allUsersPermissionsGetHandler struct {
	rm    gimlet.RoleManager
	input UsersPermissionsInput
}

func makeGetAllUsersPermissions(rm gimlet.RoleManager) gimlet.RouteHandler {
	return &allUsersPermissionsGetHandler{
		rm: rm,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get all user permissions for resource
//	@Description	Retrieves all users with permissions for the resource, and their highest permissions, and returns this as a mapping. This ignores basic permissions that are given to all users.
//	@Tags			users
//	@Router			/users/permissions [get]
//	@Security		Api-User || Api-Key
//	@Param			{object}	body		UsersPermissionsInput	true	"parameters"
//	@Success		200			{object}	swaggerUsersPermissionsResult
func (h *allUsersPermissionsGetHandler) Factory() gimlet.RouteHandler {
	return &allUsersPermissionsGetHandler{
		rm: h.rm,
	}
}

func (h *allUsersPermissionsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	err := utility.ReadJSON(r.Body, &h.input)
	if err != nil {
		return errors.Wrap(err, "reading permissions request from JSON request body")
	}
	if !utility.StringSliceContains(evergreen.ValidResourceTypes, h.input.ResourceType) {
		return errors.Errorf("invalid resource type '%s'", h.input.ResourceType)
	}
	if h.input.ResourceId == "" {
		return errors.New("resource ID is required")
	}
	return nil
}

func (h *allUsersPermissionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	// Get roles for resource ID.
	allRoles, err := h.rm.GetAllRoles()
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "getting all roles"))
	}

	roles, err := h.rm.FilterForResource(allRoles, h.input.ResourceId, h.input.ResourceType)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "finding roles for resource '%s'", h.input.ResourceId))
	}
	roleIds := []string{}
	permissionsMap := map[string]gimlet.Permissions{}
	for _, role := range roles {
		// Don't return internal roles.
		if !utility.StringSliceContains(evergreen.GeneralAccessRoles, role.ID) {
			roleIds = append(roleIds, role.ID)
			permissionsMap[role.ID] = role.Permissions
		}
	}
	// Get users with roles.
	usersWithRoles, err := user.FindHumanUsersByRoles(roleIds)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "finding users for roles %v", roleIds))
	}
	// Map from users to their highest permissions.
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
	rm         gimlet.RoleManager
	userID     string
	includeAll bool
}

func makeGetUserPermissions(rm gimlet.RoleManager) gimlet.RouteHandler {
	return &userPermissionsGetHandler{
		rm: rm,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get user permissions
//	@Description	Retrieves all permissions for the user (ignoring basic permissions that are given to all users, unless all=true is included).
//	@Tags			users
//	@Router			/users/{user_id}/permissions [get]
//	@Security		Api-User || Api-Key
//	@Param			user_id	path	string	true	"the user's ID"
//	@Param			all		query	boolean	false	"If included, we will not filter out basic permissions"
//	@Success		200		{array}	swaggerPermissionSummary
func (h *userPermissionsGetHandler) Factory() gimlet.RouteHandler {
	return &userPermissionsGetHandler{
		rm: h.rm,
	}
}

// Swagger-only type, included because this API route returns an external type
// nolint:all
type swaggerPermissionSummary struct {
	//   type - the type of resources for which the listed permissions apply.
	//   Will be "project", "distro", or "superuser"
	Type string `json:"type"`
	//   permissions - an object whose keys are the resources for which the user has
	//   permissions. Note that these objects will often have many keys, since
	//   logged-in users have basic permissions to every project and distro. The
	//   values in the keys are objects representing the permissions that the user
	//   has for that resource, identical to the format of the permissions field in
	//   the POST /users/\<user_id\>/permissions API.
	Permissions swaggerPermissionsForResources `json:"permissions"`
}

// Swagger-only type, included because this API route returns an external type
// nolint:all
type swaggerPermissionsForResources map[string]swaggerPermissions

// Swagger-only type, included because this API route returns an external type
// nolint:all
type swaggerPermissions map[string]int

func (h *userPermissionsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	h.userID = vars["user_id"]
	if h.userID == "" {
		return errors.New("no user found")
	}
	h.includeAll = r.URL.Query().Get("all") == "true"

	return nil
}

func (h *userPermissionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	u, err := user.FindOneById(h.userID)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error finding user",
			"route":   "userPermissionsGetHandler",
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "finding user '%s'", h.userID))
	}
	if u == nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Errorf("user '%s' not found", h.userID))
	}
	rolesToSearch := u.SystemRoles
	if !h.includeAll {
		rolesToSearch, _ = utility.StringSliceSymmetricDifference(u.SystemRoles, evergreen.GeneralAccessRoles)
	}
	// Filter out the roles that everybody has automatically
	permissions, err := rolemanager.PermissionSummaryForRoles(ctx, rolesToSearch, h.rm)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "getting permissions for user '%s'", h.userID))
	}
	// Hidden projects are not meant to be exposed to the user, so we remove them from the response here.
	if err = removeHiddenProjects(permissions); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	return gimlet.NewJSONResponse(permissions)
}

func removeHiddenProjects(permissions []rolemanager.PermissionSummary) error {
	var projectIDs []string
	var projectResourceIndex int
	for i, permission := range permissions {
		if permission.Type == evergreen.ProjectResourceType {
			projectResourceIndex = i
			for projectID := range permission.Permissions {
				projectIDs = append(projectIDs, projectID)
			}
		}
	}
	projectRefs, err := serviceModel.FindProjectRefsByIds(projectIDs...)
	if err != nil {
		return errors.Wrapf(err, "getting projects")
	}
	for _, projectRef := range projectRefs {
		if utility.FromBoolPtr(projectRef.Hidden) {
			delete(permissions[projectResourceIndex].Permissions, projectRef.Id)
		}
	}
	return nil
}

type rolesPostRequest struct {
	// the list of roles to add for the user
	Roles []string `json:"roles"`
	// if true, will also create a shell user document for the user. By default, specifying a user that does not exist will error
	CreateUser bool `json:"create_user"`
}

type userRolesPostHandler struct {
	rm         gimlet.RoleManager
	userID     string
	roles      []string
	createUser bool
}

func makeModifyUserRoles(rm gimlet.RoleManager) gimlet.RouteHandler {
	return &userRolesPostHandler{
		rm: rm,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Give roles to user
//	@Description	Adds the specified roles to the specified user. Attempting to add a duplicate role will result in an error. If you're unsure of what roles you want to add, you probably want to POST To /users/user_id/permissions instead.
//	@Tags			users
//	@Router			/users/{user_id}/roles [post]
//	@Security		Api-User || Api-Key
//	@Param			user_id		path	string				true	"user ID"
//	@Param			{object}	body	rolesPostRequest	true	"parameters"
//	@Success		200
func (h *userRolesPostHandler) Factory() gimlet.RouteHandler {
	return &userRolesPostHandler{
		rm: h.rm,
	}
}

func (h *userRolesPostHandler) Parse(ctx context.Context, r *http.Request) error {
	var request rolesPostRequest
	if err := utility.ReadJSON(r.Body, &request); err != nil {
		return errors.Wrap(err, "reading role modification request from JSON request body")
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
	u, err := user.FindOneById(h.userID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding user '%s'", h.userID))
	}
	if u == nil {
		if h.createUser {
			um := evergreen.GetEnvironment().UserManager()
			newUser := user.DBUser{
				Id:          h.userID,
				SystemRoles: h.roles,
			}
			_, err = um.GetOrCreateUser(&newUser)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "creating new user '%s'", h.userID))
			}
			return gimlet.NewJSONResponse(struct{}{})
		} else {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message:    fmt.Sprintf("user '%s' not found", h.userID),
				StatusCode: http.StatusNotFound,
			})
		}
	}
	dbRoles, err := h.rm.GetRoles(h.roles)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    errors.Wrapf(err, "finding roles for user '%s'", u.Username()).Error(),
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
		if err = u.AddRole(toAdd); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding role '%s' to user '%s'", toAdd, u.Username()))
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

type UsersWithRoleResponse struct {
	Users []*string `json:"users"`
}

type usersWithRoleGetHandler struct {
	role string
}

func makeGetUsersWithRole() gimlet.RouteHandler {
	return &usersWithRoleGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get users for role
//	@Description	Gets a list of users for the specified role
//	@Tags			users
//	@Router			/roles/{role_id}/users [get]
//	@Security		Api-User || Api-Key
//	@Param			role_id	path		string					true	"role ID"
//	@Success		200		{object}	UsersWithRoleResponse	"list of users"
func (h *usersWithRoleGetHandler) Factory() gimlet.RouteHandler {
	return &usersWithRoleGetHandler{}
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
	u *model.APIDBUser
}

func makeUpdateServiceUser() gimlet.RouteHandler {
	return &serviceUserPostHandler{
		u: &model.APIDBUser{},
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Create or update service user
//	@Description	Creates a new service user or updates an existing one (restricted to Evergreen admins).
//	@Tags			admin
//	@Router			/admin/service_users [post]
//	@Security		Api-User || Api-Key
//	@Param			{object}	body	model.APIDBUser	true	"parameters"
//	@Success		200
func (h *serviceUserPostHandler) Factory() gimlet.RouteHandler {
	return &serviceUserPostHandler{
		u: &model.APIDBUser{},
	}
}

func (h *serviceUserPostHandler) Parse(ctx context.Context, r *http.Request) error {
	h.u = &model.APIDBUser{}
	if err := utility.ReadJSON(r.Body, h.u); err != nil {
		return errors.Wrap(err, "reading user from JSON request body")
	}
	if h.u.UserID == nil || *h.u.UserID == "" {
		return errors.New("must specify user ID")
	}
	return nil
}

func (h *serviceUserPostHandler) Run(ctx context.Context) gimlet.Responder {
	if h.u == nil {
		return gimlet.NewJSONErrorResponse("no user read from request body")
	}
	err := data.AddOrUpdateServiceUser(*h.u)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding/updating service user '%s'", utility.FromStringPtr(h.u.UserID)))
	}
	return gimlet.NewJSONResponse(struct{}{})
}

type serviceUserDeleteHandler struct {
	username string
}

func makeDeleteServiceUser() gimlet.RouteHandler {
	return &serviceUserDeleteHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Delete service user
//	@Description	Deletes a service user by its ID (restricted to Evergreen admins).
//	@Tags			admin
//	@Router			/admin/service_users [delete]
//	@Security		Api-User || Api-Key
//	@Param			id	query	string	true	"the user ID"
//	@Success		200
func (h *serviceUserDeleteHandler) Factory() gimlet.RouteHandler {
	return &serviceUserDeleteHandler{}
}

func (h *serviceUserDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.username = r.FormValue("id")
	if h.username == "" {
		return errors.New("user ID must be specified")
	}

	return nil
}

func (h *serviceUserDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	err := user.DeleteServiceUser(ctx, h.username)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "deleting service user '%s'", h.username))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

type serviceUsersGetHandler struct {
}

func makeGetServiceUsers() gimlet.RouteHandler {
	return &serviceUsersGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get all service users
//	@Description	Fetches all service users (restricted to Evergreen admins).
//	@Tags			admin
//	@Router			/admin/service_users [get]
//	@Security		Api-User || Api-Key
//	@Success		200	{object}	[]model.APIDBUser
func (h *serviceUsersGetHandler) Factory() gimlet.RouteHandler {
	return &serviceUsersGetHandler{}
}

func (h *serviceUsersGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *serviceUsersGetHandler) Run(ctx context.Context) gimlet.Responder {
	users, err := data.GetServiceUsers()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting all service users"))
	}

	return gimlet.NewJSONResponse(users)
}

////////////////////////////////////////////////////////////////////////
//
// POST /users/rename_user

func makeRenameUser(env evergreen.Environment) gimlet.RouteHandler {
	return &renameUserHandler{
		env: env,
	}
}

type renameUserHandler struct {
	oldUsr   *user.DBUser
	newEmail string
	env      evergreen.Environment
}

// Factory creates an instance of the handler.
//
//	@Summary		Rename user
//	@Description	Migrate a user to a new username. Note that this may overwrite settings on the new user if it already exists.
//	@Tags			users
//	@Router			/users/rename_user [post]
//	@Security		Api-User || Api-Key
//	@Param			{object}	body	renameUserInfo	true	"parameters"
//	@Success		200
func (h *renameUserHandler) Factory() gimlet.RouteHandler {
	return &renameUserHandler{
		env: h.env,
	}
}

type renameUserInfo struct {
	// The old email of the user
	Email string `json:"email" bson:"email" validate:"required"`

	// The new email of the user
	NewEmail string `json:"new_email" bson:"new_email" validate:"required"`
}

func (h *renameUserHandler) Parse(ctx context.Context, r *http.Request) error {
	input := renameUserInfo{}
	err := utility.ReadJSON(r.Body, &input)
	if err != nil {
		return errors.Wrap(err, "reading user offboarding information from JSON request body")
	}
	if len(input.Email) == 0 {
		return errors.New("missing email")
	}
	splitString := strings.Split(input.Email, "@")
	if len(splitString) != 2 {
		return errors.New("email address is missing '@'")
	}
	username := splitString[0]
	if username == "" {
		return errors.New("no user could be parsed from the email address")
	}
	h.oldUsr, err = user.FindOneById(username)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrapf(err, "finding user '%s'", username).Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}
	if h.oldUsr == nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("user '%s' not found", username),
			StatusCode: http.StatusNotFound,
		}
	}
	h.newEmail = input.NewEmail
	return nil
}

func (h *renameUserHandler) Run(ctx context.Context) gimlet.Responder {
	// Need to unset the GitHub UID because our index enforces uniqueness.
	// Assuming that we're able to upsert the user, we update the settings with this UID later.
	githubUID := h.oldUsr.Settings.GithubUser.UID
	h.oldUsr.Settings.GithubUser.UID = 0

	newUsr, err := user.UpsertOneFromExisting(h.oldUsr, h.newEmail)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(err)
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(user.ClearUser(h.oldUsr.Id))
	newUsr.Settings.GithubUser.UID = githubUID
	catcher.Add(newUsr.UpdateSettings(newUsr.Settings))

	catcher.Add(patch.ConsolidatePatchesForUser(h.oldUsr.Id, newUsr))
	catcher.Add(host.ConsolidateHostsForUser(ctx, h.oldUsr.Id, newUsr.Id))

	if catcher.HasErrors() {
		err := catcher.Resolve()
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "users not fully consolidated",
			"old_user": h.oldUsr.Id,
			"new_user": newUsr.Id,
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "consolidating new user '%s' with old user '%s'",
			newUsr.Id, h.oldUsr.Id))
	}
	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /users/offboard_user

func makeOffboardUser(env evergreen.Environment) gimlet.RouteHandler {
	return &offboardUserHandler{
		env: env,
	}
}

type offboardUserHandler struct {
	user   string
	dryRun bool

	env evergreen.Environment
}

// Factory creates an instance of the handler.
//
//	@Summary		Offboard user
//	@Description	Marks unexpirable volumes and hosts as expirable for the user, and removes the user as a project admin for any projects, if applicable.
//	@Tags			users
//	@Router			/users/offboard_user [post]
//	@Security		Api-User || Api-Key
//	@Param			dry_run		query		boolean				false	"If set to true, route returns the IDs of the hosts/volumes that *would* be modified."
//	@Param			{object}	body		offboardUserEmail	true	"parameters"
//	@Success		200			{object}	model.APIOffboardUserResults
func (ch *offboardUserHandler) Factory() gimlet.RouteHandler {
	return &offboardUserHandler{
		env: ch.env,
	}
}

type offboardUserEmail struct {
	// the email of the user
	Email string `json:"email" bson:"email" validate:"required"`
}

func (ch *offboardUserHandler) Parse(ctx context.Context, r *http.Request) error {
	input := offboardUserEmail{}
	err := utility.ReadJSON(r.Body, &input)
	if err != nil {
		return errors.Wrap(err, "reading user offboarding information from JSON request body")
	}
	if len(input.Email) == 0 {
		return errors.New("missing email")
	}
	splitString := strings.Split(input.Email, "@")
	if len(splitString) != 2 {
		return errors.New("email address is missing '@'")
	}
	ch.user = splitString[0]
	if ch.user == "" {
		return errors.New("no user could be parsed from the email address")
	}
	u, err := user.FindOneById(ch.user)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrapf(err, "finding user '%s'", ch.user).Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}
	if u == nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("user '%s' not found", ch.user),
			StatusCode: http.StatusNotFound,
		}
	}

	vals := r.URL.Query()
	ch.dryRun = vals.Get("dry_run") == "true"

	return nil
}

func (ch *offboardUserHandler) Run(ctx context.Context) gimlet.Responder {
	opts := model.APIHostParams{
		UserSpawned: true,
	}
	hosts, err := data.FindHostsInRange(ctx, opts, ch.user)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "getting user hosts from options"))
	}

	volumes, err := host.FindVolumesByUser(ch.user)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "finding user volumes"))
	}

	toTerminate := model.APIOffboardUserResults{
		TerminatedHosts:   []string{},
		TerminatedVolumes: []string{},
	}

	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		if h.NoExpiration {
			if !ch.dryRun {
				catcher.Wrapf(h.MarkShouldExpire(ctx, ""), "marking host '%s' expirable", h.Id)
			}
			toTerminate.TerminatedHosts = append(toTerminate.TerminatedHosts, h.Id)
		}
	}

	for _, v := range volumes {
		if v.NoExpiration {
			if !ch.dryRun {
				catcher.Wrapf(v.SetNoExpiration(false), "marking volume '%s' expirable", v.ID)
			}
			toTerminate.TerminatedVolumes = append(toTerminate.TerminatedVolumes, v.ID)
		}
	}

	if !ch.dryRun {
		grip.Info(message.Fields{
			"message":            "executing user offboarding",
			"user":               ch.user,
			"terminated_hosts":   toTerminate.TerminatedHosts,
			"terminated_volumes": toTerminate.TerminatedVolumes,
		})

		grip.Error(message.WrapError(serviceModel.RemoveAdminFromProjects(ch.user), message.Fields{
			"message": "could not remove user as an admin",
			"context": "user offboarding",
			"user":    ch.user,
		}))

		grip.Error(message.WrapError(ch.clearLogin(), message.Fields{
			"message": "could not clear login token",
			"context": "user offboarding",
			"user":    ch.user,
		}))
		err = user.ClearUser(ch.user)
		catcher.Wrapf(err, "clearing user '%s'", ch.user)
	}

	if catcher.HasErrors() {
		err := catcher.Resolve()
		grip.CriticalWhen(!ch.dryRun, message.WrapError(err, message.Fields{
			"message": "the user did not offboard fully",
			"context": "user offboarding",
			"user":    ch.user,
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "offboarding user '%s'", ch.user))
	}

	return gimlet.NewJSONResponse(toTerminate)
}

// clearLogin invalidates the user's login session.
func (ch *offboardUserHandler) clearLogin() error {
	usrMngr := ch.env.UserManager()
	if usrMngr == nil {
		return errors.New("no user manager found in environment")
	}
	usr, err := usrMngr.GetUserByID(ch.user)
	if err != nil {
		return errors.Wrap(err, "finding user")
	}
	return errors.Wrap(usrMngr.ClearUser(usr, false), "clearing login cache")
}
