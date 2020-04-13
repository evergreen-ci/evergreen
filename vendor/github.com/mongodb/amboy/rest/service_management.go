package rest

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy/management"
	"github.com/pkg/errors"
)

// ManagementService wraps a manager instance as described in the management
// package and provides an HTTP interface for all of the methods provided by
// methods provided by the manager.
type ManagementService struct {
	manager management.Management
}

// NewManagementService constructs a management service from the manager
// provided.
func NewManagementService(m management.Management) *ManagementService {
	return &ManagementService{
		manager: m,
	}
}

// App returns a gimlet application with all of the routes
// configured.
func (s *ManagementService) App() *gimlet.APIApp {
	app := gimlet.NewApp()

	app.AddRoute("/status/{filter}").Version(1).Get().Handler(s.GetJobStatus)
	app.AddRoute("/status/{filter}/{type}").Version(1).Get().Handler(s.GetJobStatusByType)
	app.AddRoute("/timing/{filter}/{seconds}").Version(1).Get().Handler(s.GetRecentTimings)
	app.AddRoute("/errors/{filter}/{seconds}").Version(1).Get().Handler(s.GetRecentErrors)
	app.AddRoute("/errors/{filter}/{type}/{seconds}").Version(1).Get().Handler(s.GetRecentErrorsByType)
	app.AddRoute("/jobs/mark_complete/{name}").Version(1).Post().Handler(s.MarkComplete)
	app.AddRoute("/jobs/mark_complete_by_type/{type}/{filter}").Version(1).Post().Handler(s.MarkCompleteByType)
	app.AddRoute("/jobs/mark_many_complete/{filter}").Version(1).Post().Handler(s.MarkManyComplete)

	return app
}

// GetJobStatus is an http.HandlerFunc that provides access to counts
// of all jobs that match a defined filter.
func (s *ManagementService) GetJobStatus(rw http.ResponseWriter, r *http.Request) {
	filter := management.StatusFilter(gimlet.GetVars(r)["filter"])
	ctx := r.Context()

	err := filter.Validate()
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	data, err := s.manager.JobStatus(ctx, filter)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	gimlet.WriteJSON(rw, data)
}

// GetJobStatusByType is an http.HandlerFunc that produces a list of job IDs for
// jobs that match a defined filter.
func (s *ManagementService) GetJobStatusByType(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	filter := management.StatusFilter(vars["filter"])
	jobType := vars["type"]

	if err := filter.Validate(); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	ctx := r.Context()
	data, err := s.manager.JobIDsByState(ctx, jobType, filter)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	gimlet.WriteJSON(rw, data)
}

// GetRecentTimings is an http.HandlerFunc that produces a report that lists the average runtime
// (duration) or latency of jobs.
func (s *ManagementService) GetRecentTimings(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	dur, err := time.ParseDuration(vars["seconds"])
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrapf(err,
			"problem parsing duration from %s", vars["seconds"])))
		return
	}

	filter := management.RuntimeFilter(vars["filter"])
	if err = filter.Validate(); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	ctx := r.Context()
	data, err := s.manager.RecentTiming(ctx, dur, filter)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	gimlet.WriteJSON(rw, data)
}

// GetRecentErrors is an http.HandlerFunc that returns an error report
// including number of errors, total number of jobs, grouped by type,
// with the error messages. Uses a filter that can optionally remove
// duplicate errors.
func (s *ManagementService) GetRecentErrors(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)

	dur, err := time.ParseDuration(vars["seconds"])
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrapf(err,
			"problem parsing duration from %s", vars["seconds"])))
		return
	}

	filter := management.ErrorFilter(vars["filter"])
	if err = filter.Validate(); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	ctx := r.Context()
	data, err := s.manager.RecentErrors(ctx, dur, filter)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	gimlet.WriteJSON(rw, data)
}

// GetRecentErrorsByType is an http.Handlerfunc returns an errors report for
// only a single type of jobs.
func (s *ManagementService) GetRecentErrorsByType(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	jobType := vars["type"]

	dur, err := time.ParseDuration(vars["seconds"])
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrapf(err,
			"problem parsing duration from %s", vars["seconds"])))
		return
	}

	filter := management.ErrorFilter(vars["filter"])
	if err = filter.Validate(); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	ctx := r.Context()
	data, err := s.manager.RecentJobErrors(ctx, jobType, dur, filter)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	gimlet.WriteJSON(rw, data)
}

// MarkComplete is an http.Handlerfunc marks the given job complete.
func (s *ManagementService) MarkComplete(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	name := vars["name"]

	ctx := r.Context()
	if err := s.manager.CompleteJob(ctx, name); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeTextErrorResponder(errors.Wrapf(err,
			"problem complete job '%s'", name)))
	}

	gimlet.WriteJSON(rw, struct {
		Message string `json:"message"`
		JobName string `json:"job_name"`
	}{
		Message: "mark job complete successful",
		JobName: name,
	})
}

// MarkCompleteByType is an http.Handlerfunc marks all jobs of the given type
// complete.
func (s *ManagementService) MarkCompleteByType(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	jobType := vars["type"]
	filter := vars["filter"]

	ctx := r.Context()
	if err := s.manager.CompleteJobsByType(ctx, management.StatusFilter(filter), jobType); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeTextErrorResponder(errors.Wrapf(err,
			"problem completing jobs by type '%s'", jobType)))
	}

	gimlet.WriteJSON(rw, struct {
		Message string `json:"message"`
		JobType string `json:"job_type"`
	}{
		Message: "mark jobs complete by type successful",
		JobType: jobType,
	})
}

// MarkManyComplete is an http.Handlerfunc marks all jobs of the
// specified status complete.
func (s *ManagementService) MarkManyComplete(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	filter := vars["filter"]

	ctx := r.Context()
	if err := s.manager.CompleteJobs(ctx, management.StatusFilter(filter)); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeTextErrorResponder(errors.Wrapf(err,
			"problem completing jobs with filter '%s'", filter)))
	}

	gimlet.WriteJSON(rw, struct {
		Message string `json:"message"`
	}{
		Message: "mark jobs complete by type successful",
	})
}
