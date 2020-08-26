package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
)

type APITaskQueueItem struct {
	Id                  *string     `json:"id"`
	DisplayName         *string     `json:"display_name"`
	BuildVariant        *string     `json:"build_variant"`
	RevisionOrderNumber int         `json:"order"`
	Requester           *string     `json:"requester"`
	Revision            *string     `json:"gitspec"`
	Project             *string     `json:"project"`
	Version             *string     `json:"version"`
	Build               *string     `json:"build"`
	ExpectedDuration    APIDuration `json:"exp_dur"`
	Priority            int64       `json:"priority"`
}

func (s *APITaskQueueItem) BuildFromService(h interface{}) error {
	tqi, ok := h.(model.TaskQueueItem)
	if !ok {
		return errors.New("interface is not of type TaskQueueItem")
	}

	s.Id = ToStringPtr(tqi.Id)
	s.DisplayName = ToStringPtr(tqi.DisplayName)
	s.BuildVariant = ToStringPtr(tqi.BuildVariant)
	s.RevisionOrderNumber = tqi.RevisionOrderNumber
	s.Requester = ToStringPtr(tqi.Requester)
	s.Revision = ToStringPtr(tqi.Revision)
	s.Project = ToStringPtr(tqi.Project)
	s.Version = ToStringPtr(tqi.Version)
	s.Build = ToStringPtr(tqi.BuildVariant)
	s.ExpectedDuration = NewAPIDuration(tqi.ExpectedDuration)
	s.Priority = tqi.Priority

	return nil
}
