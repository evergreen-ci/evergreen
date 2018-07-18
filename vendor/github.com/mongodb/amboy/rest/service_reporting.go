package rest

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy/reporting"
	"github.com/pkg/errors"
)

// ReportingService wraps a reporter instance as described in the
// reporting package and provides an HTTP interface for all of the
// methods provided by the reporter.
type ReportingService struct {
	reporter reporting.Reporter
}

// NewReportingService constructs a reporting service from the
// reporter provided.
func NewReportingService(r reporting.Reporter) *ReportingService {
	return &ReportingService{
		reporter: r,
	}
}

// App returns a gimlet application with all of the routes
// configured.
func (s *ReportingService) App() *gimlet.APIApp {
	app := gimlet.NewApp()

	app.AddRoute("/status/{filter}").Version(1).Get().Handler(s.GetJobStatus)
	app.AddRoute("/status/{filter}/{type}").Version(1).Get().Handler(s.GetJobStatusByType)
	app.AddRoute("/timing/{filter}/{seconds}").Version(1).Get().Handler(s.GetRecentTimings)
	app.AddRoute("/errors/{filter}/{seconds}").Version(1).Get().Handler(s.GetRecentErrors)
	app.AddRoute("/errors/{filter}/{type}/{seconds}").Version(1).Get().Handler(s.GetRecentErrorsByType)

	return app
}

// GetJobStatus is an http.HandlerFunc that provides access to counts
// of all jobs that match a defined filter.
func (s *ReportingService) GetJobStatus(rw http.ResponseWriter, r *http.Request) {
	filter := reporting.CounterFilter(gimlet.GetVars(r)["filter"])
	ctx := r.Context()

	err := filter.Validate()
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	data, err := s.reporter.JobStatus(ctx, filter)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	gimlet.WriteJSON(rw, data)
}

// GetJobStatusByType is an http.HandlerFunc that produces a list of job IDs for
// jobs that match a defined filter.
func (s *ReportingService) GetJobStatusByType(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	filter := reporting.CounterFilter(vars["filter"])
	jobType := vars["type"]

	if err := filter.Validate(); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	ctx := r.Context()
	data, err := s.reporter.JobIDsByState(ctx, jobType, filter)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	gimlet.WriteJSON(rw, data)
}

// GetRecentTimings is an http.HandlerFunc that produces a report that lists the average runtime
// (duration) or latency of jobs.
func (s *ReportingService) GetRecentTimings(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	dur, err := time.ParseDuration(vars["seconds"])
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrapf(err,
			"problem parsing duration from %s", vars["seconds"])))
		return
	}

	filter := reporting.RuntimeFilter(vars["filter"])
	if err = filter.Validate(); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	ctx := r.Context()
	data, err := s.reporter.RecentTiming(ctx, dur, filter)
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
func (s *ReportingService) GetRecentErrors(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)

	dur, err := time.ParseDuration(vars["seconds"])
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrapf(err,
			"problem parsing duration from %s", vars["seconds"])))
		return
	}

	filter := reporting.ErrorFilter(vars["filter"])
	if err = filter.Validate(); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	ctx := r.Context()
	data, err := s.reporter.RecentErrors(ctx, dur, filter)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	gimlet.WriteJSON(rw, data)
}

// GetRecentErrorsByType is an http.Handlerfunc returns an errors report for
// only a single type of jobs.
func (s *ReportingService) GetRecentErrorsByType(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	jobType := vars["type"]

	dur, err := time.ParseDuration(vars["seconds"])
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(errors.Wrapf(err,
			"problem parsing duration from %s", vars["seconds"])))
		return
	}

	filter := reporting.ErrorFilter(vars["filter"])
	if err = filter.Validate(); err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	ctx := r.Context()
	data, err := s.reporter.RecentJobErrors(ctx, jobType, dur, filter)
	if err != nil {
		gimlet.WriteResponse(rw, gimlet.MakeJSONErrorResponder(err))
		return
	}

	gimlet.WriteJSON(rw, data)
}
