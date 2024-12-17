package graphql

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
)

// AbortInfo is the resolver for the abortInfo field.
func (r *taskResolver) AbortInfo(ctx context.Context, obj *restModel.APITask) (*AbortInfo, error) {
	if !obj.Aborted {
		return nil, nil
	}

	info := AbortInfo{
		User:       obj.AbortInfo.User,
		TaskID:     obj.AbortInfo.TaskID,
		NewVersion: obj.AbortInfo.NewVersion,
		PrClosed:   obj.AbortInfo.PRClosed,
	}

	if len(obj.AbortInfo.TaskID) > 0 {
		abortedTask, err := task.FindOneId(obj.AbortInfo.TaskID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Problem getting aborted task %s: %s", *obj.Id, err.Error()))
		}
		if abortedTask == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find aborted task '%s'", obj.AbortInfo.TaskID))
		}
		abortedTaskBuild, err := build.FindOneId(abortedTask.BuildId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Problem getting build for aborted task %s: %s", abortedTask.BuildId, err.Error()))
		}
		if abortedTaskBuild == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find build '%s' for aborted task", abortedTask.BuildId))
		}
		info.TaskDisplayName = abortedTask.DisplayName
		info.BuildVariantDisplayName = abortedTaskBuild.DisplayName
	}
	return &info, nil
}

// Ami is the resolver for the ami field.
func (r *taskResolver) Ami(ctx context.Context, obj *restModel.APITask) (*string, error) {
	err := obj.GetAMI(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return obj.AMI, nil
}

// Annotation is the resolver for the annotation field.
func (r *taskResolver) Annotation(ctx context.Context, obj *restModel.APITask) (*restModel.APITaskAnnotation, error) {
	hasPermission, err := hasAnnotationPermission(ctx, obj, evergreen.AnnotationsView.Value)
	if err != nil {
		return nil, err
	}
	if !hasPermission {
		return nil, Forbidden.Send(ctx, "insufficient permission for viewing annotation")
	}

	annotation, err := annotations.FindOneByTaskIdAndExecution(*obj.Id, obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding annotation: %s", err.Error()))
	}
	if annotation == nil {
		return nil, nil
	}
	apiAnnotation := restModel.APITaskAnnotationBuildFromService(*annotation)
	return apiAnnotation, nil
}

// BaseStatus is the resolver for the baseStatus field.
func (r *taskResolver) BaseStatus(ctx context.Context, obj *restModel.APITask) (*string, error) {
	t, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting service model for APITask '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	baseStatus := t.BaseTask.Status
	if baseStatus == "" {
		return nil, nil
	}
	return &baseStatus, nil
}

// BaseTask is the resolver for the baseTask field.
func (r *taskResolver) BaseTask(ctx context.Context, obj *restModel.APITask) (*restModel.APITask, error) {
	t, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting service model for APITask '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}

	var baseTask *task.Task
	// BaseTask is sometimes added via aggregation when Task is resolved via GetTasksByVersion.
	if t.BaseTask.Id != "" {
		baseTask, err = task.FindOneId(t.BaseTask.Id)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s': %s", t.BaseTask.Id, err.Error()))
		}
		if baseTask == nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("task with id '%s' not found", t.BaseTask.Id),
			}
		}
	} else {
		if evergreen.IsPatchRequester(t.Requester) {
			baseTask, err = t.FindTaskOnBaseCommit()
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s' on base commit: %s", utility.FromStringPtr(obj.Id), err.Error()))
			}
		} else {
			baseTask, err = t.FindTaskOnPreviousCommit()
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s' on previous commit: %s", utility.FromStringPtr(obj.Id), err.Error()))
			}
		}
	}

	if baseTask == nil {
		return nil, nil
	}
	apiTask := &restModel.APITask{}
	err = apiTask.BuildFromService(ctx, baseTask, nil)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert baseTask %s to APITask : %s", baseTask.Id, err.Error()))
	}
	return apiTask, nil
}

// BuildVariantDisplayName is the resolver for the buildVariantDisplayName field.
func (r *taskResolver) BuildVariantDisplayName(ctx context.Context, obj *restModel.APITask) (*string, error) {
	if obj.BuildVariantDisplayName != nil {
		return obj.BuildVariantDisplayName, nil
	}
	if obj.BuildId == nil {
		return nil, nil
	}
	buildID := utility.FromStringPtr(obj.BuildId)
	b, err := build.FindOneId(buildID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to find build id: %s for task: %s, '%s'", buildID, utility.FromStringPtr(obj.Id), err.Error()))
	}
	if b == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to find build id: %s for task: %s", buildID, utility.FromStringPtr(obj.Id)))
	}
	displayName := b.DisplayName
	return &displayName, nil
}

// CanAbort is the resolver for the canAbort field.
func (r *taskResolver) CanAbort(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return *obj.Status == evergreen.TaskDispatched || *obj.Status == evergreen.TaskStarted, nil
}

// CanDisable is the resolver for the canDisable field.
func (r *taskResolver) CanDisable(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return obj.ParentTaskId == "", nil
}

// CanModifyAnnotation is the resolver for the canModifyAnnotation field.
func (r *taskResolver) CanModifyAnnotation(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return hasAnnotationPermission(ctx, obj, evergreen.AnnotationsModify.Value)
}

// CanOverrideDependencies is the resolver for the canOverrideDependencies field.
func (r *taskResolver) CanOverrideDependencies(ctx context.Context, obj *restModel.APITask) (bool, error) {
	currentUser := mustHaveUser(ctx)
	if obj.OverrideDependencies {
		return false, nil
	}
	// if the task is not the latest execution of the task, it can't be overridden
	if obj.Archived {
		return false, nil
	}
	requiredPermission := gimlet.PermissionOpts{
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionTasks,
		RequiredLevel: evergreen.TasksAdmin.Value,
		Resource:      *obj.ProjectId,
	}
	overrideRequesters := []string{
		evergreen.PatchVersionRequester,
		evergreen.GithubPRRequester,
	}
	if len(obj.DependsOn) > 0 && (utility.StringSliceContains(overrideRequesters, utility.FromStringPtr(obj.Requester)) ||
		currentUser.HasPermission(requiredPermission)) {
		return true, nil
	}
	return false, nil
}

// CanRestart is the resolver for the canRestart field.
func (r *taskResolver) CanRestart(ctx context.Context, obj *restModel.APITask) (bool, error) {
	t, err := obj.ToService()
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to service", *obj.Id))
	}
	return canRestartTask(t), nil
}

// CanSchedule is the resolver for the canSchedule field.
func (r *taskResolver) CanSchedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	t, err := obj.ToService()
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to service", *obj.Id))
	}
	return canScheduleTask(t), nil
}

// CanSetPriority is the resolver for the canSetPriority field.
func (r *taskResolver) CanSetPriority(ctx context.Context, obj *restModel.APITask) (bool, error) {
	if *obj.Status == evergreen.TaskUndispatched {
		return true, nil
	}
	if len(obj.ExecutionTasks) != 0 && !evergreen.IsFinishedTaskStatus(utility.FromStringPtr(obj.Status)) {
		tasks, err := task.FindByExecutionTasksAndMaxExecution(utility.FromStringPtrSlice(obj.ExecutionTasks), obj.Execution)
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("finding execution tasks for task '%s': %s", *obj.Id, err.Error()))
		}
		for _, t := range tasks {
			if t.Status == evergreen.TaskUndispatched {
				return true, nil
			}
		}
	}
	return false, nil
}

// CanUnschedule is the resolver for the canUnschedule field.
func (r *taskResolver) CanUnschedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return (obj.Activated && *obj.Status == evergreen.TaskUndispatched && obj.ParentTaskId == ""), nil
}

// DependsOn is the resolver for the dependsOn field.
func (r *taskResolver) DependsOn(ctx context.Context, obj *restModel.APITask) ([]*Dependency, error) {
	dependencies := []*Dependency{}
	if len(obj.DependsOn) == 0 {
		return nil, nil
	}
	depIds := []string{}
	for _, dep := range obj.DependsOn {
		depIds = append(depIds, dep.TaskId)
	}

	dependencyTasks, err := task.FindWithFields(task.ByIds(depIds), task.DisplayNameKey, task.StatusKey,
		task.ActivatedKey, task.BuildVariantKey, task.DetailsKey, task.DependsOnKey)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Cannot find dependency tasks for task %s: %s", *obj.Id, err.Error()))
	}

	taskMap := map[string]*task.Task{}
	for i := range dependencyTasks {
		taskMap[dependencyTasks[i].Id] = &dependencyTasks[i]
	}

	t, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting service model for APITask '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}

	for _, dep := range obj.DependsOn {
		depTask, ok := taskMap[dep.TaskId]
		if !ok {
			continue
		}
		var metStatus MetStatus
		if depTask.Status == evergreen.TaskStarted {
			metStatus = "STARTED"
		} else if !depTask.IsFinished() {
			metStatus = "PENDING"
		} else if t.SatisfiesDependency(depTask) {
			metStatus = "MET"
		} else {
			metStatus = "UNMET"
		}
		var requiredStatus RequiredStatus
		switch dep.Status {
		case model.AllStatuses:
			requiredStatus = "MUST_FINISH"
		case evergreen.TaskFailed:
			requiredStatus = "MUST_FAIL"
		default:
			requiredStatus = "MUST_SUCCEED"
		}

		dependency := Dependency{
			Name:           depTask.DisplayName,
			BuildVariant:   depTask.BuildVariant,
			MetStatus:      metStatus,
			RequiredStatus: requiredStatus,
			TaskID:         dep.TaskId,
		}

		dependencies = append(dependencies, &dependency)
	}
	return dependencies, nil
}

// DisplayTask is the resolver for the displayTask field.
func (r *taskResolver) DisplayTask(ctx context.Context, obj *restModel.APITask) (*restModel.APITask, error) {
	t, err := task.FindOneId(*obj.Id)
	if err != nil || t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find task with id: %s", *obj.Id))
	}
	dt, err := t.GetDisplayTask()
	if dt == nil || err != nil {
		return nil, nil
	}
	apiTask := &restModel.APITask{}
	if err = apiTask.BuildFromService(ctx, dt, nil); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert display task: %s to APITask", dt.Id))
	}
	return apiTask, nil
}

// EstimatedStart is the resolver for the estimatedStart field.
func (r *taskResolver) EstimatedStart(ctx context.Context, obj *restModel.APITask) (*restModel.APIDuration, error) {
	t, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("while converting task '%s' to service", utility.FromStringPtr(obj.Id)))
	}
	start, err := model.GetEstimatedStartTime(ctx, *t)
	if err != nil {
		return nil, InternalServerError.Send(ctx, "getting estimated start time")
	}
	duration := restModel.NewAPIDuration(start)
	return &duration, nil
}

// ExecutionTasksFull is the resolver for the executionTasksFull field.
func (r *taskResolver) ExecutionTasksFull(ctx context.Context, obj *restModel.APITask) ([]*restModel.APITask, error) {
	if len(obj.ExecutionTasks) == 0 {
		return nil, nil
	}
	tasks, err := task.FindByExecutionTasksAndMaxExecution(utility.FromStringPtrSlice(obj.ExecutionTasks), obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding execution tasks for task '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	apiTasks := []*restModel.APITask{}
	for _, t := range tasks {
		apiTask := &restModel.APITask{}
		err = apiTask.BuildFromService(ctx, &t, nil)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert task %s to APITask : %s", t.Id, err.Error()))
		}
		apiTasks = append(apiTasks, apiTask)
	}
	return apiTasks, nil
}

// FailedTestCount is the resolver for the failedTestCount field.
func (r *taskResolver) FailedTestCount(ctx context.Context, obj *restModel.APITask) (int, error) {
	dbTask, err := obj.ToService()
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting service model for APITask '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}

	stats, err := dbTask.GetTestResultsStats(ctx, evergreen.GetEnvironment())
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting failed test count: %s", err.Error()))
	}

	return stats.FailedCount, nil
}

// Files is the resolver for the files field.
func (r *taskResolver) Files(ctx context.Context, obj *restModel.APITask) (*TaskFiles, error) {
	emptyTaskFiles := TaskFiles{
		FileCount:    0,
		GroupedFiles: []*GroupedFiles{},
	}
	groupedFilesList := []*GroupedFiles{}
	fileCount := 0

	if obj.DisplayOnly {
		execTasks, err := task.Find(task.ByIds(utility.FromStringPtrSlice(obj.ExecutionTasks)))
		if err != nil {
			return &emptyTaskFiles, ResourceNotFound.Send(ctx, err.Error())
		}
		for _, execTask := range execTasks {
			groupedFiles, err := getGroupedFiles(ctx, execTask.DisplayName, execTask.Id, obj.Execution)
			if err != nil {
				return &emptyTaskFiles, err
			}
			fileCount += len(groupedFiles.Files)
			groupedFilesList = append(groupedFilesList, groupedFiles)
		}
	} else {
		groupedFiles, err := getGroupedFiles(ctx, *obj.DisplayName, *obj.Id, obj.Execution)
		if err != nil {
			return &emptyTaskFiles, err
		}
		fileCount += len(groupedFiles.Files)
		groupedFilesList = append(groupedFilesList, groupedFiles)
	}
	taskFiles := TaskFiles{
		FileCount:    fileCount,
		GroupedFiles: groupedFilesList,
	}
	return &taskFiles, nil
}

// GeneratedByName is the resolver for the generatedByName field.
func (r *taskResolver) GeneratedByName(ctx context.Context, obj *restModel.APITask) (*string, error) {
	if obj.GeneratedBy == "" {
		return nil, nil
	}
	generator, err := task.FindOneIdWithFields(obj.GeneratedBy, task.DisplayNameKey)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("unable to find generator: %s", err.Error()))
	}
	if generator == nil {
		return nil, nil
	}
	name := generator.DisplayName

	return &name, nil
}

// ImageID is the resolver for the imageId field.
func (r *taskResolver) ImageID(ctx context.Context, obj *restModel.APITask) (string, error) {
	distroID := utility.FromStringPtr(obj.DistroId)
	// Don't try to look up image ID if the task has no associated distro.
	if distroID == "" {
		return "", nil
	}
	imageID, err := distro.GetImageIDFromDistro(ctx, distroID)
	if err != nil {
		return "", InternalServerError.Send(ctx, fmt.Sprintf("finding imageID from distro '%s': %s", distroID, err.Error()))
	}
	return imageID, nil
}

// IsPerfPluginEnabled is the resolver for the isPerfPluginEnabled field.
func (r *taskResolver) IsPerfPluginEnabled(ctx context.Context, obj *restModel.APITask) (bool, error) {
	if !evergreen.IsFinishedTaskStatus(utility.FromStringPtr(obj.Status)) {
		return false, nil
	}
	if !model.IsPerfEnabledForProject(*obj.ProjectId) {
		return false, nil
	}
	opts := apimodels.GetPerfCountOptions{
		SPSBaseURL:   evergreen.GetEnvironment().Settings().Cedar.SPSKanopyURL,
		TaskID:       utility.FromStringPtr(obj.Id),
		Execution:    obj.Execution,
		CedarBaseURL: evergreen.GetEnvironment().Settings().Cedar.BaseURL,
	}
	if opts.SPSBaseURL == "" && opts.CedarBaseURL == "" {
		return false, nil
	}
	result, err := apimodels.PerfResultsCount(ctx, opts)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("requesting perf data from Cedar: %s", err.Error()))
	}
	if result.NumberOfResults == 0 {
		return false, nil
	}
	return true, nil
}

// LatestExecution is the resolver for the latestExecution field.
func (r *taskResolver) LatestExecution(ctx context.Context, obj *restModel.APITask) (int, error) {
	return task.GetLatestExecution(*obj.Id)
}

// MinQueuePosition is the resolver for the minQueuePosition field.
func (r *taskResolver) MinQueuePosition(ctx context.Context, obj *restModel.APITask) (int, error) {
	position, err := model.FindMinimumQueuePositionForTask(*obj.Id)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("finding minimum queue position for task: %s", err.Error()))
	}
	if position < 0 {
		return 0, nil
	}
	return position, nil
}

// Patch is the resolver for the patch field.
func (r *taskResolver) Patch(ctx context.Context, obj *restModel.APITask) (*restModel.APIPatch, error) {
	if !evergreen.IsPatchRequester(*obj.Requester) {
		return nil, nil
	}
	apiPatch, err := data.FindPatchById(*obj.Version)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Couldn't find a patch with id: `%s` %s", *obj.Version, err.Error()))
	}
	return apiPatch, nil
}

// PatchNumber is the resolver for the patchNumber field.
func (r *taskResolver) PatchNumber(ctx context.Context, obj *restModel.APITask) (*int, error) {
	order := obj.Order
	return &order, nil
}

// Pod is the resolver for the pod field.
func (r *taskResolver) Pod(ctx context.Context, obj *restModel.APITask) (*restModel.APIPod, error) {
	if utility.FromStringPtr(obj.PodID) == "" {
		return nil, nil
	}
	pod, err := data.FindAPIPodByID(utility.FromStringPtr(obj.PodID))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding pod: %s", err.Error()))
	}
	return pod, nil
}

// Project is the resolver for the project field.
func (r *taskResolver) Project(ctx context.Context, obj *restModel.APITask) (*restModel.APIProjectRef, error) {
	pRef, err := data.FindProjectById(*obj.ProjectId, true, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding project ref for project '%s': %s", utility.FromStringPtr(obj.ProjectId), err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find a ProjectRef for project %s", *obj.ProjectId))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(*pRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("building APIProject from service: %s", err.Error()))
	}
	return &apiProjectRef, nil
}

// ProjectIdentifier is the resolver for the projectIdentifier field.
func (r *taskResolver) ProjectIdentifier(ctx context.Context, obj *restModel.APITask) (*string, error) {
	obj.GetProjectIdentifier()
	return obj.ProjectIdentifier, nil
}

// SpawnHostLink is the resolver for the spawnHostLink field.
func (r *taskResolver) SpawnHostLink(ctx context.Context, obj *restModel.APITask) (*string, error) {
	host, err := host.FindOne(ctx, host.ById(*obj.HostId))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding host for task '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	if host == nil {
		return nil, nil
	}
	if host.Distro.SpawnAllowed && utility.StringSliceContains(evergreen.ProviderUserSpawnable, host.Distro.Provider) {
		link := fmt.Sprintf("%s/spawn?distro_id=%s&task_id=%s", evergreen.GetEnvironment().Settings().Ui.Url, host.Distro.Id, *obj.Id)
		return &link, nil
	}
	return nil, nil
}

// Status is the resolver for the status field.
func (r *taskResolver) Status(ctx context.Context, obj *restModel.APITask) (string, error) {
	return *obj.DisplayStatus, nil
}

// TaskLogs is the resolver for the taskLogs field.
func (r *taskResolver) TaskLogs(ctx context.Context, obj *restModel.APITask) (*TaskLogs, error) {
	canView := hasLogViewPermission(ctx, obj)
	if !canView {
		return nil, Forbidden.Send(ctx, "insufficient permission for viewing task logs")
	}
	// Let the individual TaskLogs resolvers handle fetching logs for the task
	// We can avoid the overhead of fetching task logs that we will not view
	// and we can avoid handling errors that we will not see
	return &TaskLogs{TaskID: *obj.Id, Execution: obj.Execution}, nil
}

// Tests is the resolver for the tests field.
func (r *taskResolver) Tests(ctx context.Context, obj *restModel.APITask, opts *TestFilterOptions) (*TaskTestResult, error) {
	dbTask, err := task.FindOneIdAndExecution(utility.FromStringPtr(obj.Id), obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting service model for APITask '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}

	filterOpts, err := convertTestFilterOptions(ctx, dbTask, opts)
	if err != nil {
		return nil, err
	}

	taskResults, err := dbTask.GetTestResults(ctx, evergreen.GetEnvironment(), filterOpts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting test results for APITask '%s': %s", dbTask.Id, err.Error()))
	}

	apiResults := make([]*restModel.APITest, len(taskResults.Results))
	for i, t := range taskResults.Results {
		apiTest := &restModel.APITest{}
		if err = apiTest.BuildFromService(t.TaskID); err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		if err = apiTest.BuildFromService(&t); err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}

		apiResults[i] = apiTest
	}

	return &TaskTestResult{
		TestResults:       apiResults,
		TotalTestCount:    taskResults.Stats.TotalCount,
		FilteredTestCount: utility.FromIntPtr(taskResults.Stats.FilteredCount),
	}, nil
}

// TotalTestCount is the resolver for the totalTestCount field.
func (r *taskResolver) TotalTestCount(ctx context.Context, obj *restModel.APITask) (int, error) {
	dbTask, err := obj.ToService()
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting service model for APITask '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}

	stats, err := dbTask.GetTestResultsStats(ctx, evergreen.GetEnvironment())
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting test count: %s", err.Error()))
	}

	return stats.TotalCount, nil
}

// VersionMetadata is the resolver for the versionMetadata field.
func (r *taskResolver) VersionMetadata(ctx context.Context, obj *restModel.APITask) (*restModel.APIVersion, error) {
	v, err := model.VersionFindOneId(utility.FromStringPtr(obj.Version))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to find version id: %s for task: %s", *obj.Version, utility.FromStringPtr(obj.Id)))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find version with id: `%s`", *obj.Version))
	}
	apiVersion := &restModel.APIVersion{}
	apiVersion.BuildFromService(*v)
	return apiVersion, nil
}

// Task returns TaskResolver implementation.
func (r *Resolver) Task() TaskResolver { return &taskResolver{r} }

type taskResolver struct{ *Resolver }
