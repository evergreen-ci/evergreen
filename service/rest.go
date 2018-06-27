package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// restContextKey is the type used to store
type restContextKey int

const RestContext restContextKey = 0

type restAPIService interface {
	GetSettings() evergreen.Settings
	LoggedError(http.ResponseWriter, *http.Request, int, error)
}

type restAPI struct {
	restAPIService
}

type restV1middleware struct {
	restAPIService
}

func (ra *restV1middleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	vars := gimlet.GetVars(r)
	ctx := r.Context()
	pctx, err := model.LoadContext(vars["task_id"], vars["build_id"], vars["version_id"], vars["patch_id"], vars["project_id"])
	if err != nil {
		// Some database lookup failed when fetching the data - log it
		ra.LoggedError(rw, r, http.StatusInternalServerError, errors.Wrap(err, "Error loading project context"))
		return
	}

	usr := gimlet.GetUser(ctx)

	if pctx.ProjectRef != nil && pctx.ProjectRef.Private && usr == nil {
		gimlet.WriteTextResponse(rw, http.StatusUnauthorized, "unauthorized")
		return
	}

	if pctx.Patch != nil && usr == nil {
		gimlet.WriteTextResponse(rw, http.StatusUnauthorized, "unauthorized")
		return
	}

	r = setRestContext(r, &pctx)
	next(rw, r)
}

// MustHaveRESTContext fetches the model.Context stored with the request, and panics if the key
// is not set.
func MustHaveRESTContext(r *http.Request) *model.Context {
	pc, err := GetRESTContext(r)
	if err != nil {
		panic(err)
	}
	return pc
}

// AttachRESTHandler attaches a router at the given root that hooks up REST endpoint URIs to be
// handled by the given restAPIService.
func GetRESTv1App(evgService restAPIService) *gimlet.APIApp {
	app := gimlet.NewApp()
	rest := &restAPI{evgService}
	app.ResetMiddleware()
	app.SetPrefix(evergreen.RestRoutePrefix)
	app.AddWrapper(&restV1middleware{rest})

	// REST routes
	app.AddRoute("/builds/{build_id}").Version(1).Get().Handler(rest.getBuildInfo)
	app.AddRoute("/builds/{build_id}/status").Version(1).Get().Handler(rest.getBuildStatus)
	app.AddRoute("/patches/{patch_id}").Version(1).Get().Handler(rest.getPatch)
	app.AddRoute("/patches/{patch_id}/config").Version(1).Get().Handler(rest.getPatchConfig)
	app.AddRoute("/projects").Version(1).Get().Handler(rest.getProjectIds)
	app.AddRoute("/projects/{project_id}").Version(1).Get().Handler(rest.getProject)
	app.AddRoute("/projects/{project_id}/last_green").Version(1).Get().Handler(rest.lastGreen)
	app.AddRoute("/projects/{project_id}/revisions/{revision}").Version(1).Get().Handler(rest.getVersionInfoViaRevision)
	app.AddRoute("/projects/{project_id}/test_history").Version(1).Get().Handler(rest.GetTestHistory)
	app.AddRoute("/projects/{project_id}/versions").Version(1).Get().Handler(rest.getRecentVersions)
	app.AddRoute("/scheduler/distro/{distro_id}/stats").Version(1).Get().Handler(rest.getAverageSchedulerStats)
	app.AddRoute("/scheduler/host_utilization").Version(1).Get().Handler(rest.getHostUtilizationStats)
	app.AddRoute("/scheduler/makespans").Version(1).Get().Handler(rest.getOptimalAndActualMakespans)
	app.AddRoute("/tasks/{task_id}").Version(1).Get().Handler(rest.getTaskInfo)
	app.AddRoute("/tasks/{task_id}/status").Version(1).Get().Handler(rest.getTaskStatus)
	app.AddRoute("/tasks/{task_name}/history").Version(1).Get().Handler(rest.getTaskHistory)
	app.AddRoute("/versions/{version_id}").Version(1).Get().Handler(rest.getVersionInfo)
	app.AddRoute("/versions/{version_id}").Version(1).Patch().Handler(requireUser(rest.modifyVersionInfo, nil))
	app.AddRoute("/versions/{version_id}/config").Version(1).Get().Handler(rest.getVersionConfig)
	app.AddRoute("/versions/{version_id}/status").Version(1).Get().Handler(rest.getVersionStatus)

	return app
}
