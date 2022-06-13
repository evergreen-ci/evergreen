package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/graphql/resolvers/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	werrors "github.com/pkg/errors"
)

func (r *mutationResolver) AbortTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	user := gimlet.GetUser(ctx).DisplayName()
	err = model.AbortTask(taskID, user)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error aborting task %s: %s", taskID, err.Error()))
	}
	t, err = task.FindOneId(taskID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *t)
	return apiTask, err
}

func (r *mutationResolver) OverrideTaskDependencies(ctx context.Context, taskID string) (*restModel.APITask, error) {
	currentUser := util.MustHaveUser(ctx)
	t, err := task.FindByIdExecution(taskID, nil)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	if err = t.SetOverrideDependencies(currentUser.Username()); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error overriding dependencies for task %s: %s", taskID, err.Error()))
	}
	t.DisplayStatus = t.GetDisplayStatus()
	return util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *t)
}

func (r *mutationResolver) RestartTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	usr := util.MustHaveUser(ctx)
	username := usr.Username()
	if err := model.TryResetTask(taskID, username, evergreen.UIPackage, nil); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error restarting task %s: %s", taskID, err.Error()))
	}
	t, err := task.FindOneIdAndExecutionWithDisplayStatus(taskID, nil)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *t)
	return apiTask, err
}

func (r *mutationResolver) ScheduleTasks(ctx context.Context, taskIds []string) ([]*restModel.APITask, error) {
	scheduledTasks := []*restModel.APITask{}
	scheduled, err := util.SetManyTasksScheduled(ctx, r.sc.GetURL(), true, taskIds...)
	if err != nil {
		return scheduledTasks, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Failed to schedule tasks : %s", err.Error()))
	}
	scheduledTasks = append(scheduledTasks, scheduled...)
	return scheduledTasks, nil
}

func (r *mutationResolver) SetTaskPriority(ctx context.Context, taskID string, priority int) (*restModel.APITask, error) {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	authUser := gimlet.GetUser(ctx)
	if priority > evergreen.MaxTaskPriority {
		requiredPermission := gimlet.PermissionOpts{
			Resource:      t.Project,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionTasks,
			RequiredLevel: evergreen.TasksAdmin.Value,
		}
		isTaskAdmin := authUser.HasPermission(requiredPermission)
		if !isTaskAdmin {
			return nil, gqlError.Forbidden.Send(ctx, fmt.Sprintf("Insufficient access to set priority %v, can only set priority less than or equal to %v", priority, evergreen.MaxTaskPriority))
		}
	}
	if err = model.SetTaskPriority(*t, int64(priority), authUser.Username()); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error setting task priority %v: %v", taskID, err.Error()))
	}

	t, err = task.FindOneId(taskID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *t)
	return apiTask, err
}

func (r *mutationResolver) UnscheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	scheduled, err := util.SetManyTasksScheduled(ctx, r.sc.GetURL(), false, taskID)
	if err != nil {
		return nil, err
	}
	if len(scheduled) == 0 {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find task: %s", taskID))
	}
	return scheduled[0], nil
}

func (r *queryResolver) Task(ctx context.Context, taskID string, execution *int) (*restModel.APITask, error) {
	dbTask, err := task.FindOneIdAndExecutionWithDisplayStatus(taskID, execution)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, err.Error())
	}
	if dbTask == nil {
		return nil, werrors.Errorf("unable to find task %s", taskID)
	}
	apiTask, err := util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *dbTask)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, "error converting task")
	}
	return apiTask, err
}

func (r *queryResolver) TaskAllExecutions(ctx context.Context, taskID string) ([]*restModel.APITask, error) {
	latestTask, err := task.FindOneId(taskID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, err.Error())
	}
	if latestTask == nil {
		return nil, werrors.Errorf("unable to find task %s", taskID)
	}
	allTasks := []*restModel.APITask{}
	for i := 0; i < latestTask.Execution; i++ {
		var dbTask *task.Task
		dbTask, err = task.FindByIdExecution(taskID, &i)
		if err != nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, err.Error())
		}
		if dbTask == nil {
			return nil, werrors.Errorf("unable to find task %s", taskID)
		}
		var apiTask *restModel.APITask
		apiTask, err = util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *dbTask)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, "error converting task")
		}
		allTasks = append(allTasks, apiTask)
	}
	apiTask, err := util.GetAPITaskFromTask(ctx, r.sc.GetURL(), *latestTask)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, "error converting task")
	}
	allTasks = append(allTasks, apiTask)
	return allTasks, nil
}

func (r *queryResolver) TaskFiles(ctx context.Context, taskID string, execution *int) (*gqlModel.TaskFiles, error) {
	emptyTaskFiles := gqlModel.TaskFiles{
		FileCount:    0,
		GroupedFiles: []*gqlModel.GroupedFiles{},
	}
	t, err := task.FindByIdExecution(taskID, execution)
	if t == nil {
		return &emptyTaskFiles, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	if err != nil {
		return &emptyTaskFiles, gqlError.ResourceNotFound.Send(ctx, err.Error())
	}
	groupedFilesList := []*gqlModel.GroupedFiles{}
	fileCount := 0
	if t.DisplayOnly {
		execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
		if err != nil {
			return &emptyTaskFiles, gqlError.ResourceNotFound.Send(ctx, err.Error())
		}
		for _, execTask := range execTasks {
			groupedFiles, err := util.GetGroupedFiles(ctx, execTask.DisplayName, execTask.Id, t.Execution)
			if err != nil {
				return &emptyTaskFiles, err
			}
			fileCount += len(groupedFiles.Files)
			groupedFilesList = append(groupedFilesList, groupedFiles)
		}
	} else {
		groupedFiles, err := util.GetGroupedFiles(ctx, t.DisplayName, taskID, t.Execution)
		if err != nil {
			return &emptyTaskFiles, err
		}
		fileCount += len(groupedFiles.Files)
		groupedFilesList = append(groupedFilesList, groupedFiles)
	}
	taskFiles := gqlModel.TaskFiles{
		FileCount:    fileCount,
		GroupedFiles: groupedFilesList,
	}
	return &taskFiles, nil
}

func (r *queryResolver) TaskLogs(ctx context.Context, taskID string, execution *int) (*gqlModel.TaskLogs, error) {
	t, err := task.FindByIdExecution(taskID, execution)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	// need project to get default logger
	p, err := data.FindProjectById(t.Project, true, true)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("error finding project '%s': %s", t.Project, err.Error()))
	}
	if p == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", t.Project))
	}
	defaultLogger := p.DefaultLogger
	if defaultLogger == "" {
		defaultLogger = evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger
	}

	// Let the individual TaskLogs resolvers handle fetching logs for the task
	// We can avoid the overhead of fetching task logs that we will not view
	// and we can avoid handling errors that we will not see
	return &gqlModel.TaskLogs{TaskID: taskID, Execution: t.Execution, DefaultLogger: defaultLogger}, nil
}

func (r *queryResolver) TaskTests(ctx context.Context, taskID string, execution *int, sortCategory *gqlModel.TestSortCategory, sortDirection *gqlModel.SortDirection, page *int, limit *int, testName *string, statuses []string, groupID *string) (*gqlModel.TaskTestResult, error) {
	dbTask, err := task.FindByIdExecution(taskID, execution)
	if dbTask == nil || err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("finding task with id %s", taskID))
	}
	baseTask, err := dbTask.FindTaskOnBaseCommit()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("finding base task for task %s: %s", taskID, err))
	}

	limitNum := utility.FromIntPtr(limit)
	var sortBy, cedarSortBy string
	if sortCategory != nil {
		switch *sortCategory {
		case gqlModel.TestSortCategoryStatus:
			cedarSortBy = apimodels.CedarTestResultsSortByStatus
			sortBy = testresult.StatusKey
		case gqlModel.TestSortCategoryDuration:
			cedarSortBy = apimodels.CedarTestResultsSortByDuration
			sortBy = "duration"
		case gqlModel.TestSortCategoryTestName:
			cedarSortBy = apimodels.CedarTestResultsSortByTestName
			sortBy = testresult.TestFileKey
		case gqlModel.TestSortCategoryStartTime:
			cedarSortBy = apimodels.CedarTestResultsSortByStart
			sortBy = testresult.StartTimeKey
		case gqlModel.TestSortCategoryBaseStatus:
			cedarSortBy = apimodels.CedarTestResultsSortByBaseStatus
			sortBy = "base_status"
		}
	} else if limitNum > 0 { // Don't sort TaskID if unlimited EVG-13965.
		sortBy = testresult.TaskIDKey
	}

	if dbTask.HasCedarResults {
		opts := apimodels.GetCedarTestResultsOptions{
			BaseURL:      evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:       taskID,
			Execution:    utility.ToIntPtr(dbTask.Execution),
			DisplayTask:  dbTask.DisplayOnly,
			TestName:     utility.FromStringPtr(testName),
			Statuses:     statuses,
			GroupID:      utility.FromStringPtr(groupID),
			SortBy:       cedarSortBy,
			SortOrderDSC: sortDirection != nil && *sortDirection == gqlModel.SortDirectionDesc,
			Limit:        limitNum,
			Page:         utility.FromIntPtr(page),
		}
		if baseTask != nil && baseTask.HasCedarResults {
			opts.BaseTaskID = baseTask.Id
		}
		cedarTestResults, err := apimodels.GetCedarTestResultsWithStatusError(ctx, opts)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("finding test results for task %s: %s", taskID, err))
		}

		apiTestResults := make([]*restModel.APITest, len(cedarTestResults.Results))
		for i, t := range cedarTestResults.Results {
			apiTest := &restModel.APITest{}
			if err = apiTest.BuildFromService(t.TaskID); err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, err.Error())
			}
			if err = apiTest.BuildFromService(&t); err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, err.Error())
			}

			apiTestResults[i] = apiTest
		}

		return &gqlModel.TaskTestResult{
			TestResults:       apiTestResults,
			TotalTestCount:    cedarTestResults.Stats.TotalCount,
			FilteredTestCount: utility.FromIntPtr(cedarTestResults.Stats.FilteredCount),
		}, nil
	}

	baseTestStatusMap := map[string]string{}
	if baseTask != nil {
		baseTestResults, _ := r.sc.FindTestsByTaskId(data.FindTestsByTaskIdOpts{
			TaskID:         baseTask.Id,
			Execution:      baseTask.Execution,
			ExecutionTasks: baseTask.ExecutionTasks,
		})
		for _, t := range baseTestResults {
			baseTestStatusMap[t.TestFile] = t.Status
		}
	}
	sortDir := 1
	if sortDirection != nil && *sortDirection == gqlModel.SortDirectionDesc {
		sortDir = -1
	}
	filteredTestResults, err := r.sc.FindTestsByTaskId(data.FindTestsByTaskIdOpts{
		TaskID:         taskID,
		Execution:      dbTask.Execution,
		ExecutionTasks: dbTask.ExecutionTasks,
		TestName:       utility.FromStringPtr(testName),
		Statuses:       statuses,
		SortBy:         sortBy,
		SortDir:        sortDir,
		GroupID:        utility.FromStringPtr(groupID),
		Limit:          limitNum,
		Page:           utility.FromIntPtr(page),
	})
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, err.Error())
	}

	apiTestResults := make([]*restModel.APITest, len(filteredTestResults))
	for i, t := range filteredTestResults {
		apiTest := &restModel.APITest{}
		if err = apiTest.BuildFromService(t.TaskID); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, err.Error())
		}
		if err = apiTest.BuildFromService(&t); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, err.Error())
		}
		apiTest.BaseStatus = utility.ToStringPtr(baseTestStatusMap[utility.FromStringPtr(apiTest.TestFile)])

		apiTestResults[i] = apiTest
	}
	totalTestCount, err := task.GetTestCountByTaskIdAndFilters(taskID, "", []string{}, dbTask.Execution)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting total test count: %s", err))
	}
	filteredTestCount, err := task.GetTestCountByTaskIdAndFilters(taskID, utility.FromStringPtr(testName), statuses, dbTask.Execution)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting filtered test count: %s", err))
	}

	return &gqlModel.TaskTestResult{
		TestResults:       apiTestResults,
		TotalTestCount:    totalTestCount,
		FilteredTestCount: filteredTestCount,
	}, nil
}

func (r *queryResolver) TaskTestSample(ctx context.Context, tasks []string, filters []*gqlModel.TestFilter) ([]*gqlModel.TaskTestResultSample, error) {
	const testSampleLimit = 10
	if len(tasks) == 0 {
		return nil, nil
	}
	dbTasks, err := task.FindAll(db.Query(task.ByIds(tasks)))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding tasks %s: %s", tasks, err))
	}
	if len(dbTasks) == 0 {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("tasks %s not found", tasks))
	}
	testResultsToReturn := []*gqlModel.TaskTestResultSample{}

	// We can assume that if one of the tasks has cedar results, all of them do.
	if dbTasks[0].HasCedarResults {
		failingTests := []string{}
		for _, f := range filters {
			failingTests = append(failingTests, f.TestName)
		}

		results, err := util.GetCedarFailedTestResultsSample(ctx, dbTasks, failingTests)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting test results sample: %s", err))
		}
		for _, r := range results {
			tr := &gqlModel.TaskTestResultSample{
				TaskID:                  utility.FromStringPtr(r.TaskID),
				Execution:               r.Execution,
				MatchingFailedTestNames: r.MatchingFailedTestNames,
				TotalTestCount:          r.TotalFailedNames,
			}
			testResultsToReturn = append(testResultsToReturn, tr)
		}
	} else {
		testFilters := []string{}
		for _, f := range filters {
			testFilters = append(testFilters, f.TestName)
		}
		regexFilter := strings.Join(testFilters, "|")
		for _, t := range dbTasks {
			filteredTestResults, err := r.sc.FindTestsByTaskId(data.FindTestsByTaskIdOpts{
				TaskID:         t.Id,
				Execution:      t.Execution,
				ExecutionTasks: t.ExecutionTasks,
				TestName:       regexFilter,
				Statuses:       []string{evergreen.TestFailedStatus},
				SortBy:         testresult.TaskIDKey,
				Limit:          testSampleLimit,
				SortDir:        1,
				Page:           0,
			})
			if err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting test results sample: %s", err))
			}
			failedTestCount, err := task.GetTestCountByTaskIdAndFilters(t.Id, "", []string{evergreen.TestFailedStatus}, t.Execution)
			if err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting failed test count: %s", err))
			}
			tr := &gqlModel.TaskTestResultSample{
				TaskID:         t.Id,
				Execution:      t.Execution,
				TotalTestCount: failedTestCount,
			}
			matchingFailingTestNames := []string{}
			for _, r := range filteredTestResults {
				matchingFailingTestNames = append(matchingFailingTestNames, r.TestFile)
			}
			tr.MatchingFailedTestNames = matchingFailingTestNames
			testResultsToReturn = append(testResultsToReturn, tr)
		}
	}
	return testResultsToReturn, nil
}

func (r *taskResolver) AbortInfo(ctx context.Context, obj *restModel.APITask) (*gqlModel.AbortInfo, error) {
	if !obj.Aborted {
		return nil, nil
	}

	info := gqlModel.AbortInfo{
		User:       obj.AbortInfo.User,
		TaskID:     obj.AbortInfo.TaskID,
		NewVersion: obj.AbortInfo.NewVersion,
		PrClosed:   obj.AbortInfo.PRClosed,
	}

	if len(obj.AbortInfo.TaskID) > 0 {
		abortedTask, err := task.FindOneId(obj.AbortInfo.TaskID)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Problem getting aborted task %s: %s", *obj.Id, err.Error()))
		}
		if abortedTask == nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find aborted task %s: %s", obj.AbortInfo.TaskID, err.Error()))
		}
		abortedTaskBuild, err := build.FindOneId(abortedTask.BuildId)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Problem getting build for aborted task %s: %s", abortedTask.BuildId, err.Error()))
		}
		if abortedTaskBuild == nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find build %s for aborted task: %s", abortedTask.BuildId, err.Error()))
		}
		info.TaskDisplayName = abortedTask.DisplayName
		info.BuildVariantDisplayName = abortedTaskBuild.DisplayName
	}
	return &info, nil
}

func (r *taskResolver) Ami(ctx context.Context, obj *restModel.APITask) (*string, error) {
	err := obj.GetAMI()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	return obj.AMI, nil
}

func (r *taskResolver) Annotation(ctx context.Context, obj *restModel.APITask) (*restModel.APITaskAnnotation, error) {
	annotation, err := annotations.FindOneByTaskIdAndExecution(*obj.Id, obj.Execution)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding annotation: %s", err.Error()))
	}
	if annotation == nil {
		return nil, nil
	}
	apiAnnotation := restModel.APITaskAnnotationBuildFromService(*annotation)
	return apiAnnotation, nil
}

func (r *taskResolver) BaseStatus(ctx context.Context, obj *restModel.APITask) (*string, error) {
	i, err := obj.ToService()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *obj.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *obj.Id))
	}
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s on base commit", *obj.Id))
	}
	baseStatus := t.BaseTask.Status
	if baseStatus == "" {
		return nil, nil
	}
	return &baseStatus, nil
}

func (r *taskResolver) BaseTask(ctx context.Context, obj *restModel.APITask) (*restModel.APITask, error) {
	i, err := obj.ToService()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *obj.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *obj.Id))
	}

	var baseTask *task.Task
	// BaseTask is sometimes added via aggregation when Task is resolved via GetTasksByVersion.
	if t.BaseTask.Id != "" {
		baseTask, err = task.FindOneId(t.BaseTask.Id)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s: %s", t.BaseTask.Id, err.Error()))
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
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s on base commit: %s", *obj.Id, err.Error()))
			}
		} else {
			baseTask, err = t.FindTaskOnPreviousCommit()
			if err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s on previous commit: %s", *obj.Id, err.Error()))
			}
		}
	}

	if baseTask == nil {
		return nil, nil
	}
	apiTask := &restModel.APITask{}
	err = apiTask.BuildFromArgs(baseTask, nil)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert baseTask %s to APITask : %s", baseTask.Id, err))
	}
	return apiTask, nil
}

func (r *taskResolver) BuildVariantDisplayName(ctx context.Context, obj *restModel.APITask) (*string, error) {
	if obj.BuildId == nil {
		return nil, nil
	}
	buildID := utility.FromStringPtr(obj.BuildId)
	b, err := build.FindOneId(buildID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to find build id: %s for task: %s, '%s'", buildID, utility.FromStringPtr(obj.Id), err.Error()))
	}
	if b == nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to find build id: %s for task: %s", buildID, utility.FromStringPtr(obj.Id)))
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
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding patch for task: %s", err.Error()))
		}
		if p == nil {
			return false, gqlError.InternalServerError.Send(ctx, "patch for task doesn't exist")
		}
		if p.Author == authUser.Username() {
			return true, nil
		}
	}
	return false, nil
}

func (r *taskResolver) CanOverrideDependencies(ctx context.Context, obj *restModel.APITask) (bool, error) {
	currentUser := util.MustHaveUser(ctx)
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
	i, err := obj.ToService()
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to service", *obj.Id))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to Task", *obj.Id))
	}
	return util.CanRestartTask(t), nil
}

func (r *taskResolver) CanSchedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	i, err := obj.ToService()
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to service", *obj.Id))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("converting APITask '%s' to Task", *obj.Id))
	}
	return util.CanScheduleTask(t), nil
}

func (r *taskResolver) CanSetPriority(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return *obj.Status == evergreen.TaskUndispatched, nil
}

func (r *taskResolver) CanUnschedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return (obj.Activated && *obj.Status == evergreen.TaskUndispatched), nil
}

func (r *taskResolver) DependsOn(ctx context.Context, obj *restModel.APITask) ([]*gqlModel.Dependency, error) {
	dependencies := []*gqlModel.Dependency{}
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
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Cannot find dependency tasks for task %s: %s", *obj.Id, err.Error()))
	}

	taskMap := map[string]*task.Task{}
	for i := range dependencyTasks {
		taskMap[dependencyTasks[i].Id] = &dependencyTasks[i]
	}

	i, err := obj.ToService()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *obj.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *obj.Id))
	}

	for _, dep := range obj.DependsOn {
		depTask, ok := taskMap[dep.TaskId]
		if !ok {
			continue
		}
		var metStatus gqlModel.MetStatus
		if depTask.Status == evergreen.TaskStarted {
			metStatus = "STARTED"
		} else if !depTask.IsFinished() {
			metStatus = "PENDING"
		} else if t.SatisfiesDependency(depTask) {
			metStatus = "MET"
		} else {
			metStatus = "UNMET"
		}
		var requiredStatus gqlModel.RequiredStatus
		switch dep.Status {
		case model.AllStatuses:
			requiredStatus = "MUST_FINISH"
		case evergreen.TaskFailed:
			requiredStatus = "MUST_FAIL"
		default:
			requiredStatus = "MUST_SUCCEED"
		}

		dependency := gqlModel.Dependency{
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
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find task with id: %s", *obj.Id))
	}
	dt, err := t.GetDisplayTask()
	if dt == nil || err != nil {
		return nil, nil
	}
	apiTask := &restModel.APITask{}
	if err = apiTask.BuildFromArgs(dt, nil); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert display task: %s to APITask", dt.Id))
	}
	return apiTask, nil
}

func (r *taskResolver) EstimatedStart(ctx context.Context, obj *restModel.APITask) (*restModel.APIDuration, error) {
	i, err := obj.ToService()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while converting task %s to service", *obj.Id))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *obj.Id))
	}
	start, err := model.GetEstimatedStartTime(*t)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, "error getting estimated start time")
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
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding execution tasks for task: %s : %s", *obj.Id, err.Error()))
	}
	apiTasks := []*restModel.APITask{}
	for _, t := range tasks {
		apiTask := &restModel.APITask{}
		err = apiTask.BuildFromArgs(&t, nil)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert task %s to APITask : %s", t.Id, err.Error()))
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
			return 0, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting failed test count: %s", err))
		}
		return stats.FailedCount, nil
	}

	failedTestCount, err := task.GetTestCountByTaskIdAndFilters(*obj.Id, "", []string{evergreen.TestFailedStatus}, obj.Execution)
	if err != nil {
		return 0, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting failed test count: %s", err))
	}
	return failedTestCount, nil
}

func (r *taskResolver) GeneratedByName(ctx context.Context, obj *restModel.APITask) (*string, error) {
	if obj.GeneratedBy == "" {
		return nil, nil
	}
	generator, err := task.FindOneIdWithFields(obj.GeneratedBy, task.DisplayNameKey)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("unable to find generator: %s", err.Error()))
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
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error requesting perf data from cedar: %s", err))
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
		return 0, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error queue position for task: %s", err.Error()))
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
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Couldn't find a patch with id: `%s` %s", *obj.Version, err.Error()))
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
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding project ref for project %s: %s", *obj.ProjectId, err.Error()))
	}
	if pRef == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find a ProjectRef for project %s", *obj.ProjectId))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(pRef); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIProject from service: %s", err.Error()))
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
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding host for task %s", *obj.Id))
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
			return 0, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting test count: %s", err))
		}

		return stats.TotalCount, nil
	}
	testCount, err := task.GetTestCountByTaskIdAndFilters(*obj.Id, "", nil, obj.Execution)
	if err != nil {
		return 0, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("getting test count: %s", err))
	}

	return testCount, nil
}

func (r *taskResolver) VersionMetadata(ctx context.Context, obj *restModel.APITask) (*restModel.APIVersion, error) {
	v, err := model.VersionFindOneId(utility.FromStringPtr(obj.Version))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to find version id: %s for task: %s", *obj.Version, utility.FromStringPtr(obj.Id)))
	}
	if v == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find version with id: `%s`", *obj.Version))
	}
	apiVersion := &restModel.APIVersion{}
	if err = apiVersion.BuildFromService(v); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert version: %s to APIVersion", v.Id))
	}
	return apiVersion, nil
}

func (r *taskLogsResolver) AgentLogs(ctx context.Context, obj *gqlModel.TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var agentLogs []apimodels.LogMessage
	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {
		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.AgentLogPrefix,
		}
		// agent logs
		agentLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, err.Error())
		}
		agentLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, agentLogReader)
	} else {
		var err error
		// agent logs
		agentLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{},
			[]string{apimodels.AgentLogPrefix})
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding agent logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}

	agentLogPointers := []*apimodels.LogMessage{}

	for i := range agentLogs {
		agentLogPointers = append(agentLogPointers, &agentLogs[i])
	}
	return agentLogPointers, nil
}

func (r *taskLogsResolver) AllLogs(ctx context.Context, obj *gqlModel.TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var allLogs []apimodels.LogMessage

	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {

		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.AllTaskLevelLogs,
		}

		// all logs
		allLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, err.Error())
		}

		allLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, allLogReader)

	} else {
		var err error
		// all logs
		allLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{}, []string{})
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding all logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}

	allLogPointers := []*apimodels.LogMessage{}
	for i := range allLogs {
		allLogPointers = append(allLogPointers, &allLogs[i])
	}
	return allLogPointers, nil
}

func (r *taskLogsResolver) EventLogs(ctx context.Context, obj *gqlModel.TaskLogs) ([]*restModel.TaskAPIEventLogEntry, error) {
	const logMessageCount = 100
	var loggedEvents []event.EventLogEntry
	// loggedEvents is ordered ts descending
	loggedEvents, err := event.Find(event.AllLogCollection, event.MostRecentTaskEvents(obj.TaskID, logMessageCount))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to find EventLogs for task %s: %s", obj.TaskID, err.Error()))
	}

	// TODO (EVG-16969) remove once TaskScheduled events TTL
	// remove all scheduled events except the youngest and push to filteredEvents
	filteredEvents := []event.EventLogEntry{}
	foundScheduled := false
	for i := 0; i < len(loggedEvents); i++ {
		if !foundScheduled || loggedEvents[i].EventType != event.TaskScheduled {
			filteredEvents = append(filteredEvents, loggedEvents[i])
		}
		if loggedEvents[i].EventType == event.TaskScheduled {
			foundScheduled = true
		}
	}

	// reverse order so it is ascending
	for i := len(filteredEvents)/2 - 1; i >= 0; i-- {
		opp := len(filteredEvents) - 1 - i
		filteredEvents[i], filteredEvents[opp] = filteredEvents[opp], filteredEvents[i]
	}

	// populate eventlogs pointer arrays
	apiEventLogPointers := []*restModel.TaskAPIEventLogEntry{}
	for _, e := range filteredEvents {
		apiEventLog := restModel.TaskAPIEventLogEntry{}
		err = apiEventLog.BuildFromService(&e)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}
	return apiEventLogPointers, nil
}

func (r *taskLogsResolver) SystemLogs(ctx context.Context, obj *gqlModel.TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var systemLogs []apimodels.LogMessage

	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {
		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.TaskLogPrefix,
		}

		// system logs
		opts.LogType = apimodels.SystemLogPrefix
		systemLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, err.Error())
		}
		systemLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, systemLogReader)
	} else {
		var err error

		systemLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{},
			[]string{apimodels.SystemLogPrefix})
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding system logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}
	systemLogPointers := []*apimodels.LogMessage{}
	for i := range systemLogs {
		systemLogPointers = append(systemLogPointers, &systemLogs[i])
	}

	return systemLogPointers, nil
}

func (r *taskLogsResolver) TaskLogs(ctx context.Context, obj *gqlModel.TaskLogs) ([]*apimodels.LogMessage, error) {
	const logMessageCount = 100

	var taskLogs []apimodels.LogMessage

	// get logs from cedar
	if obj.DefaultLogger == model.BuildloggerLogSender {

		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        obj.TaskID,
			Execution:     utility.ToIntPtr(obj.Execution),
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.TaskLogPrefix,
		}
		// task logs
		taskLogReader, err := apimodels.GetBuildloggerLogs(ctx, opts)

		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Encountered an error while fetching build logger logs: %s", err.Error()))
		}

		taskLogs = apimodels.ReadBuildloggerToSlice(ctx, obj.TaskID, taskLogReader)

	} else {
		var err error

		// task logs
		taskLogs, err = model.FindMostRecentLogMessages(obj.TaskID, obj.Execution, logMessageCount, []string{},
			[]string{apimodels.TaskLogPrefix})
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding task logs for task %s: %s", obj.TaskID, err.Error()))
		}
	}

	taskLogPointers := []*apimodels.LogMessage{}
	for i := range taskLogs {
		taskLogPointers = append(taskLogPointers, &taskLogs[i])
	}

	return taskLogPointers, nil
}

// Task returns generated.TaskResolver implementation.
func (r *Resolver) Task() generated.TaskResolver { return &taskResolver{r} }

// TaskLogs returns generated.TaskLogsResolver implementation.
func (r *Resolver) TaskLogs() generated.TaskLogsResolver { return &taskLogsResolver{r} }

type taskResolver struct{ *Resolver }
type taskLogsResolver struct{ *Resolver }
