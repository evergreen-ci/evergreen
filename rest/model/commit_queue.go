package model

import (
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/pkg/errors"
)

type APICommitQueue struct {
	ProjectID APIString            `json:"queue_id"`
	Queue     []APICommitQueueItem `json:"queue"`
}

type APICommitQueueItem struct {
	Issue   APIString   `json:"issue"`
	Modules []APIModule `json:"modules"`
}

type APIModule struct {
	Module APIString `json:"module"`
	Issue  APIString `json:"issue"`
}

func (cq *APICommitQueue) BuildFromService(h interface{}) error {
	cqService, ok := h.(commitqueue.CommitQueue)
	if !ok {
		return errors.Errorf("incorrect type '%T' when converting commit queue", h)
	}

	cq.ProjectID = ToAPIString(cqService.ProjectID)
	for _, item := range cqService.Queue {
		cqItem := APICommitQueueItem{}
		if err := cqItem.BuildFromService(item); err != nil {
			return errors.Wrap(err, "can't build API commit queue item from db model")
		}
		cq.Queue = append(cq.Queue, cqItem)
	}

	return nil
}

func (cq *APICommitQueue) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}

func (item *APICommitQueueItem) BuildFromService(h interface{}) error {
	cqItemService, ok := h.(commitqueue.CommitQueueItem)
	if !ok {
		return errors.Errorf("incorrect type '%T' when converting commit queue item", h)
	}
	item.Issue = ToAPIString(cqItemService.Issue)
	for _, module := range cqItemService.Modules {
		item.Modules = append(item.Modules, APIModule{
			Module: ToAPIString(module.Module),
			Issue:  ToAPIString(module.Issue),
		})
	}

	return nil
}

func (item *APICommitQueueItem) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
