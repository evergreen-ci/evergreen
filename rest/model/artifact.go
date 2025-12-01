package model

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/utility"
)

type APIFile struct {
	// Human-readable name of the file
	Name *string `json:"name"`
	// Link to the file
	Link       *string `json:"url"`
	URLParsley *string `json:"url_parsley"`
	// Determines who can see the file in the UI
	Visibility *string `json:"visibility"`
	// When true, these artifacts are excluded from reproduction
	IgnoreForFetch bool    `json:"ignore_for_fetch"`
	ContentType    *string `json:"content_type"`
}

type APIEntry struct {
	TaskId          *string   `json:"task_id"`
	TaskDisplayName *string   `json:"task_name"`
	BuildId         *string   `json:"build"`
	Files           []APIFile `json:"files"`
	Execution       int       `json:"execution"`
}

func (f *APIFile) BuildFromService(file artifact.File) {
	f.ContentType = utility.ToStringPtr(file.ContentType)
	f.Name = utility.ToStringPtr(file.Name)
	f.Link = utility.ToStringPtr(file.Link)
	f.Visibility = utility.ToStringPtr(file.Visibility)
	f.IgnoreForFetch = file.IgnoreForFetch

}

func (f *APIFile) GetLogURL(env evergreen.Environment, taskID string, execution int) {
	settings := env.Settings()

	contentType := utility.FromStringPtr(f.ContentType)
	if contentType == "" {
		return
	}
	hasContentType := false
	for _, fileStreamingContentType := range settings.Ui.FileStreamingContentTypes {
		if strings.HasPrefix(contentType, fileStreamingContentType) {
			hasContentType = true
			break
		}
	}
	if hasContentType {
		fileName := utility.FromStringPtr(f.Name)
		if fileName == "" {
			return
		}
		f.URLParsley = utility.ToStringPtr(fmt.Sprintf("%s/taskFile/%s/%d/%s", settings.Ui.ParsleyUrl, taskID, execution, url.PathEscape(fileName)))
	}
}

func (f *APIFile) ToService() artifact.File {
	return artifact.File{
		ContentType:    utility.FromStringPtr(f.ContentType),
		Name:           utility.FromStringPtr(f.Name),
		Link:           utility.FromStringPtr(f.Link),
		Visibility:     utility.FromStringPtr(f.Visibility),
		IgnoreForFetch: f.IgnoreForFetch,
	}
}

func (e *APIEntry) BuildFromService(v artifact.Entry) {
	e.TaskId = utility.ToStringPtr(v.TaskId)
	e.TaskDisplayName = utility.ToStringPtr(v.TaskDisplayName)
	e.BuildId = utility.ToStringPtr(v.BuildId)
	e.Execution = v.Execution
	for _, file := range v.Files {
		apiFile := APIFile{}
		apiFile.BuildFromService(file)
		e.Files = append(e.Files, apiFile)
	}
}

func (e *APIEntry) ToService() artifact.Entry {
	entry := artifact.Entry{
		TaskId:          utility.FromStringPtr(e.TaskId),
		TaskDisplayName: utility.FromStringPtr(e.TaskDisplayName),
		BuildId:         utility.FromStringPtr(e.BuildId),
		Execution:       e.Execution,
	}
	for _, apiFile := range e.Files {
		entry.Files = append(entry.Files, apiFile.ToService())
	}

	return entry
}
