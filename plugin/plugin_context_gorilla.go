// +build !go1.7

package plugin

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/gorilla/context"
)

// SetTask puts the task for an API request into the context of a request.
// This task can be retrieved in a handler function by using "GetTask()"
func SetTask(request *http.Request, task *task.Task) {
	context.Set(request, pluginTaskContextKey, task)
}

// GetTask returns the task object for a plugin API request at runtime,
// it is a valuable helper function for API PluginRoute handlers.
func GetTask(request *http.Request) *task.Task {
	if rv := context.Get(request, pluginTaskContextKey); rv != nil {
		return rv.(*task.Task)
	}
	return nil
}

// SetUser sets the user for the request context. This is a helper for UI middleware.
func SetUser(u *user.DBUser, r *http.Request) {
	context.Set(r, pluginUserKey, u)
}

// GetUser fetches the user, if it exists, from the request context.
func GetUser(r *http.Request) *user.DBUser {
	if rv := context.Get(r, pluginUserKey); rv != nil {
		return rv.(*user.DBUser)
	}
	return nil
}
