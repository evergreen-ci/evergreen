// +build go1.7

package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func setAPIHostContext(r *http.Request, h *host.Host) {
	r.WithContext(context.WithValue(r.Context(), apiHostKey, h))
}
func setAPITaskContext(r *http.Request, t *task.Task) {
	r.WithContext(context.WithValue(r.Context(), apiTaskKey, t))
}
func setProjectReftContext(r *http.Request, p *model.ProjectRef) {
	r.WithContext(context.WithValue(r.Context(), apiProjectRefKey, p))
}
func setProjectContext(r *http.Request, p *model.Project) {
	r.WithContext(context.WithValue(r.Context(), apiProjectKey, p))
}
func setUIRequestContext(r *http.Request, p projectContext) {
	r.WithContext(context.WithValue(r.Context(), RequestProjectContext, p))
}
func setRestContext(r *http.Request, c *model.Context) {
	r.WithContext(context.WithValue(r.Context(), RestContext, c))
}
func setRequestUser(r *http.Request, u auth.User) {
	r.WithContext(context.WithValue(r.Context(), RequestUser, u))
}

// GetTask loads the task attached to a request.
func GetTask(r *http.Request) *task.Task {
	if rv := r.Context().Value(apiTaskKey); rv != nil {
		if t, ok := rv.(*task.Task); ok {
			return t
		}
	}
	return nil
}

// GetHost loads the host attached to a request
func GetHost(r *http.Request) *host.Host {
	if rv := r.Context().Value(apiHostKey); rv != nil {
		return rv.(*host.Host)
	}
	return nil
}

// GetProject loads the project attached to a request into request
// context.
func GetProject(r *http.Request) (*model.ProjectRef, *model.Project) {
	pref := r.Context().Value(apiProjectRefKey)
	if pref == nil {
		return nil, nil
	}

	p := r.Context().Value(apiProjectKey)
	if p == nil {
		return nil, nil
	}

	return pref.(*model.ProjectRef), p.(*model.Project)
}

// GetUser returns a user if one is attached to the request. Returns nil if the user is not logged
// in, assuming that the middleware to lookup user information is enabled on the request handler.
func GetUser(r *http.Request) *user.DBUser {
	if rv := r.Context().Value(RequestUser); rv != nil {
		return rv.(*user.DBUser)
	}
	return nil
}

// GetProjectContext fetches the projectContext associated with the request. Returns an error
// if no projectContext has been loaded and attached to the request.
func GetProjectContext(r *http.Request) (projectContext, error) {
	if rv := r.Context().Value(RequestProjectContext); rv != nil {
		return rv.(projectContext), nil
	}
	return projectContext{}, errors.New("No context loaded")
}

// GetRESTContext fetches the context associated with the request.
func GetRESTContext(r *http.Request) (*model.Context, error) {
	if rv := r.Context().Value(RestContext); rv != nil {
		return rv.(*model.Context), nil
	}
	return nil, errors.New("No context loaded")
}
