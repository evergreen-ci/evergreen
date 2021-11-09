package rest

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
)

type createResponse struct {
	Registered bool   `bson:"registered" json:"registered" yaml:"registered"`
	QueueDepth int    `bson:"queue_depth,omitempty" json:"queue_depth,omitempty" yaml:"queue_depth,omitempty"`
	ID         string `bson:"id" json:"id" yaml:"id"`
	Error      string `bson:"error,omitempty" json:"error,omitempty" yaml:"error,omitempty"`
	Status     status `bson:"status,omitempty" json:"status,omitempty" yaml:"status,omitempty"`
}

func (s *QueueService) createJobResponseBase(ctx context.Context) *createResponse {
	return &createResponse{
		QueueDepth: s.queue.Stats(ctx).Pending,
		Status:     s.getStatus(ctx),
	}
}

func (s *QueueService) createJob(ctx context.Context, payload *registry.JobInterchange) (*createResponse, error) {
	resp := s.createJobResponseBase(ctx)
	j, err := payload.Resolve(amboy.JSON)

	if err != nil {
		resp.Error = err.Error()
		return resp, err
	}

	resp.ID = j.ID()

	err = s.queue.Put(ctx, j)
	if err != nil {
		resp.Error = err.Error()
		return resp, err
	}

	resp.Registered = true
	return resp, nil
}

// Create provides an interface for REST clients to create jobs in the
// local queue that backs the service.
func (s *QueueService) Create(w http.ResponseWriter, r *http.Request) {
	jobPayload := &registry.JobInterchange{}
	ctx := r.Context()
	resp := s.createJobResponseBase(ctx)

	err := gimlet.GetJSON(r.Body, jobPayload)
	if err != nil {
		resp.Error = err.Error()
		grip.Error(err)
		gimlet.WriteJSONError(w, resp)
		return
	}

	resp, err = s.createJob(ctx, jobPayload)
	if err != nil {
		resp.Error = err.Error()
		grip.Error(err)
		gimlet.WriteJSONError(w, resp)
		return
	}

	gimlet.WriteJSON(w, resp)
}
