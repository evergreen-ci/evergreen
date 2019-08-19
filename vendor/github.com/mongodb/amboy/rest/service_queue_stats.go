package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
)

type status struct {
	Status            string   `bson:"string" json:"string" yaml:"string"`
	QueueRunning      bool     `bson:"queue_running" json:"queue_running" yaml:"queue_running"`
	PendingJobs       int      `bson:"pending_jobs,omitempty" json:"pending_jobs,omitempty" yaml:"pending_jobs,omitempty"`
	SupportedJobTypes []string `bson:"supported_job_types" json:"supported_job_types" yaml:"supported_job_types"`
}

func (s *QueueService) getStatus(ctx context.Context) status {
	output := status{
		SupportedJobTypes: s.registeredTypes,
	}

	if s.queue != nil && s.queue.Started() {
		output.Status = "ok"
		output.QueueRunning = true
		output.PendingJobs = s.queue.Stats(ctx).Pending
	} else {
		output.Status = "degraded"
	}

	return output
}

// Status defines an http.HandlerFunc that returns health check and
// current staus status information for the entire service.
func (s *QueueService) Status(w http.ResponseWriter, r *http.Request) {
	gimlet.WriteJSON(w, s.getStatus(r.Context()))
}

// WaitAll blocks waiting for all pending jobs in the queue to
// stop. Has a default timeout of 10 seconds, and returns 408 (request
// timeout) when the timeout succeeds.
func (s *QueueService) WaitAll(w http.ResponseWriter, r *http.Request) {
	timeout, err := parseTimeout(r)
	if err != nil {
		grip.Infof("problem parsing timeout for wait-all operation: %v", err)
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	ok := amboy.WaitInterval(ctx, s.queue, 100*time.Millisecond)
	st := s.getStatus(ctx)
	if !ok {
		gimlet.WriteJSONResponse(w, http.StatusRequestTimeout, st)
		return
	}

	gimlet.WriteJSON(w, st)
}
