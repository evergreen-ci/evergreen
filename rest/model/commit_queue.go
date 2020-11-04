package model

import (
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/pkg/errors"
)

type APICommitQueue struct {
	ProjectID *string              `json:"queue_id"`
	Message   *string              `json:"message"` // note: this field is not populated by the conversion methods
	Owner     *string              `json:"owner"`
	Repo      *string              `json:"repo"`
	Queue     []APICommitQueueItem `json:"queue"`
}

type APICommitQueueItem struct {
	Issue           *string     `json:"issue"`
	Version         *string     `json:"version"`
	EnqueueTime     *time.Time  `json:"enqueueTime"`
	Modules         []APIModule `json:"modules"`
	Patch           *APIPatch   `json:"patch"`
	TitleOverride   *string     `json:"title_override"`
	MessageOverride *string     `json:"message_override"`
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
	item.EnqueueTime = ToTimePtr(cqItemService.EnqueueTime)
	item.TitleOverride = ToStringPtr(cqItemService.TitleOverride)
	item.MessageOverride = ToStringPtr(cqItemService.MessageOverride)

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
		Issue:           FromStringPtr(item.Issue),
		Version:         FromStringPtr(item.Version),
		TitleOverride:   FromStringPtr(item.TitleOverride),
		MessageOverride: FromStringPtr(item.MessageOverride),
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

type GithubCommentCqData struct {
	Modules         []APIModule
	TitleOverride   string
	MessageOverride string
}

func ParseGitHubComment(comment string) GithubCommentCqData {
	data := GithubCommentCqData{}
	comment = strings.TrimSpace(comment)
	modules := []APIModule{}

	moduleRegex := regexp.MustCompile(`(?:--module|-m)\s+(\w+):(\d+)`)
	moduleSlices := moduleRegex.FindAllStringSubmatch(comment, -1)
	for _, moduleSlice := range moduleSlices {
		modules = append(modules, APIModule{
			Module: ToStringPtr(moduleSlice[1]),
			Issue:  ToStringPtr(moduleSlice[2]),
		})
	}
	data.Modules = modules

	titleOverride := ""
	titleRegex := regexp.MustCompile(`(?:--title|-t)\s+\"([^\"]+)\"`)
	titleMatch := titleRegex.FindStringSubmatch(comment)
	if len(titleMatch) >= 2 {
		titleOverride = titleMatch[1]
	}
	data.TitleOverride = titleOverride

	messageOverride := ""
	messageRegex := regexp.MustCompile(`(?:--message|-m)\s+\"([^\"]+)\"`)
	messageMatch := messageRegex.FindStringSubmatch(comment)
	if len(messageMatch) >= 2 {
		messageOverride = messageMatch[1]
	}
	data.MessageOverride = messageOverride

	return data
}
