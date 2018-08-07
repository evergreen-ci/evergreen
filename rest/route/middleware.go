package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
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
