package model

import (
	"regexp"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/pkg/errors"
)

type APICommitQueue struct {
	ProjectID APIString            `json:"queue_id"`
	Queue     []APICommitQueueItem `json:"queue"`
}

type APICommitQueueItem struct {
	Issue   APIString   `json:"issue"`
	Version APIString   `json:"version"`
	Modules []APIModule `json:"modules"`
}

type APIModule struct {
	Module APIString `json:"module"`
	Issue  APIString `json:"issue"`
}

type APICommitQueuePosition struct {
	Position int `json:"position"`
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
	item.Version = ToAPIString(cqItemService.Version)
	for _, module := range cqItemService.Modules {
		item.Modules = append(item.Modules, APIModule{
			Module: ToAPIString(module.Module),
			Issue:  ToAPIString(module.Issue),
		})
	}

	return nil
}

func (item *APICommitQueueItem) ToService() (interface{}, error) {
	serviceItem := commitqueue.CommitQueueItem{
		Issue:   FromAPIString(item.Issue),
		Version: FromAPIString(item.Version),
	}
	for _, module := range item.Modules {
		serviceModule := commitqueue.Module{
			Module: FromAPIString(module.Module),
			Issue:  FromAPIString(module.Issue),
		}
		serviceItem.Modules = append(serviceItem.Modules, serviceModule)
	}
	return serviceItem, nil
}

func ParseGitHubCommentModules(comment string) []APIModule {
	modules := []APIModule{}

	r := regexp.MustCompile(`(?:--module|-m)\s+(\w+):(\d+)`)
	moduleSlices := r.FindAllStringSubmatch(comment, -1)

	for _, moduleSlice := range moduleSlices {
		modules = append(modules, APIModule{
			Module: ToAPIString(moduleSlice[1]),
			Issue:  ToAPIString(moduleSlice[2]),
		})
	}

	return modules
}
