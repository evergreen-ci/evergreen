package route

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
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

func validPriority(priority int64, user gimlet.User, sc data.Connector) bool {
	if priority > evergreen.MaxTaskPriority {
		return auth.IsSuperUser(sc.GetSuperUsers(), user)
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

	isSuperuser := util.StringSliceContains(m.sc.GetSuperUsers(), user.Username())
	isAdmin := util.StringSliceContains(opCtx.ProjectRef.Admins, user.Username())
	if !(isSuperuser || isAdmin) {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "Not authorized",
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
	if code, err := m.sc.CheckHostSecret(r); err != nil {
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
	isSuperuser := util.StringSliceContains(m.sc.GetSuperUsers(), user.Username())
	isAdmin := util.StringSliceContains(projRef.Admins, user.Username())
	if isSuperuser || isAdmin {
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
		if user.Id != patch.Author {
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
			githubUID = *pr.User.ID
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
	if !evergreen.AclCheckingIsEnabled {
		return &noopMiddleware{}
	}

	opts := gimlet.RequiresPermissionMiddlewareOpts{
		RM:            evergreen.GetEnvironment().RoleManager(),
		PermissionKey: permission,
		ResourceType:  "project",
		RequiredLevel: level.Value(),
		ResourceFunc:  urlVarsToScopes,
	}
	return gimlet.RequiresPermission(opts)
}

type noopMiddleware struct{}

func (n *noopMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	next(rw, r)
}

func urlVarsToScopes(r *http.Request) string {
	vars := gimlet.GetVars(r)
	query := r.URL.Query()

	projectID := util.CoalesceStrings(append(query["project_id"], query["projectId"]...), vars["project_id"], vars["projectId"])
	if projectID != "" {
		return projectID
	}

	versionID := util.CoalesceStrings(append(query["version_id"], query["versionId"]...), vars["version_id"], vars["versionId"])
	if versionID != "" {
		proj, err := model.FindProjectForVersion(versionID)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "error finding version",
				"version": versionID,
			}))
			return ""
		}
		return proj
	}

	patchID := util.CoalesceStrings(append(query["patch_id"], query["patchId"]...), vars["patch_id"], vars["patchId"])
	if patchID != "" && patch.IsValidId(patchID) {
		proj, err := patch.FindProjectForPatch(patch.NewId(patchID))
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "error finding patch",
				"patch":   patchID,
			}))
			return ""
		}
		return proj
	}

	buildID := util.CoalesceStrings(append(query["build_id"], query["buildId"]...), vars["build_id"], vars["buildId"])
	if buildID != "" {
		proj, err := build.FindProjectForBuild(buildID)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "error finding build",
				"build":   buildID,
			}))
			return ""
		}
		return proj
	}

	// retrieve all possible naming conventions for task ID
	taskID := util.CoalesceStrings(append(query["task_id"], query["taskId"]...), vars["task_id"], vars["taskId"])
	if taskID != "" {
		proj, err := task.FindProjectForTask(taskID)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "error finding task",
				"build":   taskID,
			}))
			return ""
		}
		return proj
	}

	return ""
}
