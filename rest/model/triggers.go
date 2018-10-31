package model

import (
	"errors"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
)

type UpstreamMetadata struct {
	Build   APIBuild   `json:"build"`
	Task    *APITask   `json:"task,omitempty"`
	EventID APIString  `json:"event_id"`
	Project APIProject `json:"project"`
}

func (m *UpstreamMetadata) BuildFromService(h interface{}) error {
	return errors.New("BuildFromService not implemented")
}

func (m *UpstreamMetadata) ToService() (interface{}, error) {
	metadata := &model.UpstreamMetadata{
		EventID: FromAPIString(m.EventID),
	}
	b, err := m.Build.ToService()
	if err != nil {
		return nil, err
	}
	metadata.Build = b.(build.Build)
	var t interface{}
	if m.Task != nil {
		t, err = m.Task.ToService()
		if err != nil {
			return nil, err
		}
		task := t.(task.Task)
		metadata.Task = &task
	}
	p, err := m.Project.ToService()
	if err != nil {
		return nil, err
	}
	metadata.Project = p.(model.ProjectRef)

	return metadata, nil
}
