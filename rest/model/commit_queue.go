package model

import (
	"regexp"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/pkg/errors"
)

type APICommitQueue struct {
	ProjectID *string              `json:"queue_id"`
	Queue     []APICommitQueueItem `json:"queue"`
}

type APICommitQueueItem struct {
	Issue   *string     `json:"issue"`
	Version *string     `json:"version"`
	Modules []APIModule `json:"modules"`
}

type APIModule struct {
	Module *string `json:"module"`
	Issue  *string `json:"issue"`
}

type APICommitQueuePosition struct {
	Position int `json:"position"`
}

type APICommitQueueItemAuthor struct {
	Author *string `json:"author"`
}

func (cq *APICommitQueue) BuildFromService(h interface{}) error {
	cqService, ok := h.(commitqueue.CommitQueue)
	if !ok {
		return errors.Errorf("incorrect type '%T' when converting commit queue", h)
	}

	cq.ProjectID = ToStringPtr(cqService.ProjectID)
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
	item.Issue = ToStringPtr(cqItemService.Issue)
	item.Version = ToStringPtr(cqItemService.Version)
	for _, module := range cqItemService.Modules {
		item.Modules = append(item.Modules, APIModule{
			Module: ToStringPtr(module.Module),
			Issue:  ToStringPtr(module.Issue),
		})
	}

	return nil
}

func (item *APICommitQueueItem) ToService() (interface{}, error) {
	serviceItem := commitqueue.CommitQueueItem{
		Issue:   FromStringPtr(item.Issue),
		Version: FromStringPtr(item.Version),
	}
	for _, module := range item.Modules {
		serviceModule := commitqueue.Module{
			Module: FromStringPtr(module.Module),
			Issue:  FromStringPtr(module.Issue),
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
			Module: ToStringPtr(moduleSlice[1]),
			Issue:  ToStringPtr(moduleSlice[2]),
		})
	}

	return modules
}
