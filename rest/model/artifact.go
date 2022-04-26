package model

import (
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type APIFile struct {
	Name           *string `json:"name"`
	Link           *string `json:"url"`
	Visibility     *string `json:"visibility"`
	IgnoreForFetch bool    `json:"ignore_for_fetch"`
}

type APIEntry struct {
	TaskId          *string   `json:"task_id"`
	TaskDisplayName *string   `json:"task_name"`
	BuildId         *string   `json:"build"`
	Files           []APIFile `json:"files"`
	Execution       int       `json:"execution"`
}

func (f *APIFile) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case artifact.File:
		f.Name = utility.ToStringPtr(v.Name)
		f.Link = utility.ToStringPtr(v.Link)
		f.Visibility = utility.ToStringPtr(v.Visibility)
		f.IgnoreForFetch = v.IgnoreForFetch
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (f *APIFile) ToService() (interface{}, error) {
	return artifact.File{
		Name:           utility.FromStringPtr(f.Name),
		Link:           utility.FromStringPtr(f.Link),
		Visibility:     utility.FromStringPtr(f.Visibility),
		IgnoreForFetch: f.IgnoreForFetch,
	}, nil
}

func (e *APIEntry) BuildFromService(h interface{}) error {
	catcher := grip.NewBasicCatcher()
	switch v := h.(type) {
	case artifact.Entry:
		e.TaskId = utility.ToStringPtr(v.TaskId)
		e.TaskDisplayName = utility.ToStringPtr(v.TaskDisplayName)
		e.BuildId = utility.ToStringPtr(v.BuildId)
		e.Execution = v.Execution
		for _, file := range v.Files {
			apiFile := APIFile{}
			catcher.Add(apiFile.BuildFromService(file))
			e.Files = append(e.Files, apiFile)
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return catcher.Resolve()
}

func (e *APIEntry) ToService() (interface{}, error) {
	entry := artifact.Entry{
		TaskId:          utility.FromStringPtr(e.TaskId),
		TaskDisplayName: utility.FromStringPtr(e.TaskDisplayName),
		BuildId:         utility.FromStringPtr(e.BuildId),
		Execution:       e.Execution,
	}
	catcher := grip.NewBasicCatcher()
	for _, apiFile := range e.Files {
		f, err := apiFile.ToService()
		if err != nil {
			catcher.Add(err)
		}
		file, ok := f.(artifact.File)
		if !ok {
			catcher.Add(errors.New("unable to convert artifact file"))
			continue
		}
		entry.Files = append(entry.Files, file)
	}
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return entry, nil
}
