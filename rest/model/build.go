package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

var (
	commitOrigin  = "commit"
	patchOrigin   = "patch"
	triggerOrigin = "trigger"
	triggerAdHoc  = "ad_hoc"
)

// APIBuild is the model to be returned by the API whenever builds are fetched.
type APIBuild struct {
	Id                  APIString      `json:"_id"`
	ProjectId           APIString      `json:"project_id"`
	CreateTime          APITime        `json:"create_time"`
	StartTime           APITime        `json:"start_time"`
	FinishTime          APITime        `json:"finish_time"`
	Version             APIString      `json:"version"`
	Branch              APIString      `json:"branch"`
	Revision            APIString      `json:"git_hash"`
	BuildVariant        APIString      `json:"build_variant"`
	Status              APIString      `json:"status"`
	Activated           bool           `json:"activated"`
	ActivatedBy         APIString      `json:"activated_by"`
	ActivatedTime       APITime        `json:"activated_time"`
	RevisionOrderNumber int            `json:"order"`
	TaskCache           []APITaskCache `json:"task_cache"`
	// Tasks is the build's task cache with just the names
	Tasks             []string             `json:"tasks"`
	TimeTaken         APIDuration          `json:"time_taken_ms"`
	DisplayName       APIString            `json:"display_name"`
	PredictedMakespan APIDuration          `json:"predicted_makespan_ms"`
	ActualMakespan    APIDuration          `json:"actual_makespan_ms"`
	Origin            APIString            `json:"origin"`
	StatusCounts      task.TaskStatusCount `json:"status_counts"`
}

// BuildFromService converts from service level structs to an APIBuild.
// APIBuild.ProjectId is set in the route builder's Execute method.
func (apiBuild *APIBuild) BuildFromService(h interface{}) error {
	v, ok := h.(build.Build)
	if !ok {
		return fmt.Errorf("incorrect type when fetching converting build type")
	}
	apiBuild.Id = ToAPIString(v.Id)
	apiBuild.CreateTime = NewTime(v.CreateTime)
	apiBuild.StartTime = NewTime(v.StartTime)
	apiBuild.FinishTime = NewTime(v.FinishTime)
	apiBuild.Version = ToAPIString(v.Version)
	apiBuild.Branch = ToAPIString(v.Branch)
	apiBuild.Revision = ToAPIString(v.Revision)
	apiBuild.BuildVariant = ToAPIString(v.BuildVariant)
	apiBuild.Status = ToAPIString(v.Status)
	apiBuild.Activated = v.Activated
	apiBuild.ActivatedBy = ToAPIString(v.ActivatedBy)
	apiBuild.ActivatedTime = NewTime(v.ActivatedTime)
	apiBuild.RevisionOrderNumber = v.RevisionOrderNumber
	for _, t := range v.Tasks {
		apiBuild.Tasks = append(apiBuild.Tasks, t.Id)
	}
	apiBuild.TimeTaken = NewAPIDuration(v.TimeTaken)
	apiBuild.DisplayName = ToAPIString(v.DisplayName)
	apiBuild.PredictedMakespan = NewAPIDuration(v.PredictedMakespan)
	apiBuild.ActualMakespan = NewAPIDuration(v.ActualMakespan)
	var origin string
	switch v.Requester {
	case evergreen.RepotrackerVersionRequester:
		origin = commitOrigin
	case evergreen.GithubPRRequester:
		origin = patchOrigin
	case evergreen.MergeTestRequester:
		origin = patchOrigin
	case evergreen.PatchVersionRequester:
		origin = patchOrigin
	case evergreen.TriggerRequester:
		origin = triggerOrigin
	case evergreen.AdHocRequester:
		origin = triggerAdHoc
	}
	apiBuild.Origin = ToAPIString(origin)
	apiBuild.TaskCache = []APITaskCache{}
	for _, t := range v.Tasks {
		apiBuild.TaskCache = append(apiBuild.TaskCache, APITaskCache{
			Id:            t.Id,
			DisplayName:   t.DisplayName,
			Status:        t.Status,
			StatusDetails: t.StatusDetails,
			StartTime:     t.StartTime,
			TimeTaken:     t.TimeTaken,
			Activated:     t.Activated,
		})
		apiBuild.StatusCounts.IncrementStatus(t.Status, t.StatusDetails)
	}
	return nil
}

// ToService returns a service layer build using the data from the APIBuild.
func (apiBuild *APIBuild) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}

type APITaskCache struct {
	Id              string                  `json:"id"`
	DisplayName     string                  `json:"display_name"`
	Status          string                  `json:"status"`
	StatusDetails   apimodels.TaskEndDetail `json:"task_end_details"`
	StartTime       time.Time               `json:"start_time"`
	TimeTaken       time.Duration           `json:"time_taken"`
	Activated       bool                    `json:"activated"`
	FailedTestNames []string                `json:"failed_test_names,omitempty"`
}
