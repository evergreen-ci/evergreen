package model

import (
	"regexp"
	"time"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/utility"
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
	PatchId         *string     `json:"patch_id"`
	Version         *string     `json:"version"`
	EnqueueTime     *time.Time  `json:"enqueueTime"`
	Modules         []APIModule `json:"modules"`
	Patch           *APIPatch   `json:"patch"`
	MessageOverride *string     `json:"message_override"`
	Source          *string     `json:"source"`
}

type APICommitQueuePosition struct {
	Position int `json:"position"`
}

type APICommitQueueItemAuthor struct {
	Author *string `json:"author"`
}

func (cq *APICommitQueue) BuildFromService(cqService commitqueue.CommitQueue) {
	cq.ProjectID = utility.ToStringPtr(cqService.ProjectID)
	for _, item := range cqService.Queue {
		cqItem := APICommitQueueItem{}
		cqItem.BuildFromService(item)
		cq.Queue = append(cq.Queue, cqItem)
	}
}

func (item *APICommitQueueItem) BuildFromService(cqItemService commitqueue.CommitQueueItem) {
	item.Issue = utility.ToStringPtr(cqItemService.Issue)
	item.Version = utility.ToStringPtr(cqItemService.Version)
	item.EnqueueTime = ToTimePtr(cqItemService.EnqueueTime)
	item.MessageOverride = utility.ToStringPtr(cqItemService.MessageOverride)
	item.Source = utility.ToStringPtr(cqItemService.Source)
	item.PatchId = utility.ToStringPtr(cqItemService.PatchId)

	for _, module := range cqItemService.Modules {
		item.Modules = append(item.Modules, *APIModuleBuildFromService(module))
	}
}

func (item *APICommitQueueItem) ToService() commitqueue.CommitQueueItem {
	serviceItem := commitqueue.CommitQueueItem{
		Issue:           utility.FromStringPtr(item.Issue),
		Version:         utility.FromStringPtr(item.Version),
		MessageOverride: utility.FromStringPtr(item.MessageOverride),
		Source:          utility.FromStringPtr(item.Source),
		PatchId:         utility.FromStringPtr(item.PatchId),
	}
	for _, module := range item.Modules {
		serviceItem.Modules = append(serviceItem.Modules, *APIModuleToService(module))
	}
	return serviceItem
}

type GithubCommentCqData struct {
	Modules         []APIModule
	MessageOverride string
}

func ParseGitHubComment(comment string) GithubCommentCqData {
	data := GithubCommentCqData{}

	lineRegex := regexp.MustCompile(`(\A.*)(?:\n*)([\S\s]*)`)
	lines := lineRegex.FindAllStringSubmatch(comment, -1)
	if len(lines) == 0 {
		return data
	}
	for index, line := range lines[0] {
		if index == 1 {
			data = parseFirstLine(line)
		} else if index == 2 {
			data.MessageOverride = line
		}
	}

	return data
}

func parseFirstLine(comment string) GithubCommentCqData {
	modules := []APIModule{}

	moduleRegex := regexp.MustCompile(`(?:--module|-m)\s+(\w+):(\d+)`)
	moduleSlices := moduleRegex.FindAllStringSubmatch(comment, -1)
	for _, moduleSlice := range moduleSlices {
		modules = append(modules, APIModule{
			Module: utility.ToStringPtr(moduleSlice[1]),
			Issue:  utility.ToStringPtr(moduleSlice[2]),
		})
	}

	data := GithubCommentCqData{}
	data.Modules = modules
	return data
}
