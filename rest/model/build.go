package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/pkg/errors"
)

// APIBuild is the model to be returned by the API whenever builds are fetched.
type APIBuild struct {
	Id                  APIString     `json:"_id"`
	ProjectId           APIString     `json:"project_id"`
	CreateTime          APITime       `json:"create_time"`
	StartTime           APITime       `json:"start_time"`
	FinishTime          APITime       `json:"finish_time"`
	PushTime            APITime       `json:"push_time"`
	Version             APIString     `json:"version"`
	Branch              APIString     `json:"branch"`
	Revision            APIString     `json:"gitspec"`
	BuildVariant        APIString     `json:"build_variant"`
	BuildNumber         APIString     `json:"build_number"`
	Status              APIString     `json:"status"`
	Activated           bool          `json:"activated"`
	ActivatedBy         APIString     `json:"activated_by"`
	ActivatedTime       APITime       `json:"activated_time"`
	RevisionOrderNumber int           `json:"order"`
	Tasks               []string      `json:"tasks"`
	TimeTaken           time.Duration `json:"time_taken_ms"`
	DisplayName         APIString     `json:"display_name"`
	PredictedMakespan   time.Duration `json:"predicted_makespan_ms"`
	ActualMakespan      time.Duration `json:"actual_makespan_ms"`
	Requester           APIString     `json:"r"`
}

// BuildFromService converts from service level structs to an APIBuild.
// APIBuild.ProjectId is set in the route builder's Execute method.
func (apiBuild *APIBuild) BuildFromService(h interface{}) error {
	v, ok := h.(build.Build)
	if !ok {
		return fmt.Errorf("incorrect type when fetching converting build type")
	}
	apiBuild.Id = APIString(v.Id)
	apiBuild.CreateTime = NewTime(v.CreateTime)
	apiBuild.StartTime = NewTime(v.StartTime)
	apiBuild.FinishTime = NewTime(v.FinishTime)
	apiBuild.PushTime = NewTime(v.PushTime)
	apiBuild.Version = APIString(v.Version)
	apiBuild.Branch = APIString(v.Project)
	apiBuild.Revision = APIString(v.Revision)
	apiBuild.BuildVariant = APIString(v.BuildVariant)
	apiBuild.BuildNumber = APIString(v.BuildNumber)
	apiBuild.Status = APIString(v.Status)
	apiBuild.Activated = v.Activated
	apiBuild.ActivatedBy = APIString(v.ActivatedBy)
	apiBuild.ActivatedTime = NewTime(v.ActivatedTime)
	apiBuild.RevisionOrderNumber = v.RevisionOrderNumber
	for _, t := range v.Tasks {
		apiBuild.Tasks = append(apiBuild.Tasks, t.Id)
	}
	apiBuild.TimeTaken = v.TimeTaken
	apiBuild.DisplayName = APIString(v.DisplayName)
	apiBuild.PredictedMakespan = v.PredictedMakespan
	apiBuild.ActualMakespan = v.ActualMakespan
	apiBuild.Requester = APIString(v.Requester)
	return nil
}

// ToService returns a service layer build using the data from the APIBuild.
func (apiBuild *APIBuild) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
