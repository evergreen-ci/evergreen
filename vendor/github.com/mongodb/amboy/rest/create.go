package rest

import (
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

func (s *Service) createJobResponseBase() *createResponse {
	return &createResponse{
		QueueDepth: s.queue.Stats().Pending,
		Status:     s.getStatus(),
	}
}

func (s *Service) createJob(payload *registry.JobInterchange) (*createResponse, error) {
	resp := s.createJobResponseBase()
	j, err := payload.Resolve(amboy.JSON)

	if err != nil {
		resp.Error = err.Error()
		return resp, err
	}

	resp.ID = j.ID()

	err = s.queue.Put(j)
	if err != nil {
		resp.Error = err.Error()
		return resp, err
	}

	resp.Registered = true
	return resp, nil
}

// Create provides an interface for REST clients to create jobs in the
// local queue that backs the service.
func (s *Service) Create(w http.ResponseWriter, r *http.Request) {
	jobPayload := &registry.JobInterchange{}
	resp := s.createJobResponseBase()

	err := gimlet.GetJSON(r.Body, jobPayload)
	if err != nil {
		resp.Error = err.Error()
		grip.Error(err)
		gimlet.WriteJSONError(w, resp)
		return
	}

	resp, err = s.createJob(jobPayload)
	if err != nil {
		resp.Error = err.Error()
		grip.Error(err)
		gimlet.WriteJSONError(w, resp)
		return
	}

	gimlet.WriteJSON(w, resp)
}
