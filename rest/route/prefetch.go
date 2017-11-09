package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/gorilla/mux"
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

// PrefetchFunc is a function signature that defines types of functions which may
// be used to fetch data before the main request handler is called. They should
// fetch data using data.Connector set them on the request context.
type PrefetchFunc func(context.Context, data.Connector, *http.Request) (context.Context, error)

// PrefetchUser gets the user information from a request, and uses it to
// get the associated user from the database and attaches it to the request context.
func PrefetchUser(ctx context.Context, sc data.Connector, r *http.Request) (context.Context, error) {
	// Grab API auth details from header
	var authDataAPIKey, authDataName string

	if len(r.Header["Api-Key"]) > 0 {
		authDataAPIKey = r.Header["Api-Key"][0]
	}
	if len(r.Header["Auth-Username"]) > 0 {
		authDataName = r.Header["Auth-Username"][0]
	}
	if len(authDataName) == 0 && len(r.Header["Api-User"]) > 0 {
		authDataName = r.Header["Api-User"][0]
	}

	if len(authDataAPIKey) > 0 {
		apiUser, err := sc.FindUserById(authDataName)
		if apiUser.(*user.DBUser) != nil && err == nil {
			if apiUser.GetAPIKey() != authDataAPIKey {
				return ctx, rest.APIError{
					StatusCode: http.StatusUnauthorized,
					Message:    "Invalid API key",
				}
			}

			ctx = context.WithValue(ctx, evergreen.RequestUser, apiUser)
		}
	}

	return ctx, nil
}

// PrefetchProjectContext gets the information related to the project that the request contains
// and fetches the associated project context and attaches that to the request context.
func PrefetchProjectContext(ctx context.Context, sc data.Connector, r *http.Request) (context.Context, error) {
	vars := mux.Vars(r)
	taskId := vars["task_id"]
	buildId := vars["build_id"]
	versionId := vars["version_id"]
	patchId := vars["patch_id"]
	projectId := vars["project_id"]

	opCtx := sc.FetchContext(taskId, buildId, versionId, patchId, projectId)
	user := GetUser(ctx)

	pref, err := opCtx.GetProjectRef()
	if err != nil {
		return ctx, rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}

	if pref != nil && pref.Private && user == nil {
		// Project is private and user is not authorized so return not found
		return ctx, rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    "Project not found",
		}
	}

	patch, err := opCtx.GetPatch()
	if err != nil {
		return ctx, rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	if patch != nil && user == nil {
		return ctx, rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    "Not found",
		}
	}

	ctx = context.WithValue(ctx, RequestContext, opCtx)

	return ctx, nil
}

// GetUser returns the user associated with a given http request.
func GetUser(ctx context.Context) *user.DBUser {
	u, _ := ctx.Value(evergreen.RequestUser).(*user.DBUser)
	return u
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
	u := GetUser(ctx)
	if u == nil {
		panic("no user attached to request")
	}
	return u
}
