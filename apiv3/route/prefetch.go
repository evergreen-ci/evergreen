package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type (
	// custom types used to attach specific values to request contexts, to prevent collisions.
	requestUserKey    int
	requestContextKey int
)

const (
	// Key values used to map user and project data to request context.
	// These are private custom types to avoid key collisions.
	RequestUser    requestUserKey    = 0
	RequestContext requestContextKey = 0
)

// PrefetchFunc is a function signature that defines types of functions which may
// be used to fetch data before the main request handler is called. They should
// fetch data using the ServiceeContext and set them on the request context.
type PrefetchFunc func(*http.Request, servicecontext.ServiceContext) error

// PrefetchUser gets the user information from a request, and uses it to
// get the associated user from the database and attaches it to the request context.
func PrefetchUser(r *http.Request, sc servicecontext.ServiceContext) error {
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
				return apiv3.APIError{
					StatusCode: http.StatusUnauthorized,
					Message:    "Invalid API key",
				}
			}
			context.Set(r, RequestUser, apiUser)
		}
	}
	return nil
}

// PrefetchProjectContext gets the information related to the project that the request contains
// and fetches the associated project context and attaches that to the request context.
func PrefetchProjectContext(r *http.Request, sc servicecontext.ServiceContext) error {
	vars := mux.Vars(r)
	taskId := vars["task_id"]
	buildId := vars["build_id"]
	versionId := vars["version_id"]
	patchId := vars["patch_id"]
	projectId := vars["project_id"]
	ctx, err := sc.FetchContext(taskId, buildId, versionId, patchId, projectId)
	if err != nil {
		return err
	}

	if ctx.ProjectRef != nil && ctx.ProjectRef.Private && GetUser(r) == nil {
		// Project is private and user is not authorized so return not found
		return apiv3.APIError{
			StatusCode: http.StatusNotFound,
			Message:    "Project not found",
		}
	}

	if ctx.Patch != nil && GetUser(r) == nil {
		return apiv3.APIError{
			StatusCode: http.StatusNotFound,
			Message:    "Not found",
		}
	}
	context.Set(r, RequestContext, &ctx)
	return nil
}

// GetUser returns the user associated with a given http request.
func GetUser(r *http.Request) *user.DBUser {
	if rv := context.Get(r, RequestUser); rv != nil {
		return rv.(*user.DBUser)
	}
	return nil
}

// GetProjectContext returns the project context associated with a
// given request.
func GetProjectContext(r *http.Request) (*model.Context, error) {
	if rv := context.Get(r, RequestContext); rv != nil {
		return rv.(*model.Context), nil
	}
	return &model.Context{}, errors.New("No context loaded")
}

// MustHaveProjectContext returns the project context set on the
// http request context. It panics if none is set.
func MustHaveProjectContext(r *http.Request) *model.Context {
	pc, err := GetProjectContext(r)
	if err != nil {
		panic(err)
	}
	return pc
}

// MustHaveUser returns the user associated with a given request or panics
// if none is present.
func MustHaveUser(r *http.Request) *user.DBUser {
	u := GetUser(r)
	if u == nil {
		panic("no user attached to request")
	}
	return u
}
