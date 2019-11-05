package rest

import (
	"net/http"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
)

// ManagementGroupService provides the reporting service
// impelementation for queue groups.
type ManagementGroupService struct {
	group amboy.QueueGroup
}

// NewManagementGroupService returns a service that defines REST routes can
// manage the abortable pools of a QueueGroup.
func NewManagementGroupService(g amboy.QueueGroup) *ManagementGroupService {
	return &ManagementGroupService{
		group: g,
	}
}

// App returns a gimlet app with all of the routes registered.
func (s *ManagementGroupService) App() *gimlet.APIApp {
	app := gimlet.NewApp()

	app.AddRoute("/jobs/list").Version(1).Get().Handler(s.ListJobs)
	app.AddRoute("/jobs/abort").Version(1).Delete().Handler(s.AbortAllJobs)
	app.AddRoute("/job/{name}").Version(1).Get().Handler(s.GetJobStatus)
	app.AddRoute("/job/{name}").Version(1).Delete().Handler(s.AbortRunningJob)
	return app
}

// ListJobs is an http.HandlerFunc that returns a list of all running
// jobs in all pools for the queue group.
func (s *ManagementGroupService) ListJobs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	jobs := []string{}
	for _, group := range s.group.Queues(ctx) {
		if queue, err := s.group.Get(ctx, group); err == nil {
			if pool, ok := queue.Runner().(amboy.AbortableRunner); ok {
				jobs = append(jobs, pool.RunningJobs()...)
			}
		}
	}

	gimlet.WriteJSON(rw, jobs)
}

// AbortAllJobs is an http.HandlerFunc that sends the signal to abort
// all running jobs in the pools of the group. May return a 408 (timeout) if the
// calling context was canceled before the operation
// returned. Otherwise, this handler returns 200. The body of the
// response is always empty.
func (s *ManagementGroupService) AbortAllJobs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	for _, group := range s.group.Queues(ctx) {
		if queue, err := s.group.Get(ctx, group); err == nil {
			if pool, ok := queue.Runner().(amboy.AbortableRunner); ok {
				pool.AbortAll(ctx)
				if ctx.Err() != nil {
					gimlet.WriteJSONResponse(rw, http.StatusRequestTimeout, struct{}{})
					return
				}
			}
		}
	}

	gimlet.WriteJSON(rw, struct{}{})
}

// GetJobStatus is an http.HandlerFunc reports on the status (running
// or not running) of a specific job.
func (s *ManagementGroupService) GetJobStatus(rw http.ResponseWriter, r *http.Request) {
	name := gimlet.GetVars(r)["name"]

	ctx := r.Context()

	for _, group := range s.group.Queues(ctx) {
		if queue, err := s.group.Get(ctx, group); err == nil {
			if pool, ok := queue.Runner().(amboy.AbortableRunner); ok {
				if pool.IsRunning(name) {
					gimlet.WriteJSON(rw, map[string]string{
						"name":   name,
						"status": "running",
						"group":  group,
					})
					return
				}
			}
		}
	}

	gimlet.WriteJSONResponse(rw, http.StatusNotFound,
		map[string]string{
			"name":   name,
			"status": "not running",
		})
}

// AbortRunningJob is an http.HandlerFunc that terminates the
// execution of a single running job, returning a 400 response when
// the job doesn't exist.
func (s *ManagementGroupService) AbortRunningJob(rw http.ResponseWriter, r *http.Request) {
	name := gimlet.GetVars(r)["name"]

	ctx := r.Context()

	for _, group := range s.group.Queues(ctx) {
		if queue, err := s.group.Get(ctx, group); err == nil {
			if pool, ok := queue.Runner().(amboy.AbortableRunner); ok {
				if err = pool.Abort(ctx, name); err == nil {
					gimlet.WriteJSON(rw, map[string]string{
						"name":   name,
						"status": "aborted",
						"group":  group,
					})
					return
				}
			}
		}
	}

	gimlet.WriteJSONResponse(rw, http.StatusNotFound,
		map[string]string{
			"name":   name,
			"status": "unknown",
		})
}
