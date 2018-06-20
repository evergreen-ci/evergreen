package service

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/urfave/negroni"
)

// Key used for storing variables in request context with type safety.
type (
	reqCtxKey int
)

type (
	// projectContext defines the set of common fields required across most UI requests.
	projectContext struct {
		model.Context

		// AllProjects is a list of all available projects, limited to only the set of fields
		// necessary for display. If user is logged in, this will include private projects.
		AllProjects []UIProjectFields

		// AuthRedirect indicates whether or not redirecting during authentication is necessary.
		AuthRedirect bool

		// IsAdmin indicates if the user is an admin for at least one of the projects
		// listed in AllProjects.
		IsAdmin bool

		PluginNames []string
	}
)

const (
	// Key values used to map user and project data to request context.
	// These are private custom types to avoid key collisions.
	RequestTask reqCtxKey = iota
	RequestProjectContext
	requestID

	remoteAddrHeaderName = "X-Cluster-Client-Ip"
)

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

// ToPluginContext creates a UIContext from the projectContext data.
func (pc projectContext) ToPluginContext(settings evergreen.Settings, usr gimlet.User) plugin.UIContext {
	dbUser, ok := usr.(*user.DBUser)
	grip.CriticalWhen(!ok, message.Fields{
		"message":  "problem converting user interface to db record",
		"location": "service/middleware.ToPluginContext",
		"cause":    "programmer error",
		"type":     fmt.Sprintf("%T", usr),
	})

	return plugin.UIContext{
		Settings:   settings,
		User:       dbUser,
		Task:       pc.Task,
		Build:      pc.Build,
		Version:    pc.Version,
		Patch:      pc.Patch,
		ProjectRef: pc.ProjectRef,
	}

}

// GetSettings returns the global evergreen settings.
func (uis *UIServer) GetSettings() evergreen.Settings {
	return uis.Settings
}

// requireAdmin takes in a request handler and returns a wrapped version which verifies that requests are
// authenticated and that the user is either a super user or is part of the project context's project's admins.
func (uis *UIServer) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// get the project context
		projCtx := MustHaveProjectContext(r)
		if dbUser := gimlet.GetUser(ctx); dbUser != nil {
			if uis.isSuperUser(dbUser) || isAdmin(dbUser, projCtx.ProjectRef) {
				next(w, r)
				return
			}
		}

		uis.RedirectToLogin(w, r)
	}
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

// requireSuperUser takes a request handler and returns a wrapped version which verifies that
// the requester is authenticated as a superuser. For a requester who isn't a super user, the
// request will be redirected to the login page instead.
func (uis *UIServer) requireSuperUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(uis.Settings.SuperUsers) == 0 {
			f := requireUser(next, uis.RedirectToLogin) // Still must be user to proceed
			f(w, r)
			return
		}

		usr := gimlet.GetUser(r.Context())
		if usr != nil && uis.isSuperUser(usr) {
			next(w, r)
			return
		}
		uis.RedirectToLogin(w, r)
	}
}

// isSuperUser verifies that a given user has super user permissions.
// A user has these permission if they are in the super users list or if the list is empty,
// in which case all users are super users.
func (uis *UIServer) isSuperUser(u gimlet.User) bool {
	if util.StringSliceContains(uis.Settings.SuperUsers, u.Username()) ||
		len(uis.Settings.SuperUsers) == 0 {
		return true
	}

	return false

}

// isAdmin returns false if the user is nil or if its id is not
// located in ProjectRef's Admins field.
func isAdmin(u gimlet.User, project *model.ProjectRef) bool {
	return util.StringSliceContains(project.Admins, u.Username())
}

// RedirectToLogin forces a redirect to the login page. The redirect param is set on the query
// so that the user will be returned to the original page after they login.
func (uis *UIServer) RedirectToLogin(w http.ResponseWriter, r *http.Request) {
	querySep := ""
	if r.URL.RawQuery != "" {
		querySep = "?"
	}
	path := "/login#?"
	if uis.UserManager.IsRedirect() {
		path = "login/redirect?"
	}
	location := fmt.Sprintf("%v%vredirect=%v%v%v",
		uis.Settings.Ui.Url,
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
		projCtx, err := uis.LoadProjectContext(w, r)
		if err != nil {
			// Some database lookup failed when fetching the data - log it
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error loading project context"))
			return
		}
		usr := gimlet.GetUser(r.Context())

		if projCtx.ProjectRef != nil && projCtx.ProjectRef.Private && usr == nil {
			uis.RedirectToLogin(w, r)
			return
		}

		if projCtx.Patch != nil && usr == nil {
			uis.RedirectToLogin(w, r)
			return
		}

		r = setUIRequestContext(r, projCtx)

		next(w, r)
	}
}

// populateProjectRefs loads all project refs into the context. If includePrivate is true,
// all available projects will be included, otherwise only public projects will be loaded.
// Sets IsAdmin to true if the user id is located in a project's admin list.
func (pc *projectContext) populateProjectRefs(includePrivate, isSuperUser bool, user gimlet.User) error {
	allProjs, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return err
	}
	pc.AllProjects = make([]UIProjectFields, 0, len(allProjs))
	// User is not logged in, so only include public projects.
	for _, p := range allProjs {
		if !p.Enabled {
			continue
		}
		if !p.Private || includePrivate {
			uiProj := UIProjectFields{
				DisplayName: p.DisplayName,
				Identifier:  p.Identifier,
				Repo:        p.Repo,
				Owner:       p.Owner,
			}
			pc.AllProjects = append(pc.AllProjects, uiProj)
		}

		if includePrivate && (isSuperUser || isAdmin(user, &p)) {
			pc.IsAdmin = true
		}
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
	vars := mux.Vars(r)
	projectId := vars["project_id"]
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
func (uis *UIServer) LoadProjectContext(rw http.ResponseWriter, r *http.Request) (projectContext, error) {
	dbUser := gimlet.GetUser(r.Context())

	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	buildId := vars["build_id"]
	versionId := vars["version_id"]
	patchId := vars["patch_id"]

	projectId := uis.getRequestProjectId(r)

	pc := projectContext{AuthRedirect: uis.UserManager.IsRedirect()}
	isSuperUser := (dbUser != nil) && auth.IsSuperUser(uis.Settings.SuperUsers, dbUser)
	err := pc.populateProjectRefs(dbUser != nil, isSuperUser, dbUser)
	if err != nil {
		return pc, err
	}

	// If we still don't have a default projectId, just use the first project in the list
	// if there is one.
	if len(projectId) == 0 && len(pc.AllProjects) > 0 {
		projectId = pc.AllProjects[0].Identifier
	}

	// Build a model.Context using the data available.
	ctx, err := model.LoadContext(taskId, buildId, versionId, patchId, projectId)
	pc.Context = ctx
	if err != nil {
		return pc, err
	}

	// set the cookie for the next request if a project was found
	if ctx.ProjectRef != nil {
		http.SetCookie(rw, &http.Cookie{
			Name:    ProjectCookieName,
			Value:   ctx.ProjectRef.Identifier,
			Path:    "/",
			Expires: time.Now().Add(7 * 24 * time.Hour),
		})
	}

	if len(uis.GetAppPlugins()) > 0 {
		pluginNames := []string{}
		for _, p := range uis.GetAppPlugins() {
			pluginNames = append(pluginNames, p.Name())
		}
		pc.PluginNames = pluginNames
	}

	return pc, nil
}

func GetUserMiddlewareConf() gimlet.UserMiddlewareConfiguration {
	return gimlet.UserMiddlewareConfiguration{
		CookieName:     evergreen.AuthTokenCookie,
		HeaderKeyName:  evergreen.APIKeyHeader,
		HeaderUserName: evergreen.APIUserHeader,
	}
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

// RecoveryLogger is a middleware handler that logs the request as it goes in and the response as it goes out.
type RecoveryLogger struct {
	// ids is a channel producing unique, autoincrementing request ids that are included in logs.
	ids chan int
}

// NewRecoveryLogger returns negroni middleware that logs each
// request, and recovers from panics encountered during request processing.
func NewRecoveryLogger() *RecoveryLogger {
	ids := make(chan int, 100)
	go func() {
		reqId := 0
		for {
			ids <- reqId
			reqId++
		}
	}()

	return &RecoveryLogger{ids}
}

func (l *RecoveryLogger) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	reqID := <-l.ids

	r = setRequestID(r, reqID)

	remote := r.Header.Get(remoteAddrHeaderName)
	if remote == "" {
		remote = r.RemoteAddr
	}

	grip.Debug(message.Fields{
		"action":  "started",
		"method":  r.Method,
		"remote":  remote,
		"request": reqID,
		"path":    r.URL.Path,
	})

	defer func() {
		if err := recover(); err != nil {
			if rw.Header().Get("Content-Type") == "" {
				rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
			}

			rw.WriteHeader(http.StatusInternalServerError)

			grip.Critical(message.WrapStack(2, message.Fields{
				"panic":    err,
				"action":   "aborted",
				"request":  reqID,
				"duration": time.Since(start),
				"path":     r.URL.Path,
				"span":     time.Since(start).String(),
			}))
		}
	}()

	next(rw, r)

	res := rw.(negroni.ResponseWriter)

	grip.Info(message.Fields{
		"method":   r.Method,
		"remote":   remote,
		"request":  reqID,
		"path":     r.URL.Path,
		"duration": time.Since(start),
		"action":   "completed",
		"status":   res.Status(),
		"outcome":  http.StatusText(res.Status()),
		"span":     time.Since(start).String(),
	})
}
