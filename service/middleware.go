package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/gimlet"
	"github.com/gorilla/csrf"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// Key used for storing variables in request context with type safety.
type reqCtxKey int

const (
	ProjectCookieName string = "mci-project-cookie"

	// Key values used to map user and project data to request context.
	// These are private custom types to avoid key collisions.
	RequestProjectContext reqCtxKey = iota
)

// projectContext defines the set of common fields required across most UI requests.
type projectContext struct {
	model.Context

	// AllProjects is a list of all available projects, limited to only the set of fields
	// necessary for display. If user is logged in, this will include private projects.
	AllProjects []restModel.UIProjectFields

	// AuthRedirect indicates whether or not redirecting during authentication is necessary.
	AuthRedirect bool

	PluginNames []string
}

// MustHaveProjectContext gets the projectContext from the request,
// or panics if it does not exist.
func MustHaveProjectContext(r *http.Request) projectContext {
	pc, err := GetProjectContext(r)
	if err != nil {
		panic(err)
	}
	return pc
}

// MustHaveUser gets the user from the request or
// panics if it does not exist.
func MustHaveUser(r *http.Request) *user.DBUser {
	u := gimlet.GetUser(r.Context())
	if u == nil {
		panic("no user attached to request")
	}
	usr, ok := u.(*user.DBUser)
	if !ok {
		panic("malformed user attached to request")
	}

	return usr
}

// GetSettings returns the global evergreen settings.
func (uis *UIServer) GetSettings() evergreen.Settings {
	return uis.Settings
}

// requireUser takes a request handler and returns a wrapped version which verifies that requests
// request are authenticated before proceeding. For a request which is not authenticated, it will
// execute the onFail handler. If onFail is nil, a simple "unauthorized" error will be sent.
func requireUser(onSuccess, onFail http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if user := gimlet.GetUser(r.Context()); user == nil {
			if onFail != nil {
				onFail(w, r)
				return
			}
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		onSuccess(w, r)
	}
}

func (uis *UIServer) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return requireUser(next, uis.RedirectToLogin)
}

func (uis *UIServer) requireLoginStatusUnauthorized(next http.HandlerFunc) http.HandlerFunc {
	return requireUser(next, nil)
}

func (uis *UIServer) setCORSHeaders(next http.HandlerFunc) http.HandlerFunc {
	return route.AddCORSHeaders(uis.Settings.Ui.CORSOrigins, next)
}

// wrapUserForMCP is a middleware that wraps the request with the user id from the x-authenticated-sage-user header.
// This is used to simulate a user for the MCP server. To ensure that the user has the correct permissions,
// This should only be used for MCP requests and should only be followed by a route that checks if the user has the correct permissions.
func (uis *UIServer) wrapUserForMCP(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userId := r.Header.Get(evergreen.SageUserHeader)
		if userId == "" {
			// Throw an error
			grip.Error(message.Fields{
				"message": "No user id found in request header",
				"request": r,
			})
			http.Error(w, "Invalid MCP request", http.StatusBadRequest)
			return
		}
		user, err := user.FindOneByIdContext(r.Context(), userId)
		if err != nil {
			grip.Error(message.Fields{
				"message": "Error finding user in request",
				"request": r,
				"error":   err,
				"userId":  userId,
			})
			http.Error(w, "Invalid MCP request", http.StatusBadRequest)
			return
		}
		if user == nil {
			grip.Error(message.Fields{
				"message": "User not found in request",
				"request": r,
				"userId":  userId,
			})
			http.Error(w, "Invalid MCP request", http.StatusBadRequest)
			return
		}
		ctx := gimlet.AttachUser(r.Context(), user)
		r = r.WithContext(ctx)
		next(w, r)
	}
}

func (uis *UIServer) ownsHost(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := gimlet.GetUser(r.Context())
		hostID := gimlet.GetVars(r)["host_id"]

		h, err := uis.getHostFromCache(r.Context(), hostID)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Errorf("can't get host '%s'", hostID))
			return
		}
		if h == nil {
			uis.LoggedError(w, r, http.StatusNotFound, errors.Errorf("host '%s' does not exist", hostID))
			return
		}
		if h.owner != user.Username() {
			uis.LoggedError(w, r, http.StatusUnauthorized, errors.New("not authorized for this action"))
			return
		}

		next(w, r)
	}
}

func (uis *UIServer) vsCodeRunning(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hostID := gimlet.GetVars(r)["host_id"]
		h, err := uis.getHostFromCache(r.Context(), hostID)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Errorf("can't get host '%s'", hostID))
			return
		}
		if h == nil {
			uis.LoggedError(w, r, http.StatusNotFound, errors.Errorf("host '%s' does not exist", hostID))
			return
		}

		if !h.isVirtualWorkstation {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("host '%s' is not a virtual workstation", hostID))
			return
		}

		if !h.isRunning {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("host '%s' is not running", hostID))
		}

		next(w, r)
	}
}

// RedirectToLogin forces a redirect to the login page. The redirect param is set on the query
// so that the user will be returned to the original page after they login.
func (uis *UIServer) RedirectToLogin(w http.ResponseWriter, r *http.Request) {
	querySep := ""
	if r.URL.RawQuery != "" {
		querySep = "?"
	}
	path := "/login#?"
	if uis.env.UserManager().IsRedirect() {
		path = "/login/redirect?"
	}
	location := fmt.Sprintf("%sredirect=%s%s%s",
		path,
		url.QueryEscape(r.URL.Path),
		querySep,
		r.URL.RawQuery)
	http.Redirect(w, r, location, http.StatusFound)
}

// Loads all Task/Build/Version/Patch/Project metadata and attaches it to the request.
// If the project is private but the user is not logged in, redirects to the login page.
func (uis *UIServer) loadCtx(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		usr := gimlet.GetUser(r.Context())
		if usr == nil {
			uis.RedirectToLogin(w, r)
			return
		}
		projCtx, err := uis.loadProjectContext(w, r)
		if err != nil {
			// Some database lookup failed when fetching the data - log it
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error loading project context"))
			return
		}
		if projCtx.ProjectRef != nil {
			opts := gimlet.PermissionOpts{
				Resource:      projCtx.ProjectRef.Id,
				ResourceType:  evergreen.ProjectResourceType,
				Permission:    evergreen.PermissionTasks,
				RequiredLevel: evergreen.TasksView.Value,
			}
			if !usr.HasPermission(r.Context(), opts) {
				uis.LoggedError(w, r, http.StatusUnauthorized, errors.New("not authorized for this action"))
				return
			}
		}

		r = setUIRequestContext(r, projCtx)
		next(w, r)
	}
}

// populateProjectRefs loads all enabled project refs into the context.
func (pc *projectContext) populateProjectRefs(ctx context.Context) error {
	allProjs, err := model.FindAllMergedEnabledTrackedProjectRefs(ctx)
	if err != nil {
		return err
	}
	pc.AllProjects = make([]restModel.UIProjectFields, 0, len(allProjs))
	for _, p := range allProjs {
		uiProj := restModel.UIProjectFields{
			DisplayName: p.DisplayName,
			Identifier:  p.Identifier,
			Id:          p.Id,
			Repo:        p.Repo,
			Owner:       p.Owner,
		}
		pc.AllProjects = append(pc.AllProjects, uiProj)
	}
	return nil
}

// getRequestProjectId determines the projectId to associate with the request context,
// in cases where it could not be inferred from a task/build/version/patch etc.
// The projectId is determined using the following criteria in order of priority:
// 1. The projectId inferred by ProjectContext (checked outside this func)
// 2. The value of the project_id in the URL if present.
// 3. The value set in the request cookie, if present.
// 4. The default project in the UI settings, if present.
// 5. The first project in the complete list of all project refs.
func (uis *UIServer) getRequestProjectId(r *http.Request) string {
	projectId := gimlet.GetVars(r)["project_id"]
	if len(projectId) > 0 {
		return projectId
	}

	cookie, err := r.Cookie(ProjectCookieName)
	if err == nil && len(cookie.Value) > 0 {
		return cookie.Value
	}

	return uis.Settings.Ui.DefaultProject
}

// LoadProjectContext builds a projectContext from vars in the request's URL.
// This is done by reading in specific variables and inferring other required
// context variables when necessary (e.g. loading a project based on the task).
func (uis *UIServer) loadProjectContext(rw http.ResponseWriter, r *http.Request) (projectContext, error) {
	dbUser := MustHaveUser(r)

	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	buildId := vars["build_id"]
	versionId := vars["version_id"]
	patchId := vars["patch_id"]

	pc := projectContext{AuthRedirect: uis.env.UserManager().IsRedirect()}
	err := pc.populateProjectRefs(r.Context())
	if err != nil {
		return pc, err
	}
	requestProjectId := uis.getRequestProjectId(r)
	projectId := ""
	for _, p := range pc.AllProjects {
		if p.Identifier == requestProjectId || p.Id == requestProjectId {
			projectId = p.Id
		}
	}
	if projectId != "" {
		opts := gimlet.PermissionOpts{
			Resource:      projectId,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionTasks,
			RequiredLevel: evergreen.TasksView.Value,
		}
		if !dbUser.HasPermission(r.Context(), opts) {
			projectId = ""
		}
	}

	// If we still don't have a default projectId, just use the first
	// project in the list that the user has read access to.
	if projectId == "" {
		for _, p := range pc.AllProjects {
			opts := gimlet.PermissionOpts{
				Resource:      p.Id,
				ResourceType:  evergreen.ProjectResourceType,
				Permission:    evergreen.PermissionTasks,
				RequiredLevel: evergreen.TasksView.Value,
			}
			if dbUser.HasPermission(r.Context(), opts) {
				projectId = p.Id
				break
			}
		}
	}

	// Build a model.Context using the data available.
	ctx, err := model.LoadContext(r.Context(), taskId, buildId, versionId, patchId, projectId)
	pc.Context = ctx
	if err != nil {
		return pc, err
	}

	// set the cookie for the next request if a project was found
	if ctx.ProjectRef != nil {
		http.SetCookie(rw, &http.Cookie{
			Name:    ProjectCookieName,
			Value:   ctx.ProjectRef.Id,
			Path:    "/",
			Expires: time.Now().Add(7 * 24 * time.Hour),
			Secure:  true,
		})
	}

	return pc, nil
}

// ForbiddenHandler logs a rejected request befure returning a 403 to the client
func ForbiddenHandler(w http.ResponseWriter, r *http.Request) {
	reason := csrf.FailureReason(r)
	grip.Warning(message.Fields{
		"action": "forbidden",
		"method": r.Method,
		"remote": r.RemoteAddr,
		"path":   r.URL.Path,
		"reason": reason.Error(),
	})

	http.Error(w, fmt.Sprintf("%s - %s",
		http.StatusText(http.StatusForbidden), reason),
		http.StatusForbidden)
}
