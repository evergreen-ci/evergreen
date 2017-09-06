// +build !go1.7

package service

import (
	"errors"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/gorilla/context"
)

func setAPIHostContext(r *http.Request, h *host.Host) *http.Request {
	context.Set(r, apiHostKey, h)
	return r
}
func setAPITaskContext(r *http.Request, t *task.Task) *http.Request {
	context.Set(r, apiTaskKey, t)
	return r
}
func setProjectReftContext(r *http.Request, p *model.ProjectRef) *http.Request {
	context.Set(r, apiProjectRefKey, p)
	return r
}
func setProjectContext(r *http.Request, p *model.Project) *http.Request {
	context.Set(r, apiProjectKey, p)
	return r
}
func setUIRequestContext(r *http.Request, p projectContext) *http.Request {
	context.Set(r, RequestProjectContext, p)
	return r
}
func setRestContext(r *http.Request, c *model.Context) *http.Request {
	context.Set(r, RestContext, c)
	return r
}
func setRequestUser(r *http.Request, u auth.User) *http.Request {
	context.Set(r, evergreen.RequestUser, u)
	return r
}

// GetTask loads the task attached to a request.
func GetTask(r *http.Request) *task.Task {
	if rv := context.Get(r, apiTaskKey); rv != nil {
		return rv.(*task.Task)
	}
	return nil
}

// GetHost loads the host attached to a request
func GetHost(r *http.Request) *host.Host {
	if rv := context.Get(r, apiHostKey); rv != nil {
		return rv.(*host.Host)
	}
	return nil
}

// GetProject loads the project attached to a request into request
// context.
func GetProject(r *http.Request) (*model.ProjectRef, *model.Project) {
	pref := context.Get(r, apiProjectRefKey)
	if pref == nil {
		return nil, nil
	}

	p := context.Get(r, apiProjectKey)
	if p == nil {
		return nil, nil
	}

	return pref.(*model.ProjectRef), p.(*model.Project)
}

// GetUser returns a user if one is attached to the request. Returns nil if the user is not logged
// in, assuming that the middleware to lookup user information is enabled on the request handler.
func GetUser(r *http.Request) *user.DBUser {
	if rv := context.Get(r, evergreen.RequestUser); rv != nil {
		return rv.(*user.DBUser)
	}
	return nil
}

// GetProjectContext fetches the projectContext associated with the request. Returns an error
// if no projectContext has been loaded and attached to the request.
func GetProjectContext(r *http.Request) (projectContext, error) {
	if rv := context.Get(r, RequestProjectContext); rv != nil {
		return rv.(projectContext), nil
	}
	return projectContext{}, errors.New("No context loaded")
}

// GetRESTContext fetches the context associated with the request.
func GetRESTContext(r *http.Request) (*model.Context, error) {
	if rv := context.Get(r, RestContext); rv != nil {
		return rv.(*model.Context), nil
	}
	return nil, errors.New("No context loaded")
}
