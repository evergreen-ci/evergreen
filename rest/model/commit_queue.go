package model

import (
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/pkg/errors"
)

type APICommitQueue struct {
	ProjectID APIString   `json:"queue_id"`
	Queue     []APIString `json:"queue"`
}

func (cq *APICommitQueue) BuildFromService(h interface{}) error {
	cqService, ok := h.(commitqueue.CommitQueue)
	if !ok {
		return errors.Errorf("incorrect type '%T' when converting commit queue", h)
	}

	cq.ProjectID = ToAPIString(cqService.ProjectID)
	for _, item := range cqService.Queue {
		cq.Queue = append(cq.Queue, ToAPIString(item))
	}

	return nil
}

func (cq *APICommitQueue) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
