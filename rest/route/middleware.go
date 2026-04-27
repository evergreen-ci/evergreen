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
	"github.com/google/go-github/v70/github"
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

const (
	alertmanagerUser = "alertmanager"
	sageUser         = "sage"
	backstageUser    = "backstage"
)

type projCtxMiddleware struct{}

func (m *projCtxMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	buildId := vars["build_id"]
	versionId := vars["version_id"]
	patchId := vars["patch_id"]
	projectId := vars["project_id"]

	opCtx, err := model.LoadContext(r.Context(), taskId, buildId, versionId, patchId, projectId)
	if err != nil {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "loading resources from context")))
		return
	}

	user := gimlet.GetUser(ctx)

	if opCtx.HasProjectOrRepoRef() && user == nil {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "project not found",
		}))
		return
	}

	if opCtx.Patch != nil && user == nil {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
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

// GetTask returns the task stored in the request context by
// TaskAuthMiddleware. Returns nil if no task is in the context.
func GetTask(ctx context.Context) *task.Task {
	if rv := ctx.Value(model.ApiTaskKey); rv != nil {
		if t, ok := rv.(*task.Task); ok {
			return t
		}
	}
	return nil
}

// MustHaveTask returns the task stored in the request context by
// TaskAuthMiddleware. It panics if none is set.
func MustHaveTask(ctx context.Context) *task.Task {
	t := GetTask(ctx)
	if t == nil {
		panic("task not attached to request")
	}
	return t
}

// GetHost returns the host stored in the request context by
// hostAuthMiddleware or TaskAuthMiddleware. Returns nil if no host is
// in the context.
func GetHost(ctx context.Context) *host.Host {
	if rv := ctx.Value(model.ApiHostKey); rv != nil {
		if h, ok := rv.(*host.Host); ok {
			return h
		}
	}
	return nil
}

// MustHaveHost returns the host stored in the request context by
// hostAuthMiddleware or TaskAuthMiddleware. It panics if none is set.
func MustHaveHost(ctx context.Context) *host.Host {
	h := GetHost(ctx)
	if h == nil {
		panic("host not attached to request")
	}
	return h
}

func validPriority(ctx context.Context, priority int64, project string, user gimlet.User) bool {
	if priority > evergreen.MaxTaskPriority {
		return user.HasPermission(ctx, gimlet.PermissionOpts{
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

	if opCtx == nil || !opCtx.HasProjectOrRepoRef() {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "no project found",
		}))
		return
	}

	isAdmin := user.HasPermission(ctx, gimlet.PermissionOpts{
		Resource:      opCtx.GetProjectOrRepoRefId(),
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	})
	if !isAdmin {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
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

	canCreate, err := user.HasProjectCreatePermission(ctx)
	if err != nil {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "error checking permissions",
		}))
		return
	}
	if !canCreate {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
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
			gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "missing host ID",
			}))
			return
		}
	}
	h, statusCode, err := model.ValidateHost(hostID, r)
	if err != nil {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: statusCode,
			Message:    errors.Wrapf(err, "invalid host '%s'", hostID).Error(),
		}))
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), model.ApiHostKey, h))

	updateHostAccessTime(r.Context(), h)
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
	if !ok {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
		}))
		return
	}
	if errResp := authenticateSpecialUser(r, alertmanagerUser, username, password); errResp != nil {
		gimlet.WriteResponse(r.Context(), rw, errResp)
		return
	}
	next(rw, r)
}

// authenticateSpecialUser checks if a specific user has provided the required
// authentication. Typically for authenticating special-purpose service users.
// Returns a non-nil response if authentication fails.
func authenticateSpecialUser(r *http.Request, requiredUsername, username, apiKey string) gimlet.Responder {
	if username != requiredUsername {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
		})
	}
	u, err := user.FindOneById(r.Context(), username)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding user '%s'", username))
	}
	if u == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("user '%s' not found", username),
		})
	}
	if u.APIKey != apiKey {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "not authorized",
		})
	}
	return nil
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
			gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "not authorized",
			}))
			return
		}
	}
	t, code, err := data.CheckTaskSecret(taskID, r)
	if err != nil {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: code,
			Message:    errors.Wrapf(err, "checking secret for task '%s'", taskID).Error(),
		}))
		return
	}
	if time.Since(t.FinishTime) > completedTaskValidityWindow && utility.StringSliceContains(evergreen.TaskCompletedStatuses, t.Status) {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    fmt.Sprintf("task '%s' cannot make requests in a completed state", taskID),
		}))
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), model.ApiTaskKey, t))

	hostID, ok := gimlet.GetVars(r)["host_id"]
	if !ok {
		hostID = r.Header.Get(evergreen.HostHeader)
	}
	if hostID == "" {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "host ID must be set",
		}))
		return
	}

	h, code, err := model.ValidateHost(hostID, r)
	if err != nil {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: code,
			Message:    errors.Wrapf(err, "invalid host associated with task '%s'", taskID).Error(),
		}))
		return
	}

	r = r.WithContext(context.WithValue(r.Context(), model.ApiHostKey, h))

	next(rw, r)
}

// updateHostAccessTime updates the host access time and disables the host's flags to deploy new a new agent
// or agent monitor if they are set.
func updateHostAccessTime(ctx context.Context, h *host.Host) {
	if err := h.UpdateLastCommunicated(ctx); err != nil {
		grip.Warningf(ctx, "Could not update host last communication time for %s: %+v", h.Id, err)
	}
	// Since the host has contacted the app server, we should prevent the
	// app server from attempting to deploy agents or agent monitors.
	// Deciding whether we should redeploy agents or agent monitors
	// is handled within the REST route handler.
	if h.NeedsNewAgent {
		grip.Warning(ctx, message.WrapError(h.SetNeedsNewAgent(ctx, false), "problem clearing host needs new agent"))
	}
	if h.NeedsNewAgentMonitor {
		grip.Warning(ctx, message.WrapError(h.SetNeedsNewAgentMonitor(ctx, false), "problem clearing host needs new agent monitor"))
	}
}

type sageMiddleware struct{}

// NewSageMiddleware returns a middleware that verifies the request
// is coming from Sage.
func NewSageMiddleware() gimlet.Middleware {
	return &sageMiddleware{}
}

func (m *sageMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	apiUser := r.Header.Get(evergreen.APIUserHeader)
	apiKey := r.Header.Get(evergreen.APIKeyHeader)
	if errResp := authenticateSpecialUser(r, sageUser, apiUser, apiKey); errResp != nil {
		gimlet.WriteResponse(r.Context(), rw, errResp)
		return
	}
	next(rw, r)
}

type backstageMiddleware struct{}

// newBackstageMiddleware returns a middleware that verifies the request
// is coming from Backstage.
func newBackstageMiddleware() gimlet.Middleware {
	return &backstageMiddleware{}
}

func (m *backstageMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	apiUser := r.Header.Get(evergreen.APIUserHeader)
	apiKey := r.Header.Get(evergreen.APIKeyHeader)
	if errResp := authenticateSpecialUser(r, backstageUser, apiUser, apiKey); errResp != nil {
		gimlet.WriteResponse(r.Context(), rw, errResp)
		return
	}
	next(rw, r)
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

// NewGithubAuthMiddleware returns a middleware that verifies the payload.
func NewGithubAuthMiddleware() gimlet.Middleware {
	return &githubAuthMiddleware{}
}

type githubAuthMiddleware struct{}

func (m *githubAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	githubSecret := []byte(evergreen.GetEnvironment().Settings().GithubWebhookSecret)

	payload, err := github.ValidatePayload(r, githubSecret)
	if err != nil {
		grip.Error(r.Context(), message.WrapError(err, message.Fields{
			"source":  "GitHub hook",
			"message": "rejecting GitHub webhook",
			"msg_id":  r.Header.Get("X-Github-Delivery"),
			"event":   r.Header.Get("X-Github-Event"),
		}))
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(errors.Wrap(err, "validating GitHub payload")))
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
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(errors.Wrap(err, "reading body")))
		return
	}
	var payload sns.Payload
	if err = json.Unmarshal(body, &payload); err != nil {
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(errors.Wrap(err, "unmarshalling JSON payload")))
		return
	}

	if err = payload.VerifyPayload(); err != nil {
		msg := "AWS SNS message failed validation"
		grip.Error(r.Context(), message.WrapError(err, message.Fields{
			"message": msg,
			"payload": payload,
		}))
		gimlet.WriteResponse(r.Context(), rw, gimlet.MakeJSONErrorResponder(errors.Wrap(err, msg)))
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

// NewUserOrTaskAuthMiddleware returns a middleware that verifies the request
// is authenticated as either a user or a task/host. When the request is
// user-authenticated, the middleware additionally checks that the user has
// patch submit permission on the project associated with the task.
func NewUserOrTaskAuthMiddleware() gimlet.Middleware {
	return &userOrTaskAuthMiddleware{
		taskFallback: NewTaskAuthMiddleware(),
	}
}

type userOrTaskAuthMiddleware struct {
	taskFallback gimlet.Middleware
}

func (m *userOrTaskAuthMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	u := gimlet.GetUser(ctx)
	if u == nil {
		// Fallback to task/host auth.
		m.taskFallback.ServeHTTP(rw, r, next)
		return
	}

	vars := gimlet.GetVars(r)
	taskID := vars["task_id"]
	if taskID == "" {
		gimlet.WriteResponse(ctx, rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "task ID is required",
		}))
		return
	}

	if evergreen.GetEnvironment().Settings().ServiceFlags.DebugSpawnHostDisabled {
		gimlet.WriteResponse(ctx, rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    "debug spawn hosts are currently disabled",
		}))
		return
	}

	projectID, err := task.FindProjectForTask(ctx, taskID)
	if err != nil {
		gimlet.WriteResponse(ctx, rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("could not find project for task '%s': %s", taskID, err.Error()),
		}))
		return
	}

	pRef, err := model.FindMergedProjectRef(ctx, projectID, "", false)
	if err != nil {
		gimlet.WriteResponse(ctx, rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("finding project '%s': %s", projectID, err.Error()),
		}))
		return
	}
	if pRef == nil {
		gimlet.WriteResponse(ctx, rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project '%s' not found", projectID),
		}))
		return
	}
	if !pRef.IsDebugSpawnHostsEnabled() {
		gimlet.WriteResponse(ctx, rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    fmt.Sprintf("debug spawn hosts are disabled for project '%s'", projectID),
		}))
		return
	}

	if !u.HasPermission(ctx, gimlet.PermissionOpts{
		Resource:      projectID,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionPatches,
		RequiredLevel: evergreen.PatchSubmit.Value,
	}) {
		gimlet.WriteResponse(ctx, rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    fmt.Sprintf("user '%s' does not have patch submit permission on project '%s'", u.Username(), projectID),
		}))
		return
	}

	next(rw, r)
}
