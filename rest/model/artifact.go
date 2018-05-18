package model

import (
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/pkg/errors"
)

type APIFile struct {
	Name           APIString `json:"name"`
	Link           APIString `json:"url"`
	Visibility     APIString `json:"visibility"`
	IgnoreForFetch bool      `json:"ignore_for_fetch"`
}

type APIEntry struct {
	TaskId          APIString `json:"task"`
	TaskDisplayName APIString `json:"task_name"`
	BuildId         APIString `json:"build"`
	Files           []APIFile `json:"files"`
	Execution       int       `json:"execution"`
}

func (f *APIFile) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case artifact.File:
		f.Name = ToAPIString(v.Name)
		f.Link = ToAPIString(v.Link)
		f.Visibility = ToAPIString(v.Visibility)
		f.IgnoreForFetch = v.IgnoreForFetch
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (f *APIFile) ToService() (interface{}, error) {
	return artifact.File{
		Name:           FromAPIString(f.Name),
		Link:           FromAPIString(f.Link),
		Visibility:     FromAPIString(f.Visibility),
		IgnoreForFetch: f.IgnoreForFetch,
	}, nil
}

func (e *APIEntry) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case artifact.Entry:
		e.TaskId = ToAPIString(v.TaskId)
		e.TaskDisplayName = ToAPIString(v.TaskDisplayName)
		e.BuildId = ToAPIString(v.BuildId)
		e.Execution = v.Execution
		for _, file := range v.Files {
			apiFile := APIFile{}
			err := apiFile.BuildFromService(file)
			if err != nil {
				return err
			}
			e.Files = append(e.Files, apiFile)
		}
	default:
		return errors.Errorf("%T is not a supported type", h)
	}
	return nil
}

func (e *APIEntry) ToService() (interface{}, error) {
	entry := artifact.Entry{
		TaskId:          FromAPIString(e.TaskId),
		TaskDisplayName: FromAPIString(e.TaskDisplayName),
		BuildId:         FromAPIString(e.BuildId),
		Execution:       e.Execution,
	}
	for _, apiFile := range e.Files {
		f, err := apiFile.ToService()
		if err != nil {
			return nil, err
		}
		file := f.(artifact.File)
		entry.Files = append(entry.Files, file)
	}

	return entry, nil
}
