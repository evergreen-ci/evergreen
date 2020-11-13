package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type (
	// custom type used to attach specific values to request contexts, to prevent collisions.
	requestContextKey int
)

const (
	// Key value used to map user and project data to request context.
	// These are private custom types to avoid key collisions.
	RequestContext requestContextKey = 0
)

type projCtxMiddleware struct {
	sc data.Connector
}

func (m *projCtxMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	buildId := vars["build_id"]
	versionId := vars["version_id"]
	patchId := vars["patch_id"]
	projectId := vars["project_id"]

	opCtx, err := m.sc.FetchContext(taskId, buildId, versionId, patchId, projectId)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	user := gimlet.GetUser(ctx)

	if opCtx.ProjectRef != nil && opCtx.ProjectRef.Private && user == nil {
		// Project is private and user is not authorized so return not found
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "Project not found",
		}))
		return
	}

	if opCtx.Patch != nil && user == nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "Not found",
		}))
		return
	}

	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	next(rw, r)
}

func NewProjectContextMiddleware(sc data.Connector) gimlet.Middleware {
	return &projCtxMiddleware{
		sc: sc,
	}
}

// GetProjectContext returns the project context associated with a
// given request.
func GetProjectContext(ctx context.Context) *model.Context {
	p, _ := ctx.Value(RequestContext).(*model.Context)
	return p
}

// MustHaveProjectContext returns the project context set on the
// http request context. It panics if none is set.
func MustHaveProjectContext(ctx context.Context) *model.Context {
	pc := GetProjectContext(ctx)
	if pc == nil {
		panic("project context not attached to request")
	}
	return pc
}

// MustHaveUser returns the user associated with a given request or panics
// if none is present.
func MustHaveUser(ctx context.Context) *user.DBUser {
	u := gimlet.GetUser(ctx)
	if u == nil {
		panic("no user attached to request")
	}
	usr, ok := u.(*user.DBUser)
	if !ok {
		panic("malformed user attached to request")
	}

	return usr
}

func validPriority(priority int64, project string, user gimlet.User, sc data.Connector) bool {
	if priority > evergreen.MaxTaskPriority {
		return user.HasPermission(gimlet.PermissionOpts{
			Resource:      project,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionTasks,
			RequiredLevel: evergreen.TasksAdmin.Value,
		})
	}
	return true
}

func NewProjectAdminMiddleware(sc data.Connector) gimlet.Middleware {
	return &projectAdminMiddleware{
		sc: sc,
	}
}

type projectAdminMiddleware struct {
	sc data.Connector
}

func (m *projectAdminMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	opCtx := MustHaveProjectContext(ctx)
	user := MustHaveUser(ctx)

	if opCtx == nil || opCtx.ProjectRef == nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "No project found",
		}))
		return
	}

	isAdmin := utility.StringSliceContains(opCtx.ProjectRef.Admins, user.Username()) || user.HasPermission(gimlet.PermissionOpts{
		Resource:      opCtx.ProjectRef.Id,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	})
	if !isAdmin {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "Not authorized",
		}))
		return
	}

	next(rw, r)
}

// NewTaskHostAuthMiddleware returns route middleware that authenticates a host
// created by a task and verifies the secret of the host that created this host.
func NewTaskHostAuthMiddleware(sc data.Connector) gimlet.Middleware {
	return &TaskHostAuthMiddleware{
		sc: sc,
	}
}

type TaskHostAuthMiddleware struct {
	sc data.Connector
}

func (m *TaskHostAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	vars := gimlet.GetVars(r)
	hostID, ok := vars["host_id"]
	if !ok {
		hostID = r.Header.Get(evergreen.HostHeader)
		if hostID == "" {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "Not authorized",
			}))
			return
		}
	}
	h, err := m.sc.FindHostById(hostID)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}
	if h == nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with id '%s' not found", hostID),
		}))
		return
	}

	if h.StartedBy == "" {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Host was not started by task",
		}))
		return
	}
	t, err := m.sc.FindTaskById(h.StartedBy)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}
	if code, err := m.sc.CheckHostSecret(t.HostId, r); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: code,
			Message:    err.Error(),
		}))
		return
	}

	next(rw, r)
}

func NewTaskAuthMiddleware(sc data.Connector) gimlet.Middleware {
	return &TaskAuthMiddleware{
		sc: sc,
	}
}

type TaskAuthMiddleware struct {
	sc data.Connector
}

func (m *TaskAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	vars := gimlet.GetVars(r)
	taskID, ok := vars["task_id"]
	if !ok {
		taskID = r.Header.Get(evergreen.TaskHeader)
		if taskID == "" {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "Not authorized",
			}))
			return
		}
	}

	if code, err := m.sc.CheckTaskSecret(taskID, r); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: code,
			Message:    err.Error(),
		}))
		return
	}
	if code, err := m.sc.CheckHostSecret("", r); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: code,
			Message:    err.Error(),
		}))
		return
	}

	next(rw, r)
}

func NewCommitQueueItemOwnerMiddleware(sc data.Connector) gimlet.Middleware {
	return &CommitQueueItemOwnerMiddleware{
		sc: sc,
	}
}

type CommitQueueItemOwnerMiddleware struct {
	sc data.Connector
}

func (m *CommitQueueItemOwnerMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	user := MustHaveUser(ctx)
	opCtx := MustHaveProjectContext(ctx)
	projRef, err := opCtx.GetProjectRef()
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}))
		return
	}

	if !projRef.CommitQueue.Enabled {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Commit queue is not enabled for project",
		}))
		return
	}

	// A superuser or project admin is authorized
	isAdmin := utility.StringSliceContains(projRef.Admins, user.Username()) || user.HasPermission(gimlet.PermissionOpts{
		Resource:      opCtx.ProjectRef.Id,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	})
	if isAdmin {
		next(rw, r)
		return
	}

	// The owner of the patch can also pass
	vars := gimlet.GetVars(r)
	item, ok := vars["item"]
	if !ok {
		item, ok = vars["patch_id"]
	}
	if !ok || item == "" {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "No item provided",
		}))
		return
	}

	if projRef.CommitQueue.PatchType == commitqueue.CLIPatchType {
		patch, err := m.sc.FindPatchById(item)
		if err != nil {
			gimlet.WriteResponse(rw, gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't find item")))
			return
		}
		if user.Id != *patch.Author {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "Not authorized",
			}))
			return
		}
	}

	if projRef.CommitQueue.PatchType == commitqueue.PRPatchType {
		itemInt, err := strconv.Atoi(item)
		if err != nil {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "item is not an integer",
			}))
			return
		}

		pr, err := m.sc.GetGitHubPR(ctx, projRef.Owner, projRef.Repo, itemInt)
		if err != nil {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "can't get information about PR",
			}))
			return
		}

		var githubUID int
		if pr != nil && pr.User != nil && pr.User.ID != nil {
			githubUID = int(*pr.User.ID)
		}
		if githubUID == 0 || user.Settings.GithubUser.UID != githubUID {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "Not authorized",
			}))
			return
		}
	}

	next(rw, r)
}

func RequiresProjectPermission(permission string, level evergreen.PermissionLevel) gimlet.Middleware {
	defaultRoles, err := evergreen.GetEnvironment().RoleManager().GetRoles(evergreen.UnauthedUserRoles)
	if err != nil {
		grip.Critical(message.WrapError(err, message.Fields{
			"message": "unable to get default roles",
		}))
	}

	opts := gimlet.RequiresPermissionMiddlewareOpts{
		RM:            evergreen.GetEnvironment().RoleManager(),
		PermissionKey: permission,
		ResourceType:  evergreen.ProjectResourceType,
		RequiredLevel: level.Value,
		ResourceFunc:  urlVarsToProjectScopes,
		DefaultRoles:  defaultRoles,
	}

	return gimlet.RequiresPermission(opts)
}

func RequiresDistroPermission(permission string, level evergreen.PermissionLevel) gimlet.Middleware {
	defaultRoles, err := evergreen.GetEnvironment().RoleManager().GetRoles(evergreen.UnauthedUserRoles)
	if err != nil {
		grip.Critical(message.WrapError(err, message.Fields{
			"message": "unable to get default roles",
		}))
	}

	opts := gimlet.RequiresPermissionMiddlewareOpts{
		RM:            evergreen.GetEnvironment().RoleManager(),
		PermissionKey: permission,
		ResourceType:  evergreen.DistroResourceType,
		RequiredLevel: level.Value,
		ResourceFunc:  urlVarsToDistroScopes,
		DefaultRoles:  defaultRoles,
	}
	return gimlet.RequiresPermission(opts)
}

func RequiresSuperUserPermission(permission string, level evergreen.PermissionLevel) gimlet.Middleware {
	defaultRoles, err := evergreen.GetEnvironment().RoleManager().GetRoles(evergreen.UnauthedUserRoles)
	if err != nil {
		grip.Critical(message.WrapError(err, message.Fields{
			"message": "unable to get default roles",
		}))
	}

	opts := gimlet.RequiresPermissionMiddlewareOpts{
		RM:            evergreen.GetEnvironment().RoleManager(),
		PermissionKey: permission,
		ResourceType:  evergreen.SuperUserResourceType,
		RequiredLevel: level.Value,
		ResourceFunc:  superUserResource,
		DefaultRoles:  defaultRoles,
	}
	return gimlet.RequiresPermission(opts)
}

func urlVarsToProjectScopes(r *http.Request) ([]string, int, error) {
	var err error
	vars := gimlet.GetVars(r)
	query := r.URL.Query()
	resourceType := strings.ToUpper(util.CoalesceStrings(query["resource_type"], vars["resource_type"]))
	if resourceType != "" {
		switch resourceType {
		case model.EventResourceTypeProject:
			vars["project_id"] = vars["resource_id"]
		case event.ResourceTypeTask:
			vars["task_id"] = vars["resource_id"]
		}
	}

	projectID := util.CoalesceStrings(append(query["project_id"], query["projectId"]...), vars["project_id"], vars["projectId"])
	destProjectID := util.CoalesceString(query["dest_project"]...)

	versionID := util.CoalesceStrings(append(query["version_id"], query["versionId"]...), vars["version_id"], vars["versionId"])
	if projectID == "" && versionID != "" {
		projectID, err = model.FindProjectForVersion(versionID)
		if err != nil {
			return nil, http.StatusNotFound, err
		}
	}

	patchID := util.CoalesceStrings(append(query["patch_id"], query["patchId"]...), vars["patch_id"], vars["patchId"])
	if projectID == "" && patchID != "" && patch.IsValidId(patchID) {
		projectID, err = patch.FindProjectForPatch(patch.NewId(patchID))
		if err != nil {
			return nil, http.StatusNotFound, err
		}
	}

	buildID := util.CoalesceStrings(append(query["build_id"], query["buildId"]...), vars["build_id"], vars["buildId"])
	if projectID == "" && buildID != "" {
		projectID, err = build.FindProjectForBuild(buildID)
		if err != nil {
			return nil, http.StatusNotFound, err
		}
	}

	testLog := util.CoalesceStrings(query["log_id"], vars["log_id"])
	if projectID == "" && testLog != "" {
		var test *model.TestLog
		test, err = model.FindOneTestLogById(testLog)
		if err != nil {
			return nil, http.StatusNotFound, err
		}
		projectID, err = task.FindProjectForTask(test.Task)
		if err != nil {
			return nil, http.StatusNotFound, err
		}
	}

	// retrieve all possible naming conventions for task ID
	taskID := util.CoalesceStrings(append(query["task_id"], query["taskId"]...), vars["task_id"], vars["taskId"])
	if projectID == "" && taskID != "" {
		projectID, err = task.FindProjectForTask(taskID)
		if err != nil {
			return nil, http.StatusNotFound, err
		}
	}

	projectRef, err := model.FindOneProjectRef(projectID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.WithStack(err)
	}
	if projectRef == nil {
		return nil, http.StatusNotFound, errors.Errorf("error finding the project %s", projectID)
	}

	// check to see if this is an anonymous user requesting a private project
	user := gimlet.GetUser(r.Context())
	if user == nil {
		if err != nil || projectRef == nil {
			return nil, http.StatusNotFound, errors.New("no project found")
		}
		if projectRef.Private {
			projectID = ""
		}
	}

	// no project found - return a 404
	if projectID == "" {
		return nil, http.StatusNotFound, errors.New("no project found")
	}
	res := []string{projectID}
	if destProjectID != "" {
		res = append(res, destProjectID)
	}

	return res, http.StatusOK, nil
}

// urlVarsToDistroScopes returns all distros being requested for access and the
// HTTP status code.
func urlVarsToDistroScopes(r *http.Request) ([]string, int, error) {
	var err error
	vars := gimlet.GetVars(r)
	query := r.URL.Query()

	resourceType := strings.ToUpper(util.CoalesceStrings(query["resource_type"], vars["resource_type"]))
	if resourceType != "" {
		switch resourceType {
		case event.ResourceTypeScheduler:
		case event.ResourceTypeDistro:
			vars["distro_id"] = vars["resource_id"]
		case event.ResourceTypeHost:
			vars["host_id"] = vars["resource_id"]
		}
	}

	distroID := util.CoalesceStrings(append(query["distro_id"], query["distroId"]...), vars["distro_id"], vars["distroId"])

	hostID := util.CoalesceStrings(append(query["host_id"], query["hostId"]...), vars["host_id"], vars["hostId"])
	if distroID == "" && hostID != "" {
		distroID, err = host.FindDistroForHost(hostID)
		if err != nil {
			return nil, http.StatusNotFound, err
		}
	}

	// no distro found - return a 404
	if distroID == "" {
		return nil, http.StatusNotFound, errors.New("no distro found")
	}

	dat, err := distro.NewDistroAliasesLookupTable()
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "could not get distro lookup table")
	}
	distroIDs := dat.Expand([]string{distroID})
	if len(distroIDs) == 0 {
		return nil, http.StatusNotFound, errors.Errorf("could not resolve distro '%s'", distroID)
	}
	// Verify that all the concrete distros that this request is accessing
	// exist.
	for _, resolvedDistroID := range distroIDs {
		d, err := distro.FindByID(resolvedDistroID)
		if err != nil {
			return nil, http.StatusInternalServerError, errors.WithStack(err)
		}
		if d == nil {
			return nil, http.StatusNotFound, errors.Errorf("distro '%s' does not exist", resolvedDistroID)
		}
	}

	return distroIDs, http.StatusOK, nil
}

func superUserResource(_ *http.Request) ([]string, int, error) {
	return []string{evergreen.SuperUserPermissionsID}, http.StatusOK, nil
}

type EventLogPermissionsMiddleware struct{}

func (m *EventLogPermissionsMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	vars := gimlet.GetVars(r)
	var resources []string
	var status int
	var err error
	resourceType := strings.ToUpper(vars["resource_type"])
	opts := gimlet.PermissionOpts{}
	switch resourceType {
	case event.ResourceTypeTask:
		resources, status, err = urlVarsToProjectScopes(r)
		opts.ResourceType = evergreen.ProjectResourceType
		opts.Permission = evergreen.PermissionTasks
		opts.RequiredLevel = evergreen.TasksView.Value
	case model.EventResourceTypeProject:
		resources, status, err = urlVarsToProjectScopes(r)
		opts.ResourceType = evergreen.ProjectResourceType
		opts.Permission = evergreen.PermissionProjectSettings
		opts.RequiredLevel = evergreen.ProjectSettingsView.Value
	case event.ResourceTypeDistro:
		fallthrough
	case event.ResourceTypeScheduler:
		resources, status, err = urlVarsToDistroScopes(r)
		opts.ResourceType = evergreen.DistroResourceType
		opts.Permission = evergreen.PermissionHosts
		opts.RequiredLevel = evergreen.HostsView.Value
	case event.ResourceTypeHost:
		resources, status, err = urlVarsToDistroScopes(r)
		opts.ResourceType = evergreen.DistroResourceType
		opts.Permission = evergreen.PermissionDistroSettings
		opts.RequiredLevel = evergreen.DistroSettingsView.Value
	case event.ResourceTypeAdmin:
		resources = []string{evergreen.SuperUserPermissionsID}
		opts.ResourceType = evergreen.SuperUserResourceType
		opts.Permission = evergreen.PermissionAdminSettings
		opts.RequiredLevel = evergreen.AdminSettingsEdit.Value
	default:
		http.Error(rw, fmt.Sprintf("%s is not a valid resource type", resourceType), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(rw, err.Error(), status)
		return
	}

	if len(resources) == 0 {
		http.Error(rw, "no resources found", http.StatusNotFound)
		return
	}

	user := gimlet.GetUser(ctx)
	if user == nil {
		http.Error(rw, "no user found", http.StatusUnauthorized)
		return
	}

	authenticator := gimlet.GetAuthenticator(ctx)
	if authenticator == nil {
		http.Error(rw, "unable to determine an authenticator", http.StatusInternalServerError)
		return
	}

	if !authenticator.CheckAuthenticated(user) {
		http.Error(rw, "not authenticated", http.StatusUnauthorized)
		return
	}

	for _, item := range resources {
		opts.Resource = item
		if !user.HasPermission(opts) {
			http.Error(rw, "not authorized for this action", http.StatusUnauthorized)
			return
		}
	}

	next(rw, r)
}
