package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type jobStatusResponse struct {
	Exists      bool        `bson:"job_exists" json:"job_exists" yaml:"job_exists"`
	Completed   bool        `bson:"completed" json:"completed" yaml:"completed"`
	ID          string      `bson:"id,omitempty" json:"id,omitempty" yaml:"id,omitempty"`
	JobsPending int         `bson:"jobs_pending,omitempty" json:"jobs_pending,omitempty" yaml:"jobs_pending,omitempty"`
	Error       string      `bson:"error,omitempty" json:"error,omitempty" yaml:"error,omitempty"`
	Job         interface{} `bson:"job,omitempty" json:"job,omitempty" yaml:"job,omitempty"`
}

func (s *Service) getJobStatusResponse(name string) (*jobStatusResponse, error) {
	var msg string
	var err error

	resp := &jobStatusResponse{}
	resp.JobsPending = s.queue.Stats().Pending
	resp.ID = name

	if name == "" {
		msg = fmt.Sprintf("did not specify job name: %s", name)
		err = errors.New(msg)
		resp.Error = msg

		return resp, err
	}

	j, exists := s.queue.Get(name)
	resp.Exists = exists

	if !exists {
		msg = fmt.Sprintf("could not recover job '%s'", name)
		err = errors.New(msg)
		resp.Error = msg

		return resp, err
	}

	resp.Exists = true
	resp.Completed = j.Status().Completed
	resp.Job = j

	return resp, nil
}

// JobStatus is a http.HandlerFunc that writes a job status document to the request.
func (s *Service) JobStatus(w http.ResponseWriter, r *http.Request) {
	name := gimlet.GetVars(r)["name"]

	response, err := s.getJobStatusResponse(name)
	if err != nil {
		grip.Error(err)
		gimlet.WriteJSONError(w, response)
		return
	}

	gimlet.WriteJSON(w, response)
}

// WaitJob waits for a single job to be complete. It takes a timeout
// argument, which defaults to 10 seconds, and returns 408 (request
// timeout) if the timeout is reached before the job completes.
func (s *Service) WaitJob(w http.ResponseWriter, r *http.Request) {
	name := gimlet.GetVars(r)["name"]
	response, err := s.getJobStatusResponse(name)
	if err != nil {
		grip.Error(err)
		gimlet.WriteJSONError(w, response)
	}

	timeout, err := parseTimeout(r)
	if err != nil {
		grip.Infof("problem parsing timeout for name %s: %v", name, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	response, code, err := s.waitForJob(ctx, name)
	grip.Error(err)
	gimlet.WriteJSONResponse(w, code, response)
}

func parseTimeout(r *http.Request) (time.Duration, error) {
	var err error

	timeout := 10 * time.Second

	timeoutInput, ok := r.URL.Query()["timeout"]

	if ok || len(timeoutInput) != 0 {
		timeout, err = time.ParseDuration(timeoutInput[0])
		if err != nil {
			timeout = 10 * time.Second
		}
	}

	return timeout, errors.Wrapf(err, "problem parsing timeout from %s", timeoutInput)
}

func (s *Service) waitForJob(ctx context.Context, name string) (*jobStatusResponse, int, error) {
	job, ok := s.queue.Get(name)
	if !ok {
		response, err := s.getJobStatusResponse(name)
		grip.Error(err)
		return response, http.StatusNotFound, errors.Errorf(
			"problem finding job: %s", name)
	}

	ok = amboy.WaitJobCtxInterval(ctx, job, s.queue, 500*time.Millisecond)

	response, err := s.getJobStatusResponse(name)
	if err != nil {
		return response, http.StatusInternalServerError, errors.Wrapf(err,
			"problem constructing response for while waiting for job %s", name)
	}

	if !ok {
		return response, http.StatusRequestTimeout, errors.Errorf(
			"reached timeout waiting for job: %s", name)
	}

	return response, http.StatusOK, nil
}
