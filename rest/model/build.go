package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
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
	Id *string `json:"_id"`
	// The identifier of the project this build represents
	ProjectId         *string `json:"project_id"`
	ProjectIdentifier *string `json:"project_identifier"`
	// Time at which build was created
	CreateTime *time.Time `json:"create_time"`
	// Time at which build started running tasks
	StartTime *time.Time `json:"start_time"`
	// Time at which build finished running all tasks
	FinishTime *time.Time `json:"finish_time"`
	// The version this build is running tasks for
	Version *string `json:"version"`
	// Hash of the revision on which this build is running
	Revision *string `json:"git_hash"`
	// Build distro and architecture information
	BuildVariant *string `json:"build_variant"`
	// The status of the build (possible values are "created", "started", "success", or "failed")
	Status *string `json:"status"`
	// Whether this build was manually initiated
	Activated bool `json:"activated"`
	// Who initiated the build
	ActivatedBy *string `json:"activated_by"`
	// When the build was initiated
	ActivatedTime *time.Time `json:"activated_time"`
	// Incrementing counter of project's builds
	RevisionOrderNumber int `json:"order"`
	// Contains a subset of information about tasks for the build; this is not
	// provided/accurate for most routes (get versions for project is an
	// exception).
	TaskCache []APITaskCache `json:"task_cache,omitempty"`
	// Tasks is the build's task cache with just the names
	Tasks []string `json:"tasks"`
	// List of tags defined for the build variant, if any
	Tags []*string `json:"tags,omitempty"`
	// How long the build took to complete all tasks
	TimeTaken APIDuration `json:"time_taken_ms"`
	// Displayed title of the build showing version and variant running
	DisplayName *string `json:"display_name"`
	// Predicted makespan by the scheduler prior to execution
	PredictedMakespan APIDuration `json:"predicted_makespan_ms"`
	// Actual makespan measured during execution
	ActualMakespan APIDuration `json:"actual_makespan_ms"`
	// The source of the patch, a commit or a patch
	Origin *string `json:"origin"`
	// Contains aggregated data about the statuses of tasks in this build. The
	// keys of this object are statuses and the values are the number of tasks
	// within this build in that status. Note that this field provides data that
	// you can get yourself by querying tasks for this build.
	StatusCounts task.TaskStatusCount `json:"status_counts,omitempty"`
	// Some routes will return information about the variant as defined in the
	// project. Does not expand expansions; they will be returned as written in
	// the project yaml (i.e. ${syntax})
	DefinitionInfo DefinitionInfo `json:"definition_info"`
}

type DefinitionInfo struct {
	// The cron defined for the variant, if provided, as defined in the project settings
	CronBatchTime *string `json:"cron,omitempty"`
	// The batchtime defined for the variant, if provided, as defined in the project settings
	BatchTime *int `json:"batchtime,omitempty"`
}

// PopulateDefinitionInfo adds cron/batchtime for the variant, if applicable. Not supported for matrices.
// Batchtime and cron are nil if not explicitly configured in the project config. Expansions are not expanded.
func (apiBuild *APIBuild) populateDefinitionInfo(pp *model.ParserProject) {
	if pp == nil {
		return
	}
	variant := utility.FromStringPtr(apiBuild.BuildVariant)
	for _, bv := range pp.BuildVariants {
		if bv.Name == variant {
			if bv.CronBatchTime != "" {
				apiBuild.DefinitionInfo.CronBatchTime = utility.ToStringPtr(bv.CronBatchTime)
			}
			apiBuild.DefinitionInfo.BatchTime = bv.BatchTime
		}
	}
}

// BuildFromService converts from service level structs to an APIBuild.
// APIBuild.ProjectId is set in the route builder's Execute method.
// If ParserProject isn't nil, include info from project (not applicable to matrix builds).
func (apiBuild *APIBuild) BuildFromService(v build.Build, pp *model.ParserProject) {
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
	apiBuild.Tags = utility.ToStringPtrSlice(v.Tags)
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
	apiBuild.populateDefinitionInfo(pp)
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
			TimeTakenMS:   APIDuration(t.TimeTaken.Milliseconds()),
			Activated:     t.Activated,
		})
		apiBuild.StatusCounts.IncrementStatus(t.Status, t.Details)
	}
}

type APITaskCache struct {
	Id              string                  `json:"id"`
	DisplayName     string                  `json:"display_name"`
	Status          string                  `json:"status"`
	StatusDetails   apimodels.TaskEndDetail `json:"task_end_details"`
	StartTime       *time.Time              `json:"start_time"`
	TimeTaken       time.Duration           `json:"time_taken" swaggertype:"primitive,integer"`
	TimeTakenMS     APIDuration             `json:"time_taken_ms"`
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
