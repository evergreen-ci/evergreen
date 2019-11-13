package rest

import (
	"net/http"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

// ManagementService defines a set of rest routes that make it
// possible to remotely manage the jobs running in an abortable pool.
type ManagementService struct {
	pool amboy.AbortableRunner
}

// NewManagementService returns a service that defines REST routes can
// manage an abortable pool.
func NewManagementService(p amboy.AbortableRunner) *ManagementService {
	return &ManagementService{
		pool: p,
	}
}

// App returns a gimlet app with all of the routes registered.
func (s *ManagementService) App() *gimlet.APIApp {
	app := gimlet.NewApp()

	app.AddRoute("/jobs/list").Version(1).Get().Handler(s.ListJobs)
	app.AddRoute("/jobs/abort").Version(1).Delete().Handler(s.AbortAllJobs)
	app.AddRoute("/job/{name}").Version(1).Get().Handler(s.GetJobStatus)
	app.AddRoute("/job/{name}").Version(1).Delete().Handler(s.AbortRunningJob)

	return app
}

// ListJobs is an http.HandlerFunc that returns a list of all running
// jobs in the pool.
func (s *ManagementService) ListJobs(rw http.ResponseWriter, r *http.Request) {
	jobs := s.pool.RunningJobs()

	gimlet.WriteJSON(rw, jobs)
}

// AbortAllJobs is an http.HandlerFunc that sends the signal to abort
// all running jobs in the pool. May return a 408 (timeout) if the
// calling context was canceled before the operation
// returned. Otherwise, this handler returns 200. The body of the
// response is always empty.
func (s *ManagementService) AbortAllJobs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	s.pool.AbortAll(ctx)

	if ctx.Err() != nil {
		gimlet.WriteJSONResponse(rw, http.StatusRequestTimeout, struct{}{})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

// GetJobStatus is an http.HandlerFunc reports on the status (running
// or not running) of a specific job.
func (s *ManagementService) GetJobStatus(rw http.ResponseWriter, r *http.Request) {
	name := gimlet.GetVars(r)["name"]

	if !s.pool.IsRunning(name) {
		gimlet.WriteJSONResponse(rw, http.StatusNotFound,
			map[string]string{
				"name":   name,
				"status": "not running",
			})
		return
	}

	gimlet.WriteJSON(rw, map[string]string{
		"name":   name,
		"status": "running",
	})
}

// AbortRunningJob is an http.HandlerFunc that terminates the
// execution of a single running job, returning a 400 response when
// the job doesn't exist.
func (s *ManagementService) AbortRunningJob(rw http.ResponseWriter, r *http.Request) {
	name := gimlet.GetVars(r)["name"]
	ctx := r.Context()
	err := s.pool.Abort(ctx, name)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrapf(err,
			"problem aborting job '%s'", name)))
	}

	gimlet.WriteJSON(rw, map[string]string{
		"name":   name,
		"status": "aborted",
	})
}
