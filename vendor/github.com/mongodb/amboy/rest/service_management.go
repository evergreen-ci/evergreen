package rest

import (
	"net/http"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy/management"
	"github.com/pkg/errors"
)

// ManagementService wraps a manager instance as described in the management
// package and provides an HTTP interface for all of the methods provided by
// methods provided by the manager.
type ManagementService struct {
	manager management.Manager
}

// NewManagementService constructs a management service from the manager
// provided.
func NewManagementService(m management.Manager) *ManagementService {
	return &ManagementService{
		manager: m,
	}
}

// App returns a gimlet application with all of the routes
// configured.
func (s *ManagementService) App() *gimlet.APIApp {
	app := gimlet.NewApp()

	app.AddRoute("/status/{filter}").Version(1).Get().Handler(s.GetJobStatus)
	app.AddRoute("/id/status/{filter}/type/{type}").Version(1).Get().Handler(s.GetJobIDs)
	app.AddRoute("/jobs/mark_complete/id/{name}").Version(1).Post().Handler(s.MarkComplete)
	app.AddRoute("/jobs/mark_complete/status/{filter}").Version(1).Post().Handler(s.MarkManyComplete)
	app.AddRoute("/jobs/mark_complete/status/{filter}/type/{type}").Version(1).Post().Handler(s.MarkCompleteByType)
	app.AddRoute("/jobs/mark_complete/status/{filter}/pattern/{pattern}").Version(1).Post().Handler(s.MarkCompleteByPattern)

	return app
}

// GetJobStatus is an http.HandlerFunc that counts all jobs that match a status
// filter.
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
		gimlet.WriteResponse(rw, gimlet.MakeJSONInternalErrorResponder(err))
		return
	}

	gimlet.WriteJSON(rw, data)
}

// GetJobIDs is an http.HandlerFunc that produces a list of job IDs for jobs
// that match a status filter and job type.
func (s *ManagementService) GetJobIDs(rw http.ResponseWriter, r *http.Request) {
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
		gimlet.WriteResponse(rw, gimlet.MakeJSONInternalErrorResponder(err))
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
		gimlet.WriteResponse(rw, gimlet.MakeTextInternalErrorResponder(errors.Wrapf(err, "completing job '%s'", name)))
		return
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
		gimlet.WriteResponse(rw, gimlet.MakeTextInternalErrorResponder(errors.Wrapf(err, "completing jobs by type '%s'", jobType)))
		return
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
		gimlet.WriteResponse(rw, gimlet.MakeTextErrorResponder(errors.Wrapf(err, "completing jobs with filter '%s'", filter)))
		return
	}

	gimlet.WriteJSON(rw, struct {
		Message string `json:"message"`
	}{
		Message: "mark jobs complete by filter successful",
	})
}

// MarkCompleteByPattern is an http.Handlerfunc marks all jobs with the
// specified pattern and status complete.
func (s *ManagementService) MarkCompleteByPattern(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	pattern := vars["pattern"]
	filter := vars["filter"]

	ctx := r.Context()
	if err := s.manager.CompleteJobsByPattern(ctx, management.StatusFilter(filter), pattern); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeTextInternalErrorResponder(errors.Wrapf(err, "completing jobs by pattern '%s' with filter '%s'", pattern, filter)))
		return
	}

	gimlet.WriteJSON(rw, struct {
		Message string `json:"message"`
	}{
		Message: "mark jobs complete by pattern successful",
	})
}
