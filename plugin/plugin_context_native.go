// +build go1.7

package plugin

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
)

// SetTask puts the task for an API request into the context of a request.
// This task can be retrieved in a handler function by using "GetTask()"
func SetTask(r *http.Request, task *task.Task) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), pluginTaskContextKey, task))
}

// GetTask returns the task object for a plugin API request at runtime,
// it is a valuable helper function for API PluginRoute handlers.
func GetTask(r *http.Request) *task.Task {
	if rv := r.Context().Value(pluginTaskContextKey); rv != nil {
		if t, ok := rv.(*task.Task); ok {
			return t
		}
	}

	return nil
}

// SetUser sets the user for the request context. This is a helper for UI middleware.
func SetUser(u *user.DBUser, r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), pluginUserKey, u))
}

// GetUser fetches the user, if it exists, from the request context.
func GetUser(r *http.Request) *user.DBUser {
	if rv := r.Context().Value(pluginUserKey); rv != nil {
		if u, ok := rv.(*user.DBUser); ok {
			return u
		}
	}

	return nil
}
