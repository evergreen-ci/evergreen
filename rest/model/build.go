package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

var (
	commitOrigin  = "commit"
	patchOrigin   = "patch"
	triggerOrigin = "trigger"
	triggerAdHoc  = "ad_hoc"
	gitTagOrigin  = "git_tag"
)

// APIBuild is the model to be returned by the API whenever builds are fetched.
type APIBuild struct {
	Id                  *string        `json:"_id"`
	ProjectId           *string        `json:"project_id"`
	ProjectIdentifier   *string        `json:"project_identifier"`
	CreateTime          *time.Time     `json:"create_time"`
	StartTime           *time.Time     `json:"start_time"`
	FinishTime          *time.Time     `json:"finish_time"`
	Version             *string        `json:"version"`
	Revision            *string        `json:"git_hash"`
	BuildVariant        *string        `json:"build_variant"`
	Status              *string        `json:"status"`
	Activated           bool           `json:"activated"`
	ActivatedBy         *string        `json:"activated_by"`
	ActivatedTime       *time.Time     `json:"activated_time"`
	RevisionOrderNumber int            `json:"order"`
	TaskCache           []APITaskCache `json:"task_cache,omitempty"`
	// Tasks is the build's task cache with just the names
	Tasks             []string             `json:"tasks"`
	TimeTaken         APIDuration          `json:"time_taken_ms"`
	DisplayName       *string              `json:"display_name"`
	PredictedMakespan APIDuration          `json:"predicted_makespan_ms"`
	ActualMakespan    APIDuration          `json:"actual_makespan_ms"`
	Origin            *string              `json:"origin"`
	StatusCounts      task.TaskStatusCount `json:"status_counts,omitempty"`
}

// BuildFromService converts from service level structs to an APIBuild.
// APIBuild.ProjectId is set in the route builder's Execute method.
func (apiBuild *APIBuild) BuildFromService(h interface{}) error {
	v, ok := h.(build.Build)
	if !ok {
		return fmt.Errorf("incorrect type when fetching converting build type")
	}
	apiBuild.Id = utility.ToStringPtr(v.Id)
	apiBuild.CreateTime = ToTimePtr(v.CreateTime)
	apiBuild.StartTime = ToTimePtr(v.StartTime)
	apiBuild.FinishTime = ToTimePtr(v.FinishTime)
	apiBuild.Version = utility.ToStringPtr(v.Version)
	apiBuild.Revision = utility.ToStringPtr(v.Revision)
	apiBuild.BuildVariant = utility.ToStringPtr(v.BuildVariant)
	apiBuild.Status = utility.ToStringPtr(v.Status)
	apiBuild.Activated = v.Activated
	apiBuild.ActivatedBy = utility.ToStringPtr(v.ActivatedBy)
	apiBuild.ActivatedTime = ToTimePtr(v.ActivatedTime)
	apiBuild.RevisionOrderNumber = v.RevisionOrderNumber
	apiBuild.ProjectId = utility.ToStringPtr(v.Project)
	for _, t := range v.Tasks {
		apiBuild.Tasks = append(apiBuild.Tasks, t.Id)
	}
	apiBuild.TimeTaken = NewAPIDuration(v.TimeTaken)
	apiBuild.DisplayName = utility.ToStringPtr(v.DisplayName)
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
	case evergreen.GitTagRequester:
		origin = gitTagOrigin
	}
	apiBuild.Origin = utility.ToStringPtr(origin)
	if v.Project != "" {
		identifier, err := model.GetIdentifierForProject(v.Project)
		if err == nil {
			apiBuild.ProjectIdentifier = utility.ToStringPtr(identifier)
		}
	}
	return nil
}

func (apiBuild *APIBuild) SetTaskCache(tasks []task.Task) {
	taskMap := task.TaskSliceToMap(tasks)

	apiBuild.TaskCache = []APITaskCache{}
	for _, taskID := range apiBuild.Tasks {
		t, ok := taskMap[taskID]
		if !ok {
			continue
		}
		apiBuild.TaskCache = append(apiBuild.TaskCache, APITaskCache{
			Id:            t.Id,
			DisplayName:   t.DisplayName,
			Status:        t.Status,
			StatusDetails: t.Details,
			StartTime:     ToTimePtr(t.StartTime),
			TimeTaken:     t.TimeTaken,
			Activated:     t.Activated,
		})
		apiBuild.StatusCounts.IncrementStatus(t.Status, t.Details)
	}
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
	StartTime       *time.Time              `json:"start_time"`
	TimeTaken       time.Duration           `json:"time_taken"`
	Activated       bool                    `json:"activated"`
	FailedTestNames []string                `json:"failed_test_names,omitempty"`
}

type APIVariantTasks struct {
	Variant      *string
	Tasks        []string
	DisplayTasks []APIDisplayTask
}

func APIVariantTasksBuildFromService(v patch.VariantTasks) APIVariantTasks {
	out := APIVariantTasks{}
	out.Variant = &v.Variant
	out.Tasks = v.Tasks
	for _, e := range v.DisplayTasks {
		n := APIDisplayTaskBuildFromService(e)
		out.DisplayTasks = append(out.DisplayTasks, *n)
	}
	return out
}

func APIVariantTasksToService(v APIVariantTasks) patch.VariantTasks {
	out := patch.VariantTasks{}
	if v.Variant != nil {
		out.Variant = *v.Variant
	}
	out.Tasks = v.Tasks
	for _, e := range v.DisplayTasks {
		n := APIDisplayTaskToService(e)
		out.DisplayTasks = append(out.DisplayTasks, *n)
	}
	return out
}
