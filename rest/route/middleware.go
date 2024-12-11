package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	sns "github.com/robbiet480/go.sns"
)

type (
	// custom type used to attach specific values to request contexts, to prevent collisions.
	requestContextKey int
)

const (
	// These are private custom types to avoid key collisions.
	RequestContext   requestContextKey = 0
	githubPayloadKey requestContextKey = 3
	snsPayloadKey    requestContextKey = 5
)

const alertmanagerUser = "alertmanager"

type projCtxMiddleware struct{}

func (m *projCtxMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	buildId := vars["build_id"]
	versionId := vars["version_id"]
	patchId := vars["patch_id"]
	projectId := vars["project_id"]

	opCtx, err := model.LoadContext(taskId, buildId, versionId, patchId, projectId)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "loading resources from context")))
		return
	}

	user := gimlet.GetUser(ctx)

	if opCtx.ProjectRef != nil && user == nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "project not found",
		}))
		return
	}

	if opCtx.Patch != nil && user == nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "user associated with patch not found",
		}))
		return
	}

	r = r.WithContext(context.WithValue(ctx, RequestContext, &opCtx))

	next(rw, r)
}

func NewProjectContextMiddleware() gimlet.Middleware {
	return &projCtxMiddleware{}
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

func validPriority(priority int64, project string, user gimlet.User) bool {
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

func NewProjectAdminMiddleware() gimlet.Middleware {
	return &projectAdminMiddleware{}
}

type projectAdminMiddleware struct {
}

func (m *projectAdminMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	opCtx := MustHaveProjectContext(ctx)
	user := MustHaveUser(ctx)

	if opCtx == nil || opCtx.ProjectRef == nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "no project found",
		}))
		return
	}

	isAdmin := user.HasPermission(gimlet.PermissionOpts{
		Resource:      opCtx.ProjectRef.Id,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	})
	if !isAdmin {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
		}))
		return
	}

	next(rw, r)
}

func NewCanCreateMiddleware() gimlet.Middleware {
	return &canCreateMiddleware{}
}

type canCreateMiddleware struct {
}

func (m *canCreateMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	user := MustHaveUser(ctx)

	canCreate, err := user.HasProjectCreatePermission()
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "error checking permissions",
		}))
		return
	}
	if !canCreate {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
		}))
		return
	}

	next(rw, r)
}

// This middleware is more restrictive than checkProjectAdmin, as branch admins do not have access
func NewRepoAdminMiddleware() gimlet.Middleware {
	return &projectRepoMiddleware{}
}

type projectRepoMiddleware struct {
}

func (m *projectRepoMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	u := MustHaveUser(ctx)
	vars := gimlet.GetVars(r)
	repoId, ok := vars["repo_id"]
	if !ok || repoId == "" {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
		}))
		return
	}

	repoRef, err := model.FindOneRepoRef(repoId)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}
	if repoRef == nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("repo '%s' not found", repoId),
		}))
		return
	}
	isRepoAdmin := u.HasPermission(gimlet.PermissionOpts{
		Resource:      repoRef.Id,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	})
	if !isRepoAdmin {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
		}))
		return
	}

	next(rw, r)
}

// NewTaskHostAuthMiddleware returns route middleware that authenticates a host
// created by a task and verifies the secret of the host that created this host.
func NewTaskHostAuthMiddleware() gimlet.Middleware {
	return &TaskHostAuthMiddleware{}
}

type TaskHostAuthMiddleware struct {
}

func (m *TaskHostAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	vars := gimlet.GetVars(r)
	hostID, ok := vars["host_id"]
	if !ok {
		hostID = r.Header.Get(evergreen.HostHeader)
		if hostID == "" {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "not authorized",
			}))
			return
		}
	}
	h, err := host.FindOneId(r.Context(), hostID)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}
	if h == nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host '%s' not found", hostID),
		}))
		return
	}

	if h.StartedBy == "" {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Errorf("host '%s' is not started by any task", h.Id)))
		return
	}
	t, err := task.FindOneId(h.StartedBy)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s' started by host '%s'", h.StartedBy, h.Id)))
		return
	}
	if t == nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.StartedBy),
		}))
	}
	if _, code, err := model.ValidateHost(t.HostId, r); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: code,
			Message:    errors.Wrapf(err, "invalid host '%s' associated with task '%s'", t.HostId, t.Id).Error(),
		}))
		return
	}

	next(rw, r)
}

type hostAuthMiddleware struct{}

// NewHostAuthMiddleware returns a route middleware that verifies the request's
// host ID and secret.
func NewHostAuthMiddleware() gimlet.Middleware {
	return &hostAuthMiddleware{}
}

func (m *hostAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	hostID, ok := gimlet.GetVars(r)["host_id"]
	if !ok {
		hostID = r.Header.Get(evergreen.HostHeader)
		if hostID == "" {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "missing host ID",
			}))
			return
		}
	}
	h, statusCode, err := model.ValidateHost(hostID, r)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: statusCode,
			Message:    errors.Wrapf(err, "invalid host '%s'", hostID).Error(),
		}))
		return
	}
	updateHostAccessTime(r.Context(), h)
	next(rw, r)
}

type podOrHostAuthMiddleware struct{}

// NewPodOrHostAuthMiddleWare returns a middleware that verifies that the request comes from a valid pod or host based
// on its ID and shared secret.
func NewPodOrHostAuthMiddleWare() gimlet.Middleware {
	return &podOrHostAuthMiddleware{}
}

func (m *podOrHostAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	podID, ok := gimlet.GetVars(r)["pod_id"]
	if !ok {
		podID = r.Header.Get(evergreen.PodHeader)
	}
	hostID, ok := gimlet.GetVars(r)["host_id"]
	if !ok {
		hostID = r.Header.Get(evergreen.HostHeader)
	}
	if hostID == "" && podID == "" {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "either host ID or pod ID must be set",
		}))
		return
	}
	if hostID != "" && podID != "" {
		gimlet.WriteResponse(rw, gimlet.NewJSONErrorResponse("host ID and pod ID cannot both be set"))
		return
	}

	isHostMode := hostID != ""
	if isHostMode {
		h, statusCode, err := model.ValidateHost(hostID, r)
		if err != nil {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: statusCode,
				Message:    err.Error(),
			}))
			return
		}
		updateHostAccessTime(r.Context(), h)
		next(rw, r)
		return
	}
	if err := checkPodSecret(r, podID); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}
	next(rw, r)
}

func checkPodSecret(r *http.Request, podID string) error {
	secret := r.Header.Get(evergreen.PodSecretHeader)
	if secret == "" {
		return errors.New("missing pod secret")
	}
	if err := data.CheckPodSecret(podID, secret); err != nil {
		return errors.Wrap(err, "checking pod secret")
	}
	return nil
}

type podAuthMiddleware struct{}

// NewPodAuthMiddleware returns a middleware that verifies the request's pod ID
// and secret.
func NewPodAuthMiddleware() gimlet.Middleware {
	return &podAuthMiddleware{}
}

func (m *podAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	id := gimlet.GetVars(r)["pod_id"]
	if id == "" {
		if id = r.Header.Get(evergreen.PodHeader); id == "" {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.New("missing pod ID")))
			return
		}
	}

	if err := checkPodSecret(r, id); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	next(rw, r)
}

type alertmanagerMiddleware struct{}

// NewAlertmanagerMiddleware returns a middleware that verifies the request
// is coming from Evergreen's configured Alertmanager Kanopy webhook.
func NewAlertmanagerMiddleware() gimlet.Middleware {
	return &alertmanagerMiddleware{}
}

func (m *alertmanagerMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	// Our Alertmanager webhook sends its credentials via basic auth, so we treat the username/password
	// pair incoming from the request as we would Api-User / Api-Key header pairs to fetch a user document.
	username, password, ok := r.BasicAuth()
	if !ok || username != alertmanagerUser {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
		}))
		return
	}
	u, err := user.FindOneById(username)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding user '%s'", username)))
		return
	}
	if u == nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("user '%s' not found", username),
		}))
		return
	}
	if u.APIKey != password {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
		}))
		return
	}
	next(rw, r)
}

func NewTaskAuthMiddleware() gimlet.Middleware {
	return &TaskAuthMiddleware{}
}

type TaskAuthMiddleware struct{}

const completedTaskValidityWindow = time.Hour

func (m *TaskAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	vars := gimlet.GetVars(r)
	taskID, ok := vars["task_id"]
	if !ok {
		taskID = r.Header.Get(evergreen.TaskHeader)
		if taskID == "" {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "not authorized",
			}))
			return
		}
	}
	t, code, err := data.CheckTaskSecret(taskID, r)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: code,
			Message:    errors.Wrapf(err, "checking secret for task '%s'", taskID).Error(),
		}))
		return
	}
	if time.Since(t.FinishTime) > completedTaskValidityWindow && utility.StringSliceContains(evergreen.TaskCompletedStatuses, t.Status) {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    fmt.Sprintf("task '%s' cannot make requests in a completed state", taskID),
		}))
		return
	}
	podID, ok := gimlet.GetVars(r)["pod_id"]
	if !ok {
		podID = r.Header.Get(evergreen.PodHeader)
	}
	hostID, ok := gimlet.GetVars(r)["host_id"]
	if !ok {
		hostID = r.Header.Get(evergreen.HostHeader)
	}
	if hostID == "" && podID == "" {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "either host ID or pod ID must be set",
		}))
		return
	}
	if hostID != "" && podID != "" {
		gimlet.WriteResponse(rw, gimlet.NewJSONErrorResponse("host ID and pod ID cannot both be set"))
		return
	}
	isHostMode := hostID != ""
	if isHostMode {
		if _, code, err := model.ValidateHost(hostID, r); err != nil {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: code,
				Message:    errors.Wrapf(err, "invalid host associated with task '%s'", taskID).Error(),
			}))
			return
		}
	} else {
		if err := checkPodSecret(r, podID); err != nil {
			gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
			return
		}
	}

	next(rw, r)
}

func NewCommitQueueItemOwnerMiddleware() gimlet.Middleware {
	return &CommitQueueItemOwnerMiddleware{
		sc: &data.DBConnector{},
	}
}

func NewMockCommitQueueItemOwnerMiddleware() gimlet.Middleware {
	return &CommitQueueItemOwnerMiddleware{
		sc: &data.MockGitHubConnector{},
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
		gimlet.WriteResponse(rw, gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting project ref")))
		return
	}

	vars := gimlet.GetVars(r)
	itemId, ok := vars["item"]
	if !ok {
		itemId, ok = vars["patch_id"]
	}
	if !ok || itemId == "" {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.New("no commit queue items provided")))
		return
	}

	if err = data.CheckCanRemoveCommitQueueItem(ctx, m.sc, user, projRef, itemId); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	next(rw, r)
}

// updateHostAccessTime updates the host access time and disables the host's flags to deploy new a new agent
// or agent monitor if they are set.
func updateHostAccessTime(ctx context.Context, h *host.Host) {
	if err := h.UpdateLastCommunicated(ctx); err != nil {
		grip.Warningf("Could not update host last communication time for %s: %+v", h.Id, err)
	}
	// Since the host has contacted the app server, we should prevent the
	// app server from attempting to deploy agents or agent monitors.
	// Deciding whether we should redeploy agents or agent monitors
	// is handled within the REST route handler.
	if h.NeedsNewAgent {
		grip.Warning(message.WrapError(h.SetNeedsNewAgent(ctx, false), "problem clearing host needs new agent"))
	}
	if h.NeedsNewAgentMonitor {
		grip.Warning(message.WrapError(h.SetNeedsNewAgentMonitor(ctx, false), "problem clearing host needs new agent monitor"))
	}
}

func RequiresProjectPermission(permission string, level evergreen.PermissionLevel) gimlet.Middleware {
	opts := gimlet.RequiresPermissionMiddlewareOpts{
		RM:            evergreen.GetEnvironment().RoleManager(),
		PermissionKey: permission,
		ResourceType:  evergreen.ProjectResourceType,
		RequiredLevel: level.Value,
		ResourceFunc:  urlVarsToProjectScopes,
	}

	return gimlet.RequiresPermission(opts)
}

func RequiresDistroPermission(permission string, level evergreen.PermissionLevel) gimlet.Middleware {
	opts := gimlet.RequiresPermissionMiddlewareOpts{
		RM:            evergreen.GetEnvironment().RoleManager(),
		PermissionKey: permission,
		ResourceType:  evergreen.DistroResourceType,
		RequiredLevel: level.Value,
		ResourceFunc:  urlVarsToDistroScopes,
	}
	return gimlet.RequiresPermission(opts)
}

func RequiresSuperUserPermission(permission string, level evergreen.PermissionLevel) gimlet.Middleware {
	opts := gimlet.RequiresPermissionMiddlewareOpts{
		RM:            evergreen.GetEnvironment().RoleManager(),
		PermissionKey: permission,
		ResourceType:  evergreen.SuperUserResourceType,
		RequiredLevel: level.Value,
		ResourceFunc:  superUserResource,
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
		case event.EventResourceTypeProject:
			vars["project_id"] = vars["resource_id"]
		case event.ResourceTypeTask:
			vars["task_id"] = vars["resource_id"]
		}
	}
	destProjectID := util.CoalesceString(query["dest_project"]...)

	paramsMap := data.BuildProjectParameterMapForLegacy(query, vars)
	projectId, statusCode, err := data.GetProjectIdFromParams(r.Context(), paramsMap)
	if err != nil {
		return nil, statusCode, err
	}

	res := []string{projectId}
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
		case event.ResourceTypeDistro:
			vars["distro_id"] = vars["resource_id"]
		case event.ResourceTypeHost:
			vars["host_id"] = vars["resource_id"]
		}
	}

	distroID := util.CoalesceStrings(append(query["distro_id"], query["distroId"]...), vars["distro_id"], vars["distroId"])

	hostID := util.CoalesceStrings(append(query["host_id"], query["hostId"]...), vars["host_id"], vars["hostId"])
	if distroID == "" && hostID != "" {
		distroID, err = host.FindDistroForHost(r.Context(), hostID)
		if err != nil {
			return nil, http.StatusNotFound, errors.Wrapf(err, "finding distro for host '%s'", hostID)
		}
	}

	// no distro found - return a 404
	if distroID == "" {
		return nil, http.StatusNotFound, errors.New("no distro found")
	}

	dat, err := distro.NewDistroAliasesLookupTable(r.Context())
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "getting distro lookup table")
	}
	distroIDs := dat.Expand([]string{distroID})
	if len(distroIDs) == 0 {
		return nil, http.StatusNotFound, errors.Errorf("distro '%s' did not match any existing distros", distroID)
	}
	// Verify that all the concrete distros that this request is accessing
	// exist.
	for _, resolvedDistroID := range distroIDs {
		d, err := distro.FindOneId(r.Context(), resolvedDistroID)
		if err != nil {
			return nil, http.StatusInternalServerError, errors.Wrapf(err, "finding distro '%s'", resolvedDistroID)
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
	case event.EventResourceTypeProject:
		resources, status, err = urlVarsToProjectScopes(r)
		opts.ResourceType = evergreen.ProjectResourceType
		opts.Permission = evergreen.PermissionProjectSettings
		opts.RequiredLevel = evergreen.ProjectSettingsView.Value
	case event.ResourceTypeDistro:
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
		http.Error(rw, fmt.Sprintf("resource type '%s' is not recognized", resourceType), http.StatusBadRequest)
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

// NewGithubAuthMiddleware returns a middleware that verifies the payload.
func NewGithubAuthMiddleware() gimlet.Middleware {
	return &githubAuthMiddleware{}
}

type githubAuthMiddleware struct{}

func (m *githubAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	githubSecret := []byte(evergreen.GetEnvironment().Settings().GithubWebhookSecret)

	payload, err := github.ValidatePayload(r, githubSecret)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"source":  "GitHub hook",
			"message": "rejecting GitHub webhook",
			"msg_id":  r.Header.Get("X-Github-Delivery"),
			"event":   r.Header.Get("X-Github-Event"),
		}))
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrap(err, "validating GitHub payload")))
		return
	}

	r = setGitHubPayload(r, payload)
	next(rw, r)
}

func setGitHubPayload(r *http.Request, payload []byte) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), githubPayloadKey, payload))
}

func getGitHubPayload(ctx context.Context) []byte {
	if rv := ctx.Value(githubPayloadKey); rv != nil {
		if t, ok := rv.([]byte); ok {
			return t
		}
	}

	return []byte{}
}

type snsAuthMiddleware struct{}

// NewSNSAuthMiddleware returns a middleware that verifies the payload
func NewSNSAuthMiddleware() gimlet.Middleware {
	return &snsAuthMiddleware{}
}

func (m *snsAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrap(err, "reading body")))
		return
	}
	var payload sns.Payload
	if err = json.Unmarshal(body, &payload); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrap(err, "unmarshalling JSON payload")))
		return
	}

	if err = payload.VerifyPayload(); err != nil {
		msg := "AWS SNS message failed validation"
		grip.Error(message.WrapError(err, message.Fields{
			"message": msg,
			"payload": payload,
		}))
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrap(err, msg)))
		return
	}

	r = setSNSPayload(r, payload)
	next(rw, r)
}

func setSNSPayload(r *http.Request, payload sns.Payload) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), snsPayloadKey, payload))
}

func getSNSPayload(ctx context.Context) sns.Payload {
	if rv := ctx.Value(snsPayloadKey); rv != nil {
		if t, ok := rv.(sns.Payload); ok {
			return t
		}
	}

	return sns.Payload{}
}

func AddCORSHeaders(allowedOrigins []string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requester := r.Header.Get("Origin")
		grip.DebugWhen(requester != "", message.Fields{
			"op":              "addCORSHeaders",
			"requester":       requester,
			"allowed_origins": allowedOrigins,
			"adding_headers":  utility.StringMatchesAnyRegex(requester, allowedOrigins),
			"settings_is_nil": evergreen.GetEnvironment().Settings() == nil,
			"headers":         r.Header,
		})
		if len(allowedOrigins) > 0 {
			// Requests from a GQL client include this header, which must be added to the response to enable CORS
			gqlHeader := r.Header.Get("Access-Control-Request-Headers")
			if utility.StringMatchesAnyRegex(requester, allowedOrigins) {
				w.Header().Add("Access-Control-Allow-Origin", requester)
				w.Header().Add("Access-Control-Allow-Credentials", "true")
				w.Header().Add("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PATCH, PUT")
				w.Header().Add("Access-Control-Allow-Headers", fmt.Sprintf("%s, %s, %s", evergreen.APIKeyHeader, evergreen.APIUserHeader, gqlHeader))
				w.Header().Add("Access-Control-Max-Age", "600")
			}
		}
		next(w, r)
	}
}

func allowCORS(next http.HandlerFunc) http.HandlerFunc {
	origins := []string{}
	settings := evergreen.GetEnvironment().Settings()
	if settings != nil {
		origins = settings.Ui.CORSOrigins
	}
	return AddCORSHeaders(origins, next)
}
