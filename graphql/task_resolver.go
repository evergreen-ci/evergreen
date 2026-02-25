package graphql

import (
	"context"
	"fmt"

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
	"github.com/evergreen-ci/evergreen/thirdparty/clients/fws"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"go.mongodb.org/mongo-driver/bson"
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
		abortedTask, err := task.FindOneId(ctx, obj.AbortInfo.TaskID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", obj.AbortInfo.TaskID, err.Error()))
		}
		if abortedTask == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", obj.AbortInfo.TaskID))
		}
		abortedTaskBuild, err := build.FindOneId(ctx, abortedTask.BuildId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching build '%s': %s", abortedTask.BuildId, err.Error()))
		}
		if abortedTaskBuild == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("build '%s' not found", abortedTask.BuildId))
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
	taskID := utility.FromStringPtr(obj.Id)
	hasPermission, err := hasAnnotationPermission(ctx, obj, evergreen.AnnotationsView.Value)
	if err != nil {
		return nil, err
	}
	if !hasPermission {
		return nil, Forbidden.Send(ctx, fmt.Sprintf("not authorized to view annotations for task '%s'", taskID))
	}

	annotation, err := annotations.FindOneByTaskIdAndExecution(ctx, taskID, obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding annotation for task '%s' with execution %d: %s", taskID, obj.Execution, err.Error()))
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
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to service: %s", utility.FromStringPtr(obj.Id), err.Error()))
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
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to service: %s", utility.FromStringPtr(obj.Id), err.Error()))
	}

	var baseTask *task.Task
	// BaseTask is sometimes added via aggregation when Task is resolved via GetTasksByVersion.
	if t.BaseTask.Id != "" {
		baseTask, err = task.FindOneId(ctx, t.BaseTask.Id)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", t.BaseTask.Id, err.Error()))
		}
		if baseTask == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", t.BaseTask.Id))

		}
	} else {
		if evergreen.IsPatchRequester(t.Requester) {
			baseTask, err = t.FindTaskOnBaseCommit(ctx)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s' on base commit: %s", utility.FromStringPtr(obj.DisplayName), err.Error()))
			}
		} else {
			baseTask, err = t.FindTaskOnPreviousCommit(ctx)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s' on previous commit: %s", utility.FromStringPtr(obj.DisplayName), err.Error()))
			}
		}
	}

	if baseTask == nil {
		return nil, nil
	}
	apiTask := &restModel.APITask{}
	err = apiTask.BuildFromService(ctx, baseTask, nil)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting base task '%s' to APITask: %s", baseTask.Id, err.Error()))
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
	b, err := build.FindOneId(ctx, buildID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching build '%s': %s", buildID, err.Error()))
	}
	if b == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("build '%s' not found", buildID))
	}
	displayName := b.DisplayName
	return &displayName, nil
}

// CanAbort is the resolver for the canAbort field.
func (r *taskResolver) CanAbort(ctx context.Context, obj *restModel.APITask) (bool, error) {
	taskStatus := utility.FromStringPtr(obj.Status)
	return taskStatus == evergreen.TaskDispatched || taskStatus == evergreen.TaskStarted, nil
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
		Resource:      utility.FromStringPtr(obj.ProjectId),
	}
	overrideRequesters := []string{
		evergreen.PatchVersionRequester,
		evergreen.GithubPRRequester,
	}
	if len(obj.DependsOn) > 0 && (utility.StringSliceContains(overrideRequesters, utility.FromStringPtr(obj.Requester)) ||
		currentUser.HasPermission(ctx, requiredPermission)) {
		return true, nil
	}
	return false, nil
}

// CanRestart is the resolver for the canRestart field.
func (r *taskResolver) CanRestart(ctx context.Context, obj *restModel.APITask) (bool, error) {
	t, err := obj.ToService()
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to service: %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	return canRestartTask(ctx, t), nil
}

// CanSchedule is the resolver for the canSchedule field.
func (r *taskResolver) CanSchedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	t, err := obj.ToService()
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to service: %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	return canScheduleTask(ctx, t), nil
}

// CanSetPriority is the resolver for the canSetPriority field.
func (r *taskResolver) CanSetPriority(ctx context.Context, obj *restModel.APITask) (bool, error) {
	if utility.FromStringPtr(obj.Status) == evergreen.TaskUndispatched {
		return true, nil
	}
	if len(obj.ExecutionTasks) != 0 && !evergreen.IsFinishedTaskStatus(utility.FromStringPtr(obj.Status)) {
		tasks, err := task.FindByExecutionTasksAndMaxExecution(ctx, utility.FromStringPtrSlice(obj.ExecutionTasks), obj.Execution)
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("finding execution tasks for task '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
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

	dependencyTasks, err := task.FindWithFields(ctx, task.ByIds(depIds), task.DisplayNameKey, task.StatusKey,
		task.ActivatedKey, task.BuildVariantKey, task.DetailsKey, task.DependsOnKey)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding dependency tasks for task '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}

	taskMap := map[string]*task.Task{}
	for i := range dependencyTasks {
		taskMap[dependencyTasks[i].Id] = &dependencyTasks[i]
	}

	t, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to service: %s", utility.FromStringPtr(obj.Id), err.Error()))
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
	taskID := utility.FromStringPtr(obj.Id)
	t, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	dt, err := t.GetDisplayTask(ctx)
	if dt == nil || err != nil {
		return nil, nil
	}
	apiTask := &restModel.APITask{}
	if err = apiTask.BuildFromService(ctx, dt, nil); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting display task '%s' to APITask: %s", dt.Id, err.Error()))
	}
	return apiTask, nil
}

// Errors is the resolver for the errors field.
func (r *taskResolver) Errors(ctx context.Context, obj *restModel.APITask) ([]string, error) {
	errors := []string{}
	t, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to service", utility.FromStringPtr(obj.Id)))
	}
	if !t.HasValidDistro(ctx) {
		errors = append(errors, evergreen.DistroNotFoundForTaskError)
	}
	return errors, nil
}

// EstimatedStart is the resolver for the estimatedStart field.
func (r *taskResolver) EstimatedStart(ctx context.Context, obj *restModel.APITask) (*restModel.APIDuration, error) {
	t, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to service", utility.FromStringPtr(obj.Id)))
	}
	start, err := model.GetEstimatedStartTime(ctx, *t)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting estimated start time for task '%s'", utility.FromStringPtr(obj.Id)))
	}
	duration := restModel.NewAPIDuration(start)
	return &duration, nil
}

// ExecutionTasksFull is the resolver for the executionTasksFull field.
func (r *taskResolver) ExecutionTasksFull(ctx context.Context, obj *restModel.APITask) ([]*restModel.APITask, error) {
	if len(obj.ExecutionTasks) == 0 {
		return nil, nil
	}
	tasks, err := task.FindByExecutionTasksAndMaxExecution(ctx, utility.FromStringPtrSlice(obj.ExecutionTasks), obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding execution tasks for task '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	apiTasks := []*restModel.APITask{}
	for _, t := range tasks {
		apiTask := &restModel.APITask{}
		err = apiTask.BuildFromService(ctx, &t, nil)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to APITask: %s", t.Id, err.Error()))
		}
		apiTasks = append(apiTasks, apiTask)
	}
	return apiTasks, nil
}

// FailedTestCount is the resolver for the failedTestCount field.
func (r *taskResolver) FailedTestCount(ctx context.Context, obj *restModel.APITask) (int, error) {
	taskID := utility.FromStringPtr(obj.Id)
	dbTask, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if dbTask == nil {
		return 0, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}

	stats, err := dbTask.GetTestResultsStats(ctx, evergreen.GetEnvironment())
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting failed test count for task '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
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
		if obj.ExecutionTasks == nil {
			return &emptyTaskFiles, nil
		}

		execTasks, err := task.Find(ctx, task.ByIds(utility.FromStringPtrSlice(obj.ExecutionTasks)))
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
		groupedFiles, err := getGroupedFiles(ctx, utility.FromStringPtr(obj.DisplayName), utility.FromStringPtr(obj.Id), obj.Execution)
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
	generator, err := task.FindOneIdWithFields(ctx, obj.GeneratedBy, task.DisplayNameKey)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding generator for task '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	if generator == nil {
		return nil, nil
	}
	name := generator.DisplayName

	return &name, nil
}

// Generator is the resolver for the generator field.
func (r *taskResolver) Generator(ctx context.Context, obj *restModel.APITask) (*restModel.APITask, error) {
	if obj.GeneratedBy == "" {
		return nil, nil
	}
	generator, err := task.FindOneId(ctx, obj.GeneratedBy)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding generator for task '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	if generator == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("generator task '%s' not found", obj.GeneratedBy))
	}
	apiTask := &restModel.APITask{}
	if err = apiTask.BuildFromService(ctx, generator, nil); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting generator task '%s' to APITask: %s", generator.Id, err.Error()))
	}
	return apiTask, nil
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
	if !model.IsPerfEnabledForProject(ctx, utility.FromStringPtr(obj.ProjectId)) {
		return false, nil
	}
	opts := apimodels.GetPerfCountOptions{
		SPSBaseURL: evergreen.GetEnvironment().Settings().PerfMonitoringKanopyURL,
		TaskID:     utility.FromStringPtr(obj.Id),
		Execution:  obj.Execution,
	}
	if opts.SPSBaseURL == "" {
		return false, nil
	}
	result, err := apimodels.PerfResultsCount(ctx, opts)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("requesting perf data: %s", err.Error()))
	}
	if result.NumberOfResults == 0 {
		return false, nil
	}
	return true, nil
}

// LatestExecution is the resolver for the latestExecution field.
func (r *taskResolver) LatestExecution(ctx context.Context, obj *restModel.APITask) (int, error) {
	return task.GetLatestExecution(ctx, utility.FromStringPtr(obj.Id))
}

// MinQueuePosition is the resolver for the minQueuePosition field.
func (r *taskResolver) MinQueuePosition(ctx context.Context, obj *restModel.APITask) (int, error) {
	taskID := utility.FromStringPtr(obj.Id)
	position, err := model.FindMinimumQueuePositionForTask(ctx, taskID)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("finding minimum queue position for task '%s': %s", taskID, err.Error()))
	}
	if position < 0 {
		return 0, nil
	}
	return position, nil
}

// Patch is the resolver for the patch field.
func (r *taskResolver) Patch(ctx context.Context, obj *restModel.APITask) (*restModel.APIPatch, error) {
	if !evergreen.IsPatchRequester(utility.FromStringPtr(obj.Requester)) {
		return nil, nil
	}
	apiPatch, err := data.FindPatchById(ctx, utility.FromStringPtr(obj.Version))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding patch '%s': %s", utility.FromStringPtr(obj.Version), err.Error()))
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
	podID := utility.FromStringPtr(obj.PodID)
	if podID == "" {
		return nil, nil
	}
	pod, err := data.FindAPIPodByID(ctx, podID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding pod '%s': %s", podID, err.Error()))
	}
	return pod, nil
}

// Project is the resolver for the project field.
func (r *taskResolver) Project(ctx context.Context, obj *restModel.APITask) (*restModel.APIProjectRef, error) {
	projectID := utility.FromStringPtr(obj.ProjectId)
	pRef, err := data.FindProjectById(ctx, projectID, true, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectID, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectID))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(ctx, *pRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting project '%s' to APIProjectRef: %s", projectID, err.Error()))
	}
	return &apiProjectRef, nil
}

// ProjectIdentifier is the resolver for the projectIdentifier field.
func (r *taskResolver) ProjectIdentifier(ctx context.Context, obj *restModel.APITask) (*string, error) {
	obj.GetProjectIdentifier(ctx)
	return obj.ProjectIdentifier, nil
}

// SpawnHostLink is the resolver for the spawnHostLink field.
func (r *taskResolver) SpawnHostLink(ctx context.Context, obj *restModel.APITask) (*string, error) {
	hostID := utility.FromStringPtr(obj.HostId)
	taskID := utility.FromStringPtr(obj.Id)
	host, err := host.FindOne(ctx, host.ById(hostID))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding host '%s' for task '%s': %s", hostID, taskID, err.Error()))
	}
	if host == nil {
		return nil, nil
	}
	if host.Distro.SpawnAllowed && utility.StringSliceContains(evergreen.ProviderUserSpawnable, host.Distro.Provider) {
		link := fmt.Sprintf("%s/spawn?distro_id=%s&task_id=%s", evergreen.GetEnvironment().Settings().Ui.Url, host.Distro.Id, taskID)
		return &link, nil
	}
	return nil, nil
}

// TaskLogs is the resolver for the taskLogs field.
func (r *taskResolver) TaskLogs(ctx context.Context, obj *restModel.APITask) (*TaskLogs, error) {
	canView := hasLogViewPermission(ctx, obj)
	if !canView {
		return nil, Forbidden.Send(ctx, fmt.Sprintf("not authorized to view logs for task '%s'", utility.FromStringPtr(obj.Id)))
	}
	// Let the individual TaskLogs resolvers handle fetching logs for the task
	// We can avoid the overhead of fetching task logs that we will not view
	// and we can avoid handling errors that we will not see
	return &TaskLogs{TaskID: utility.FromStringPtr(obj.Id), Execution: obj.Execution}, nil
}

// TaskOwnerTeam is the resolver for the taskOwnerTeam field.
func (r *taskResolver) TaskOwnerTeam(ctx context.Context, obj *restModel.APITask) (*TaskOwnerTeam, error) {
	fwsBaseURL := evergreen.GetEnvironment().Settings().FWS.URL
	if fwsBaseURL == "" {
		return nil, InternalServerError.Send(ctx, "Foliage Web Services URL not set")
	}
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	cfg := fws.NewConfiguration()
	cfg.HTTPClient = httpClient
	cfg.Servers = fws.ServerConfigurations{
		fws.ServerConfiguration{
			Description: "Foliage Web Services",
			URL:         fwsBaseURL,
		},
	}
	cfg.UserAgent = "evergreen"

	client := fws.NewAPIClient(cfg)
	req := client.OwnerAPI.ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet(ctx, *obj.Id)
	results, resp, err := req.Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task owner team: %s", err.Error()))
	}
	teamName := results.SelectedAssignment.GetTeamDataWithOwner().TeamData.TeamName
	return &TaskOwnerTeam{
		TeamName:       teamName,
		AssignmentType: string(results.SelectedAssignment.GetAssignmentType()),
		Messages:       results.SelectedAssignment.GetMessages(),
	}, nil
}

// Tests is the resolver for the tests field.
func (r *taskResolver) Tests(ctx context.Context, obj *restModel.APITask, opts *TestFilterOptions) (*TaskTestResult, error) {
	// Return early if it is known that there are no test results to return.
	// Display tasks cannot take advantage of this optimization since they
	// don't populate ResultsFailed.
	if opts != nil && !obj.DisplayOnly && len(opts.Statuses) > 0 {
		diffFailureStatuses := utility.GetSetDifference(opts.Statuses, evergreen.TestFailureStatuses)
		if len(diffFailureStatuses) == 0 && !obj.ResultsFailed {
			return &TaskTestResult{
				TestResults:       []*restModel.APITest{},
				TotalTestCount:    0,
				FilteredTestCount: 0,
			}, nil
		}
	}
	taskID := utility.FromStringPtr(obj.Id)
	dbTask, err := task.FindOneIdAndExecution(ctx, taskID, obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting task '%s' with execution %d: %s", taskID, obj.Execution, err.Error()))
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
	taskID := utility.FromStringPtr(obj.Id)
	dbTask, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if dbTask == nil {
		return 0, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	stats, err := dbTask.GetTestResultsStats(ctx, evergreen.GetEnvironment())
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting test count: %s", err.Error()))
	}

	return stats.TotalCount, nil
}

// VersionMetadata is the resolver for the versionMetadata field.
func (r *taskResolver) VersionMetadata(ctx context.Context, obj *restModel.APITask) (*restModel.APIVersion, error) {
	versionID := utility.FromStringPtr(obj.Version)
	v, err := model.VersionFindOne(ctx, model.VersionById(versionID).Project(bson.M{model.VersionBuildVariantsKey: 0}))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s' for task '%s': %s", versionID, utility.FromStringPtr(obj.Id), err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}
	apiVersion := &restModel.APIVersion{}
	apiVersion.BuildFromService(ctx, *v)
	return apiVersion, nil
}

// Task returns TaskResolver implementation.
func (r *Resolver) Task() TaskResolver { return &taskResolver{r} }

type taskResolver struct{ *Resolver }
