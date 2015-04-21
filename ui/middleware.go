package ui

import (
	"10gen.com/mci"
	"10gen.com/mci/auth"
	"10gen.com/mci/model"
	"10gen.com/mci/model/build"
	"10gen.com/mci/model/patch"
	"10gen.com/mci/model/user"
	"10gen.com/mci/model/version"
	"10gen.com/mci/plugin"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/codegangsta/negroni"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

type (
	//  special types used as key types in the request context map to prevent key collisions.
	userKey           int
	projectContextKey int

	projectContext struct {
		// The task specified in the request, if applicable.
		Task *model.Task
		// The build associated with the request, if applicable.
		Build *build.Build
		// The version associated with the request, if applicable.
		Version *version.Version
		// The patch associated with the request, if applicable.
		Patch *patch.Patch
		// The project which the task/build/version in this request is a part of, if applicable.
		Project    *model.Project
		ProjectRef *model.ProjectRef

		// A list of all available projects. If user is logged in, will include private projects.
		AllProjects []model.ProjectRef
	}
)

const (
	// Key values used to map user and project data to request context.
	// These are private custom types to avoid key collisions.
	myUserKey    userKey           = 0
	myProjCtxKey projectContextKey = 0
)

// GetUser returns a user if one is attached to the request. Returns nil if the user is not logged
// in, assuming that the middleware to lookup user information is enabled on the request handler.
func GetUser(r *http.Request) *user.DBUser {
	if rv := context.Get(r, myUserKey); rv != nil {
		return rv.(*user.DBUser)
	}
	return nil
}

// GetProjectContext fetches the projectContext associated with the request. Returns an error
// if no projectContext has been loaded and attached to the request.
func GetProjectContext(r *http.Request) (projectContext, error) {
	if rv := context.Get(r, myProjCtxKey); rv != nil {
		return rv.(projectContext), nil
	}
	return projectContext{}, fmt.Errorf("No context loaded")
}

func MustHaveProjectContext(r *http.Request) projectContext {
	pc, err := GetProjectContext(r)
	if err != nil {
		panic(err)
	}
	return pc
}

func MustHaveUser(r *http.Request) *user.DBUser {
	u := GetUser(r)
	if u == nil {
		panic("no user attached to request")
	}
	return u
}

func (pc projectContext) ToPluginContext(mciSettings mci.MCISettings, user *user.DBUser) plugin.UIContext {
	return plugin.UIContext{
		MCISettings: mciSettings,
		User:        user,
		Task:        pc.Task,
		Build:       pc.Build,
		Version:     pc.Version,
		Patch:       pc.Patch,
		Project:     pc.Project,
		ProjectRef:  pc.ProjectRef,
	}
}

func (uis *UIServer) GetMCISettings() mci.MCISettings {
	return uis.MCISettings
}

// requireUser takes a request handler and returns a wrapped version which verifies that requests
// request are authenticated before proceeding. For a request which is not authenticated, it will
// be redirected to the login page instead.
func (uis *UIServer) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if GetUser(r) == nil {
			uis.RedirectToLogin(w, r)
			return
		}
		next(w, r)
	}
}

// requireSuperUser takes a request handler and returns a wrapped version which verifies that
// the requester is authenticated as a superuser. For a requester who isn't a super user, the
// request will be redirected to the login page instead.
func (uis *UIServer) requireSuperUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(uis.MCISettings.SuperUsers) == 0 {
			next(w, r)
			return
		}

		if user := GetUser(r); user != nil {
			for _, id := range uis.MCISettings.SuperUsers {
				if id == user.Id {
					next(w, r)
					return
				}
			}
		}
		uis.RedirectToLogin(w, r)
		return
	}
}

// Forces a redirect to the login page. The redirect param is set on the query
// so that the user will be returned to the original page after they login
func (uis *UIServer) RedirectToLogin(w http.ResponseWriter, r *http.Request) {
	querySep := ""
	if r.URL.RawQuery != "" {
		querySep = "?"
	}
	location := fmt.Sprintf("%v/login?redirect=%v%v%v",
		uis.MCISettings.Ui.Url,
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
			uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error loading project context: %v", err))
			return
		}
		if projCtx.ProjectRef != nil && projCtx.ProjectRef.Private && GetUser(r) == nil {
			uis.RedirectToLogin(w, r)
			return
		}

		if projCtx.Patch != nil && GetUser(r) == nil {
			uis.RedirectToLogin(w, r)
			return
		}

		context.Set(r, myProjCtxKey, projCtx)
		next(w, r)
	}
}

// populateTaskBuildVersion takes a task, build, and version ID and populates a projectContext
// with as many of the task, build, and version documents as possible.
// If any of the provided IDs is blank, they will be inferred from the more selective ones.
// Returns the project ID of the data found, which may be blank if the IDs are empty.
func (pc *projectContext) populateTaskBuildVersion(taskId, buildId, versionId string) (string, error) {
	projectId := ""
	var err error
	// Fetch task if there's a task ID present; if we find one, populate build/version IDs from it
	if len(taskId) > 0 {
		pc.Task, err = model.FindTask(taskId)
		if err != nil {
			return "", err
		}

		if pc.Task != nil {
			// override build and version ID with the ones this task belongs to
			buildId = pc.Task.BuildId
			versionId = pc.Task.Version
			projectId = pc.Task.Project
		}
	}

	// Fetch build if there's a build ID present; if we find one, populate version ID from it
	if len(buildId) > 0 {
		pc.Build, err = build.FindOne(build.ById(buildId))
		if err != nil {
			return "", err
		}
		if pc.Build != nil {
			versionId = pc.Build.Version
			projectId = pc.Build.Project
		}
	}
	if len(versionId) > 0 {
		pc.Version, err = version.FindOne(version.ById(versionId))
		if err != nil {
			return "", err
		}
		if pc.Version != nil {
			projectId = pc.Version.Project
		}
	}
	return projectId, nil

}

// populateProjectRefs loads all project refs into the context. If includePrivate is true,
// all available projects will be included, otherwise only public projects will be loaded.
func (pc *projectContext) populateProjectRefs(includePrivate bool) error {
	allProjs, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return err
	}
	pc.AllProjects = make([]model.ProjectRef, 0, len(allProjs))
	// User is not logged in, so only include public projects.
	for _, p := range allProjs {
		if !p.Enabled {
			continue
		}
		if !p.Private || includePrivate {
			pc.AllProjects = append(pc.AllProjects, p)
		}
	}
	return nil
}

// populatePatch loads a patch into the project context, using patchId if provided.
// If patchId is blank, will try to infer the patch ID from the version already loaded
// into context, if available.
func (pc *projectContext) populatePatch(patchId string) error {
	var err error
	if len(patchId) > 0 {
		// The patch is explicitly identified in the URL, so fetch it
		if !patch.IsValidId(patchId) {
			return fmt.Errorf("patch id '%v' is not an object id", patchId)
		}
		pc.Patch, err = patch.FindOne(patch.ById(patch.NewId(patchId)).Project(patch.ExcludePatchDiff))
	} else if pc.Version != nil {
		// patch isn't in URL but the version in context has one, get it
		pc.Patch, err = patch.FindOne(patch.ByVersion(pc.Version.Id).Project(patch.ExcludePatchDiff))
	}
	if err != nil {
		return err
	}

	// If there's a finalized patch loaded into context but not a version, load the version
	// associated with the patch as the context's version.
	if pc.Version == nil && pc.Patch != nil && pc.Patch.Version != "" {
		pc.Version, err = version.FindOne(version.ById(pc.Patch.Version))
		if err != nil {
			return err
		}
	}
	return nil
}

// Builds a projectContext from vars in the URL.
func (uis *UIServer) LoadProjectContext(rw http.ResponseWriter, r *http.Request) (projectContext, error) {
	user := GetUser(r)
	vars := mux.Vars(r)

	proj := &projectContext{}

	taskId := vars["task_id"]
	buildId := vars["build_id"]
	versionId := vars["version_id"]
	patchId := vars["patch_id"]

	projectId, err := proj.populateTaskBuildVersion(taskId, buildId, versionId)
	if err != nil {
		return *proj, err
	}

	err = proj.populatePatch(patchId)
	if err != nil {
		return *proj, err
	}
	if proj.Patch != nil && len(projectId) == 0 {
		projectId = proj.Patch.Project
	}

	// Try to infer project ID - if we don't already have it from the task/build/version, try to
	// get it from the URL
	if len(projectId) == 0 {
		projectId = vars["project_id"]
	}

	// Still don't have a project ID to use, check if the user's cookie contains one
	if len(projectId) == 0 {
		cookie, err := r.Cookie(ProjectCookieName)
		if err == nil {
			projectId = cookie.Value
		}
	}

	// Still no project ID found anywhere, just use the default one according to config.
	if len(projectId) == 0 {
		projectId = uis.MCISettings.Ui.DefaultProject
	}

	// Try to load project for the ID we found, and set cookie with it for subsequent requests
	if len(projectId) > 0 {
		// Also lookup the ProjectRef itself and add it to context.
		proj.ProjectRef, err = model.FindOneProjectRef(projectId)
		if err != nil {
			return *proj, err
		}
		if proj.ProjectRef != nil {
			proj.Project, err = model.FindProject("", proj.ProjectRef)
			if err != nil {
				return *proj, err
			}

			if proj.Project != nil {
				// A project was found, update the project cookie for subsequent request.
				http.SetCookie(rw, &http.Cookie{
					Name:    ProjectCookieName,
					Value:   projectId,
					Path:    "",
					Expires: time.Now().Add(7 * 24 * time.Hour),
				})

			}
		}
	}
	err = proj.populateProjectRefs(user != nil)
	if err != nil {
		return *proj, err
	}

	return *proj, nil
}

// Middleware which checks for session tokens on the request, and looks up and attaches a user
// for that token if one is found.
func UserMiddleware(um auth.UserManager) func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	return func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		token := ""
		var err error
		for _, cookie := range r.Cookies() {
			if cookie.Name == mci.AuthTokenCookie {
				if token, err = url.QueryUnescape(cookie.Value); err == nil {
					break
				}
			}
		}
		if len(token) > 0 {
			user, err := um.GetUserByToken(token)
			if err != nil {
				mci.Logger.Logf(slogger.INFO, "Error getting user: %v", err)
			} else {
				// Get the user's full details from the DB or create them if they don't exists
				dbUser, err := model.GetOrCreateUser(user.Username(), user.DisplayName(), user.Email())
				if err != nil {
					mci.Logger.Logf(slogger.INFO, "Error looking up user %v: %v", user.Username(), err)
				} else {
					context.Set(r, myUserKey, dbUser)
				}
			}
		}

		next(rw, r)
	}
}

// Logger is a middleware handler that logs the request as it goes in and the response as it goes out.
type Logger struct {
	// Logger inherits from log.Logger used to log messages with the Logger middleware
	*log.Logger
	// ids is a channel producing unique, autoincrementing request ids that are included in logs.
	ids chan int
}

// NewLogger returns a new Logger instance
func NewLogger() *Logger {
	ids := make(chan int, 100)
	go func() {
		reqId := 0
		for {
			ids <- reqId
			reqId++
		}
	}()

	return &Logger{log.New(os.Stdout, "[evergreen] ", 0), ids}
}

func (l *Logger) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	reqId := <-l.ids
	l.Printf("Started (%v) %s %s %s", reqId, r.Method, r.URL.Path, r.RemoteAddr)

	next(rw, r)

	res := rw.(negroni.ResponseWriter)
	l.Printf("Completed (%v) %v %s in %v", reqId, res.Status(), http.StatusText(res.Status()), time.Since(start))
}
