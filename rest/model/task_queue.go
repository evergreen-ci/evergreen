package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
)

type APITaskQueueItem struct {
	Id                  *string     `json:"id"`
	DisplayName         *string     `json:"display_name"`
	BuildVariant        *string     `json:"build_variant"`
	RevisionOrderNumber int         `json:"order"`
	Requester           *string     `json:"requester"`
	Revision            *string     `json:"gitspec"`
	Project             *string     `json:"project"`
	ProjectIdentifier   *string     `json:"project_identifier"`
	Version             *string     `json:"version"`
	Build               *string     `json:"build"`
	ExpectedDuration    APIDuration `json:"exp_dur"`
	Priority            int64       `json:"priority"`
	ActivatedBy         *string     `json:"activated_by"`
}

func (s *APITaskQueueItem) BuildFromService(tqi model.TaskQueueItem) {
	s.Id = utility.ToStringPtr(tqi.Id)
	s.DisplayName = utility.ToStringPtr(tqi.DisplayName)
	s.BuildVariant = utility.ToStringPtr(tqi.BuildVariant)
	s.RevisionOrderNumber = tqi.RevisionOrderNumber
	s.Requester = utility.ToStringPtr(tqi.Requester)
	s.Revision = utility.ToStringPtr(tqi.Revision)
	s.Project = utility.ToStringPtr(tqi.Project)
	s.Version = utility.ToStringPtr(tqi.Version)
	s.Build = utility.ToStringPtr(tqi.BuildVariant)
	s.ExpectedDuration = NewAPIDuration(tqi.ExpectedDuration)
	s.Priority = tqi.Priority
	s.ActivatedBy = utility.ToStringPtr(tqi.ActivatedBy)
}
