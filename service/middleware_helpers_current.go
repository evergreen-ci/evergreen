package service

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/pkg/errors"
)

func setAPIHostContext(r *http.Request, h *host.Host) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), model.ApiHostKey, h))
}
func setAPITaskContext(r *http.Request, t *task.Task) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), model.ApiTaskKey, t))
}
func setProjectReftContext(r *http.Request, p *model.ProjectRef) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), model.ApiProjectRefKey, p))
}
func setProjectContext(r *http.Request, p *model.Project) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), model.ApiProjectKey, p))
}
func setUIRequestContext(r *http.Request, p projectContext) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), RequestProjectContext, p))
}
func setRestContext(r *http.Request, c *model.Context) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), RestContext, c))
}
func setRequestUser(r *http.Request, u auth.User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), evergreen.RequestUser, u))
}

// GetTask loads the task attached to a request.
func GetTask(r *http.Request) *task.Task {
	if rv := r.Context().Value(model.ApiTaskKey); rv != nil {
		if t, ok := rv.(*task.Task); ok {
			return t
		}
	}
	return nil
}

// GetHost loads the host attached to a request
func GetHost(r *http.Request) *host.Host {
	if rv := r.Context().Value(model.ApiHostKey); rv != nil {
		return rv.(*host.Host)
	}
	return nil
}

// GetProject loads the project attached to a request into request
// context.
func GetProject(r *http.Request) (*model.ProjectRef, *model.Project) {
	pref := r.Context().Value(model.ApiProjectRefKey)
	if pref == nil {
		return nil, nil
	}

	p := r.Context().Value(model.ApiProjectKey)
	if p == nil {
		return nil, nil
	}

	return pref.(*model.ProjectRef), p.(*model.Project)
}

// GetUser returns a user if one is attached to the request. Returns nil if the user is not logged
// in, assuming that the middleware to lookup user information is enabled on the request handler.
func GetUser(r *http.Request) *user.DBUser {
	if rv := r.Context().Value(evergreen.RequestUser); rv != nil {
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
