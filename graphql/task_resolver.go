package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
)

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
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find aborted task %s: %s", obj.AbortInfo.TaskID, err.Error()))
		}
		abortedTaskBuild, err := build.FindOneId(abortedTask.BuildId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Problem getting build for aborted task %s: %s", abortedTask.BuildId, err.Error()))
		}
		if abortedTaskBuild == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find build %s for aborted task: %s", abortedTask.BuildId, err.Error()))
		}
		info.TaskDisplayName = abortedTask.DisplayName
		info.BuildVariantDisplayName = abortedTaskBuild.DisplayName
	}
	return &info, nil
}

func (r *taskResolver) Ami(ctx context.Context, obj *restModel.APITask) (*string, error) {
	err := obj.GetAMI()
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return obj.AMI, nil
}

func (r *taskResolver) Annotation(ctx context.Context, obj *restModel.APITask) (*restModel.APITaskAnnotation, error) {
	annotation, err := annotations.FindOneByTaskIdAndExecution(*obj.Id, obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding annotation: %s", err.Error()))
	}
	if annotation == nil {
		return nil, nil
	}
	apiAnnotation := restModel.APITaskAnnotationBuildFromService(*annotation)
	return apiAnnotation, nil
}

func (r *taskResolver) BaseStatus(ctx context.Context, obj *restModel.APITask) (*string, error) {
	t, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *obj.Id, err.Error()))
	}
	baseStatus := t.BaseTask.Status
	if baseStatus == "" {
		return nil, nil
	}
	return &baseStatus, nil
}

func (r *taskResolver) BaseTask(ctx context.Context, obj *restModel.APITask) (*restModel.APITask, error) {
	t, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *obj.Id, err.Error()))
	}

	var baseTask *task.Task
	// BaseTask is sometimes added via aggregation when Task is resolved via GetTasksByVersion.
	if t.BaseTask.Id != "" {
		baseTask, err = task.FindOneId(t.BaseTask.Id)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s: %s", t.BaseTask.Id, err.Error()))
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
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s on base commit: %s", *obj.Id, err.Error()))
			}
		} else {
			baseTask, err = t.FindTaskOnPreviousCommit()
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s on previous commit: %s", *obj.Id, err.Error()))
			}
		}
	}

	if baseTask == nil {
		return nil, nil
	}
	apiTask := &restModel.APITask{}
	err = apiTask.BuildFromService(baseTask, nil)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert baseTask %s to APITask : %s", baseTask.Id, err))
	}
	return apiTask, nil
}

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

func (r *taskResolver) CanAbort(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return *obj.Status == evergreen.TaskDispatched || *obj.Status == evergreen.TaskStarted, nil
}

func (r *taskResolver) CanModifyAnnotation(ctx context.Context, obj *restModel.APITask) (bool, error) {
	authUser := gimlet.GetUser(ctx)
	permissions := gimlet.PermissionOpts{
		Resource:      *obj.ProjectId,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionAnnotations,
		RequiredLevel: evergreen.AnnotationsModify.Value,
	}
	if authUser.HasPermission(permissions) {
		return true, nil
	}
	if utility.StringSliceContains(evergreen.PatchRequesters, utility.FromStringPtr(obj.Requester)) {
		p, err := patch.FindOneId(utility.FromStringPtr(obj.Version))
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding patch for task: %s", err.Error()))
		}
		if p == nil {
			return false, InternalServerError.Send(ctx, "patch for task doesn't exist")
		}
		if p.Author == authUser.Username() {
			return true, nil
		}
	}
	return false, nil
}

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

func (r *taskResolver) CanRestart(ctx context.Context, obj *restModel.APITask) (bool, error) {
	t, err := obj.ToService()
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to service", *obj.Id))
	}
	return canRestartTask(t), nil
}

func (r *taskResolver) CanSchedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	t, err := obj.ToService()
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to service", *obj.Id))
	}
	return canScheduleTask(t), nil
}

func (r *taskResolver) CanSetPriority(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return *obj.Status == evergreen.TaskUndispatched, nil
}

func (r *taskResolver) CanUnschedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return (obj.Activated && *obj.Status == evergreen.TaskUndispatched), nil
}

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
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *obj.Id, err.Error()))
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
	if err = apiTask.BuildFromService(dt, nil); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert display task: %s to APITask", dt.Id))
	}
	return apiTask, nil
}

func (r *taskResolver) EstimatedStart(ctx context.Context, obj *restModel.APITask) (*restModel.APIDuration, error) {
	t, err := obj.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while converting task %s to service", *obj.Id))
	}
	start, err := model.GetEstimatedStartTime(*t)
	if err != nil {
		return nil, InternalServerError.Send(ctx, "error getting estimated start time")
	}
	duration := restModel.NewAPIDuration(start)
	return &duration, nil
}

func (r *taskResolver) ExecutionTasksFull(ctx context.Context, obj *restModel.APITask) ([]*restModel.APITask, error) {
	if len(obj.ExecutionTasks) == 0 {
		return nil, nil
	}
	tasks, err := task.FindByExecutionTasksAndMaxExecution(obj.ExecutionTasks, obj.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding execution tasks for task: %s : %s", *obj.Id, err.Error()))
	}
	apiTasks := []*restModel.APITask{}
	for _, t := range tasks {
		apiTask := &restModel.APITask{}
		err = apiTask.BuildFromService(&t, nil)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert task %s to APITask : %s", t.Id, err.Error()))
		}
		apiTasks = append(apiTasks, apiTask)
	}
	return apiTasks, nil
}

func (r *taskResolver) FailedTestCount(ctx context.Context, obj *restModel.APITask) (int, error) {
	if obj.HasCedarResults {
		opts := apimodels.GetCedarTestResultsOptions{
			BaseURL:     evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:      utility.FromStringPtr(obj.Id),
			Execution:   utility.ToIntPtr(obj.Execution),
			DisplayTask: obj.DisplayOnly,
		}
		stats, err := apimodels.GetCedarTestResultsStatsWithStatusError(ctx, opts)
		if err != nil {
			return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting failed test count: %s", err))
		}
		return stats.FailedCount, nil
	}

	failedTestCount, err := task.GetTestCountByTaskIdAndFilters(*obj.Id, "", []string{evergreen.TestFailedStatus}, obj.Execution)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting failed test count: %s", err))
	}
	return failedTestCount, nil
}

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

func (r *taskResolver) IsPerfPluginEnabled(ctx context.Context, obj *restModel.APITask) (bool, error) {
	if !evergreen.IsFinishedTaskStatus(utility.FromStringPtr(obj.Status)) {
		return false, nil
	}
	if !model.IsPerfEnabledForProject(*obj.ProjectId) {
		return false, nil
	}
	opts := apimodels.GetCedarPerfCountOptions{
		BaseURL:   evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		TaskID:    utility.FromStringPtr(obj.Id),
		Execution: obj.Execution,
	}
	if opts.BaseURL == "" {
		return false, nil
	}
	result, err := apimodels.CedarPerfResultsCount(ctx, opts)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("error requesting perf data from cedar: %s", err))
	}
	if result.NumberOfResults == 0 {
		return false, nil
	}
	return true, nil
}

func (r *taskResolver) LatestExecution(ctx context.Context, obj *restModel.APITask) (int, error) {
	return task.GetLatestExecution(*obj.Id)
}

func (r *taskResolver) MinQueuePosition(ctx context.Context, obj *restModel.APITask) (int, error) {
	position, err := model.FindMinimumQueuePositionForTask(*obj.Id)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("error queue position for task: %s", err.Error()))
	}
	if position < 0 {
		return 0, nil
	}
	return position, nil
}

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

func (r *taskResolver) PatchNumber(ctx context.Context, obj *restModel.APITask) (*int, error) {
	order := obj.Order
	return &order, nil
}

func (r *taskResolver) Project(ctx context.Context, obj *restModel.APITask) (*restModel.APIProjectRef, error) {
	pRef, err := data.FindProjectById(*obj.ProjectId, true, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding project ref for project %s: %s", *obj.ProjectId, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find a ProjectRef for project %s", *obj.ProjectId))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(*pRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIProject from service: %s", err.Error()))
	}
	return &apiProjectRef, nil
}

func (r *taskResolver) ProjectIdentifier(ctx context.Context, obj *restModel.APITask) (*string, error) {
	obj.GetProjectIdentifier()
	return obj.ProjectIdentifier, nil
}

func (r *taskResolver) SpawnHostLink(ctx context.Context, obj *restModel.APITask) (*string, error) {
	host, err := host.FindOne(host.ById(*obj.HostId))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding host for task %s", *obj.Id))
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

func (r *taskResolver) Status(ctx context.Context, obj *restModel.APITask) (string, error) {
	return *obj.DisplayStatus, nil
}

func (r *taskResolver) TotalTestCount(ctx context.Context, obj *restModel.APITask) (int, error) {
	if obj.HasCedarResults {
		opts := apimodels.GetCedarTestResultsOptions{
			BaseURL:     evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:      utility.FromStringPtr(obj.Id),
			Execution:   utility.ToIntPtr(obj.Execution),
			DisplayTask: obj.DisplayOnly,
		}
		stats, err := apimodels.GetCedarTestResultsStatsWithStatusError(ctx, opts)
		if err != nil {
			return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting test count: %s", err))
		}

		return stats.TotalCount, nil
	}
	testCount, err := task.GetTestCountByTaskIdAndFilters(*obj.Id, "", nil, obj.Execution)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("getting test count: %s", err))
	}

	return testCount, nil
}

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
