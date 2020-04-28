package graphql

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"gopkg.in/mgo.v2/bson"
)

type Resolver struct {
	sc data.Connector
}

func (r *Resolver) Mutation() MutationResolver {
	return &mutationResolver{r}
}
func (r *Resolver) Patch() PatchResolver {
	return &patchResolver{r}
}
func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}
func (r *Resolver) Task() TaskResolver {
	return &taskResolver{r}
}

type mutationResolver struct{ *Resolver }

func (r *taskResolver) ReliesOn(ctx context.Context, at *restModel.APITask) ([]*Dependency, error) {
	dependencies := []*Dependency{}
	if len(at.DependsOn) == 0 {
		return dependencies, nil
	}
	depIds := []string{}
	for _, dep := range at.DependsOn {
		depIds = append(depIds, dep.TaskId)
	}

	dependencyTasks, err := task.Find(task.ByIds(depIds).WithFields(task.DisplayNameKey, task.StatusKey,
		task.ActivatedKey, task.BuildVariantKey, task.DetailsKey, task.DependsOnKey))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Cannot find dependency tasks for task %s: %s", *at.Id, err.Error()))
	}

	taskMap := map[string]*task.Task{}
	for i := range dependencyTasks {
		taskMap[dependencyTasks[i].Id] = &dependencyTasks[i]
	}

	i, err := at.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *at.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *at.Id))
	}

	for _, dep := range at.DependsOn {
		depTask, ok := taskMap[dep.TaskId]
		if !ok {
			continue
		}
		var metStatus MetStatus
		if !depTask.IsFinished() {
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
			break
		case evergreen.TaskFailed:
			requiredStatus = "MUST_FAIL"
			break
		default:
			requiredStatus = "MUST_SUCCEED"
		}

		dependency := Dependency{
			Name:           depTask.DisplayName,
			BuildVariant:   depTask.BuildVariant,
			MetStatus:      metStatus,
			RequiredStatus: requiredStatus,
			UILink:         fmt.Sprintf("/task/%s", depTask.Id),
		}

		dependencies = append(dependencies, &dependency)
	}
	return dependencies, nil
}

func (r *mutationResolver) AddFavoriteProject(ctx context.Context, identifier string) (*restModel.UIProjectFields, error) {
	p, err := model.FindOneProjectRef(identifier)
	if err != nil || p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", identifier))
	}

	usr := route.MustHaveUser(ctx)

	err = usr.AddFavoritedProject(identifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	return &restModel.UIProjectFields{
		DisplayName: p.DisplayName,
		Identifier:  p.Identifier,
		Repo:        p.Repo,
		Owner:       p.Owner,
	}, nil
}

func (r *mutationResolver) RemoveFavoriteProject(ctx context.Context, identifier string) (*restModel.UIProjectFields, error) {
	p, err := model.FindOneProjectRef(identifier)
	if err != nil || p == nil {
		return nil, &gqlerror.Error{
			Message: fmt.Sprintln("Could not find proj", identifier),
			Extensions: map[string]interface{}{
				"code": "RESOURCE_NOT_FOUND",
			},
		}

	}

	usr := route.MustHaveUser(ctx)

	err = usr.RemoveFavoriteProject(identifier)
	if err != nil {
		return nil, &gqlerror.Error{
			Message: fmt.Sprintln("Error removing project", identifier),
			Extensions: map[string]interface{}{
				"code": "INTERNAL_SERVER_ERROR",
			},
		}
	}

	return &restModel.UIProjectFields{
		DisplayName: p.DisplayName,
		Identifier:  p.Identifier,
		Repo:        p.Repo,
		Owner:       p.Owner,
	}, nil
}

type queryResolver struct{ *Resolver }

type patchResolver struct{ *Resolver }

func (r *patchResolver) Builds(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIBuild, error) {
	builds, err := build.FindBuildsByVersions([]string{*obj.Version})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding build by version %s: %s", *obj.Version, err.Error()))
	}
	var apiBuilds []*restModel.APIBuild
	for _, build := range builds {
		apiBuild := restModel.APIBuild{}
		err = apiBuild.BuildFromService(build)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIBuild from service: %s", err.Error()))
		}
		apiBuilds = append(apiBuilds, &apiBuild)
	}
	return apiBuilds, nil
}

func (r *patchResolver) Duration(ctx context.Context, obj *restModel.APIPatch) (*PatchDuration, error) {
	// excludes display tasks
	tasks, err := task.Find(task.ByVersion(*obj.Id).WithFields(task.TimeTakenKey, task.StartTimeKey, task.FinishTimeKey))
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	if tasks == nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	timeTaken, makespan := task.GetTimeSpent(tasks)

	// return nil if rounded timeTaken/makespan == 0s
	t := timeTaken.Round(time.Second).String()
	var tPointer *string
	if t != "0s" {
		tPointer = &t
	}
	m := makespan.Round(time.Second).String()
	var mPointer *string
	if m != "0s" {
		tPointer = &m
	}

	return &PatchDuration{
		Makespan:  tPointer,
		TimeTaken: mPointer,
	}, nil
}

func (r *patchResolver) Time(ctx context.Context, obj *restModel.APIPatch) (*PatchTime, error) {
	usr := route.MustHaveUser(ctx)

	started, err := GetFormattedDate(obj.StartTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	finished, err := GetFormattedDate(obj.FinishTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	submittedAt, err := GetFormattedDate(obj.CreateTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	return &PatchTime{
		Started:     started,
		Finished:    finished,
		SubmittedAt: *submittedAt,
	}, nil
}

func (r *patchResolver) TaskCount(ctx context.Context, obj *restModel.APIPatch) (*int, error) {
	taskCount, err := task.Count(task.ByVersion(*obj.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting task count for patch %s: %s", *obj.Id, err.Error()))
	}
	return &taskCount, nil
}

func (r *patchResolver) BaseVersionID(ctx context.Context, obj *restModel.APIPatch) (*string, error) {
	baseVersion, err := model.VersionFindOne(model.VersionBaseVersionFromPatch(*obj.ProjectId, *obj.Githash).Project(bson.M{model.VersionIdentifierKey: 1}))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting base version ID for patch %s: %s", *obj.Id, err.Error()))
	}
	if baseVersion == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("unable to find base version ID for patch %s", *obj.Id))
	}
	return &baseVersion.Id, nil
}

func (r *patchResolver) Project(ctx context.Context, apiPatch *restModel.APIPatch) (*PatchProject, error) {
	patchProject, err := GetPatchProjectVariantsAndTasksForUI(ctx, apiPatch)
	if err != nil {
		return nil, err
	}
	return patchProject, nil
}

func (r *patchResolver) ID(ctx context.Context, obj *restModel.APIPatch) (string, error) {
	return *obj.Id, nil
}

func (r *queryResolver) Patch(ctx context.Context, id string) (*restModel.APIPatch, error) {
	patch, err := r.sc.FindPatchById(id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return patch, nil
}

func (r *queryResolver) UserPatches(ctx context.Context, limit *int, page *int, patchName *string, statuses []string, userID *string, includeCommitQueue *bool) (*UserPatches, error) {
	usr := route.MustHaveUser(ctx)
	userIdParam := usr.Username()
	if userID != nil {
		userIdParam = *userID
	}
	patches, count, err := r.sc.FindPatchesByUserPatchNameStatusesCommitQueue(userIdParam, *patchName, statuses, *includeCommitQueue, *page, *limit)
	patchPointers := []*restModel.APIPatch{}
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	for i := range patches {
		patchPointers = append(patchPointers, &patches[i])
	}
	userPatches := UserPatches{
		Patches:            patchPointers,
		FilteredPatchCount: *count,
	}
	return &userPatches, nil
}

func (r *queryResolver) Task(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := task.FindOneId(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if task == nil {
		return nil, errors.Errorf("unable to find task %s", taskID)
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *task)
	return apiTask, err
}

func (r *queryResolver) Projects(ctx context.Context) (*Projects, error) {
	allProjs, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}

	usr := route.MustHaveUser(ctx)
	groupsMap := make(map[string][]*restModel.UIProjectFields)
	favorites := []*restModel.UIProjectFields{}

	for _, p := range allProjs {
		groupName := strings.Join([]string{p.Owner, p.Repo}, "/")

		uiProj := restModel.UIProjectFields{
			DisplayName: p.DisplayName,
			Identifier:  p.Identifier,
			Repo:        p.Repo,
			Owner:       p.Owner,
		}

		// favorite projects are filtered out and appended to their own array
		if utility.StringSliceContains(usr.FavoriteProjects, p.Identifier) {
			favorites = append(favorites, &uiProj)
			continue
		}
		if projs, ok := groupsMap[groupName]; ok {
			groupsMap[groupName] = append(projs, &uiProj)
		} else {
			groupsMap[groupName] = []*restModel.UIProjectFields{&uiProj}
		}
	}

	groupsArr := []*GroupedProjects{}

	for groupName, groupedProjects := range groupsMap {
		gp := GroupedProjects{
			Name:     groupName,
			Projects: groupedProjects,
		}
		groupsArr = append(groupsArr, &gp)
	}

	sort.SliceStable(groupsArr, func(i, j int) bool {
		return groupsArr[i].Name < groupsArr[j].Name
	})

	pjs := Projects{
		Favorites:     favorites,
		OtherProjects: groupsArr,
	}

	return &pjs, nil
}

func (r *queryResolver) PatchTasks(ctx context.Context, patchID string, sortBy *TaskSortCategory, sortDir *SortDirection, page *int, limit *int, statuses []string, baseStatuses []string, variant *string, taskName *string) (*PatchTasks, error) {
	sorter := ""
	if sortBy != nil {
		switch *sortBy {
		case TaskSortCategoryStatus:
			sorter = task.StatusKey
			break
		case TaskSortCategoryName:
			sorter = task.DisplayNameKey
			break
		case TaskSortCategoryBaseStatus:
			// base status is not a field on the task db model; therefore sorting by base status
			// cannot be done in the mongo query. sorting by base status is done in the resolver.
			break
		case TaskSortCategoryVariant:
			sorter = task.BuildVariantKey
			break
		default:
			break
		}
	}
	sortDirParam := 1
	if *sortDir == SortDirectionDesc {
		sortDirParam = -1
	}
	pageParam := 0
	if page != nil {
		pageParam = *page
	}
	limitParam := 0
	if limit != nil {
		limitParam = *limit
	}
	statusesParam := []string{}
	if statuses != nil {
		statusesParam = statuses
	}
	variantParam := ""
	if variant != nil {
		variantParam = *variant
	}
	taskNameParam := ""
	if taskName != nil {
		taskNameParam = *taskName
	}
	tasks, count, err := r.sc.FindTasksByVersion(patchID, sorter, statusesParam, variantParam, taskNameParam, sortDirParam, pageParam, limitParam)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting patch tasks for %s: %s", patchID, err.Error()))
	}
	baseTaskStatuses, err := GetBaseTaskStatusesFromPatchID(r, patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting base task statuses for %s: %s", patchID, err.Error()))
	}
	taskResults := ConvertDBTasksToGqlTasks(tasks, baseTaskStatuses)
	if *sortBy == TaskSortCategoryBaseStatus {
		sort.SliceStable(taskResults, func(i, j int) bool {
			if sortDirParam == 1 {
				return taskResults[i].BaseStatus < taskResults[j].BaseStatus
			}
			return taskResults[i].BaseStatus > taskResults[j].BaseStatus
		})
	}
	if len(baseStatuses) > 0 {
		// tasks cannot be filtered by base status through a DB query. tasks are filtered by base status here.
		allTasks, err := task.Find(task.ByVersion(patchID))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting tasks for patch %s: %s", patchID, err.Error()))
		}
		taskResults = FilterTasksByBaseStatuses(taskResults, baseStatuses, baseTaskStatuses)
		if count > 0 {
			// calculate filtered task count by base status
			allTasksGql := ConvertDBTasksToGqlTasks(allTasks, baseTaskStatuses)
			count = len(FilterTasksByBaseStatuses(allTasksGql, baseStatuses, baseTaskStatuses))
		}
	}
	patchTasks := PatchTasks{
		Count: count,
		Tasks: taskResults,
	}
	return &patchTasks, nil
}

func (r *queryResolver) TaskTests(ctx context.Context, taskID string, sortCategory *TestSortCategory, sortDirection *SortDirection, page *int, limit *int, testName *string, statuses []string) (*TaskTestResult, error) {
	task, err := task.FindOneId(taskID)
	if task == nil || err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}

	sortBy := ""
	if sortCategory != nil {
		switch *sortCategory {
		case TestSortCategoryStatus:
			sortBy = testresult.StatusKey
			break
		case TestSortCategoryDuration:
			sortBy = "duration"
			break
		case TestSortCategoryTestName:
			sortBy = testresult.TestFileKey
		}
	}

	sortDir := 1
	if sortDirection != nil {
		switch *sortDirection {
		case SortDirectionDesc:
			sortDir = -1
			break
		}
	}

	if *sortDirection == SortDirectionDesc {
		sortDir = -1
	}

	testNameParam := ""
	if testName != nil {
		testNameParam = *testName
	}
	pageParam := 0
	if page != nil {
		pageParam = *page
	}
	limitParam := 0
	if limit != nil {
		limitParam = *limit
	}
	statusesParam := []string{}
	if statuses != nil {
		statusesParam = statuses
	}
	paginatedFilteredTests, err := r.sc.FindTestsByTaskIdFilterSortPaginate(taskID, testNameParam, statusesParam, sortBy, sortDir, pageParam, limitParam, task.Execution)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}

	testPointers := []*restModel.APITest{}
	for _, t := range paginatedFilteredTests {
		apiTest := restModel.APITest{}
		buildErr := apiTest.BuildFromService(&t)
		if buildErr != nil {
			return nil, InternalServerError.Send(ctx, buildErr.Error())
		}
		if apiTest.Logs.HTMLDisplayURL != nil && IsURL(*apiTest.Logs.HTMLDisplayURL) == false {
			formattedURL := fmt.Sprintf("%s%s", r.sc.GetURL(), *apiTest.Logs.HTMLDisplayURL)
			apiTest.Logs.HTMLDisplayURL = &formattedURL
		}
		if apiTest.Logs.RawDisplayURL != nil && IsURL(*apiTest.Logs.RawDisplayURL) == false {
			formattedURL := fmt.Sprintf("%s%s", r.sc.GetURL(), *apiTest.Logs.RawDisplayURL)
			apiTest.Logs.RawDisplayURL = &formattedURL
		}
		testPointers = append(testPointers, &apiTest)
	}

	totalTestCount, err := r.sc.GetTestCountByTaskIdAndFilters(taskID, "", []string{}, task.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting total test count: %s", err.Error()))
	}
	filteredTestCount, err := r.sc.GetTestCountByTaskIdAndFilters(taskID, testNameParam, statusesParam, task.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting filtered test count: %s", err.Error()))
	}

	taskTestResult := TaskTestResult{
		TestResults:       testPointers,
		TotalTestCount:    totalTestCount,
		FilteredTestCount: filteredTestCount,
	}

	return &taskTestResult, nil
}

func (r *queryResolver) TaskFiles(ctx context.Context, taskID string) (*TaskFiles, error) {
	emptyTaskFiles := TaskFiles{
		FileCount:    0,
		GroupedFiles: []*GroupedFiles{},
	}
	t, err := task.FindOneId(taskID)
	if t == nil {
		return &emptyTaskFiles, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	if err != nil {
		return &emptyTaskFiles, ResourceNotFound.Send(ctx, err.Error())
	}
	if t.OldTaskId != "" {
		t, err = task.FindOneId(t.OldTaskId)
		if t == nil {
			return &emptyTaskFiles, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find old task with id %s", taskID))
		}
		if err != nil {
			return &emptyTaskFiles, ResourceNotFound.Send(ctx, err.Error())
		}
	}
	groupedFilesList := []*GroupedFiles{}
	fileCount := 0
	if t.DisplayOnly {
		execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
		if err != nil {
			return &emptyTaskFiles, ResourceNotFound.Send(ctx, err.Error())
		}
		for _, execTask := range execTasks {
			groupedFiles, err := GetGroupedFiles(ctx, execTask.DisplayName, execTask.Id, t.Execution)
			if err != nil {
				return &emptyTaskFiles, err
			}
			fileCount += len(groupedFiles.Files)
			groupedFilesList = append(groupedFilesList, groupedFiles)
		}
	} else {
		groupedFiles, err := GetGroupedFiles(ctx, t.DisplayName, taskID, t.Execution)
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

func (r *queryResolver) TaskLogs(ctx context.Context, taskID string) (*RecentTaskLogs, error) {
	const logMessageCount = 100
	var loggedEvents []event.EventLogEntry
	// loggedEvents is ordered ts descending
	loggedEvents, err := event.Find(event.AllLogCollection, event.MostRecentTaskEvents(taskID, logMessageCount))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to find EventLogs for task %s: %s", taskID, err.Error()))
	}

	// remove all scheduled events except the youngest and push to filteredEvents
	filteredEvents := []event.EventLogEntry{}
	foundScheduled := false
	for i := 0; i < len(loggedEvents); i++ {
		if foundScheduled == false || loggedEvents[i].EventType != event.TaskScheduled {
			filteredEvents = append(filteredEvents, loggedEvents[i])
		}
		if loggedEvents[i].EventType == event.TaskScheduled {
			foundScheduled = true
		}
	}

	// reverse order so ts is ascending
	for i := len(filteredEvents)/2 - 1; i >= 0; i-- {
		opp := len(filteredEvents) - 1 - i
		filteredEvents[i], filteredEvents[opp] = filteredEvents[opp], filteredEvents[i]
	}

	// populate eventlogs pointer arrays
	apiEventLogPointers := []*restModel.APIEventLogEntry{}
	for _, e := range filteredEvents {
		apiEventLog := restModel.APIEventLogEntry{}
		err = apiEventLog.BuildFromService(&e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}

	// need to task to get project id
	t, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	// need project to get default logger
	p, err := r.sc.FindProjectById(t.Project)
	if p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", t.Project))
	}

	defaultLogger := p.DefaultLogger
	if defaultLogger == "" {
		defaultLogger = evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger
	}

	taskLogs := []apimodels.LogMessage{}
	systemLogs := []apimodels.LogMessage{}
	agentLogs := []apimodels.LogMessage{}
	// get logs from cedar
	if defaultLogger == model.BuildloggerLogSender {
		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().LoggerConfig.BuildloggerBaseURL,
			TaskID:        taskID,
			Execution:     t.Execution,
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.TaskLogPrefix,
		}
		// task logs
		taskLogReader, blErr := apimodels.GetBuildloggerLogs(ctx, opts)
		if blErr != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		taskLogs = apimodels.ReadBuildloggerToSlice(ctx, taskID, taskLogReader)
		// system logs
		opts.LogType = apimodels.SystemLogPrefix
		systemLogReader, blErr := apimodels.GetBuildloggerLogs(ctx, opts)
		if blErr != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		systemLogs = apimodels.ReadBuildloggerToSlice(ctx, taskID, systemLogReader)
		// agent logs
		opts.LogType = apimodels.AgentLogPrefix
		agentLogReader, blErr := apimodels.GetBuildloggerLogs(ctx, opts)
		if blErr != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		agentLogs = apimodels.ReadBuildloggerToSlice(ctx, taskID, agentLogReader)
	} else {
		// task logs
		taskLogs, err = model.FindMostRecentLogMessages(taskID, t.Execution, logMessageCount, []string{},
			[]string{apimodels.TaskLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task logs for task %s: %s", taskID, err.Error()))
		}
		// system logs
		systemLogs, err = model.FindMostRecentLogMessages(taskID, t.Execution, logMessageCount, []string{},
			[]string{apimodels.SystemLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding system logs for task %s: %s", taskID, err.Error()))
		}
		// agent logs
		agentLogs, err = model.FindMostRecentLogMessages(taskID, t.Execution, logMessageCount, []string{},
			[]string{apimodels.AgentLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding agent logs for task %s: %s", taskID, err.Error()))
		}
	}
	taskLogPointers := []*apimodels.LogMessage{}
	systemLogPointers := []*apimodels.LogMessage{}
	agentLogPointers := []*apimodels.LogMessage{}
	for i := range taskLogs {
		taskLogPointers = append(taskLogPointers, &taskLogs[i])
	}
	for i := range systemLogs {
		systemLogPointers = append(systemLogPointers, &systemLogs[i])
	}
	for i := range agentLogs {
		agentLogPointers = append(agentLogPointers, &agentLogs[i])
	}
	return &RecentTaskLogs{EventLogs: apiEventLogPointers, TaskLogs: taskLogPointers, AgentLogs: agentLogPointers, SystemLogs: systemLogPointers}, nil
}

func (r *queryResolver) PatchBuildVariants(ctx context.Context, patchID string) ([]*PatchBuildVariant, error) {
	patch, err := r.sc.FindPatchById(patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding patch `%s`: %s", patchID, err))
	}

	var tasksByVariant map[string][]*PatchBuildVariantTask = map[string][]*PatchBuildVariantTask{}
	for _, variant := range patch.Variants {
		tasksByVariant[*variant] = []*PatchBuildVariantTask{}
	}
	tasks, _, err := r.sc.FindTasksByVersion(patchID, task.DisplayNameKey, []string{}, "", "", 1, 0, 0)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting tasks for patch `%s`: %s", patchID, err))
	}
	for _, task := range tasks {
		t := PatchBuildVariantTask{
			ID:     task.Id,
			Name:   task.DisplayName,
			Status: task.Status,
		}
		tasksByVariant[task.BuildVariant] = append(tasksByVariant[task.BuildVariant], &t)
	}

	result := []*PatchBuildVariant{}
	for variant, tasks := range tasksByVariant {
		pbv := PatchBuildVariant{
			Variant: variant,
			Tasks:   tasks,
		}
		result = append(result, &pbv)
	}
	// sort variants by name
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Variant < result[j].Variant
	})
	return result, nil
}

func (r *queryResolver) CommitQueue(ctx context.Context, id string) (*restModel.APICommitQueue, error) {
	commitQueue, err := r.sc.FindCommitQueueByID(id)
	if err != nil {
		if errors.Cause(err) == err {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding commit queue for %s: %s", id, err.Error()))
		}
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding commit queue for %s: %s", id, err.Error()))
	}
	patchIds := []string{}
	for _, item := range commitQueue.Queue {
		issue := *item.Issue
		patchIds = append(patchIds, issue)
	}
	patches, err := r.sc.FindPatchesByIds(patchIds)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding patch: %s", err.Error()))
	}
	for i := range commitQueue.Queue {
		for j := range patches {
			if *commitQueue.Queue[i].Issue == *patches[j].Id {
				commitQueue.Queue[i].Patch = &patches[j]
			}
		}
	}

	return commitQueue, nil
}

func (r *mutationResolver) SetTaskPriority(ctx context.Context, taskID string, priority int) (*restModel.APITask, error) {
	t, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	authUser := gimlet.GetUser(ctx)
	if priority > evergreen.MaxTaskPriority {
		requiredPermission := gimlet.PermissionOpts{
			Resource:      t.Project,
			ResourceType:  "project",
			Permission:    evergreen.PermissionTasks,
			RequiredLevel: evergreen.TasksAdmin.Value,
		}
		isTaskAdmin := authUser.HasPermission(requiredPermission)
		if !isTaskAdmin {
			return nil, Forbidden.Send(ctx, fmt.Sprintf("Insufficient access to set priority %v, can only set priority less than or equal to %v", priority, evergreen.MaxTaskPriority))
		}
	}
	if err = t.SetPriority(int64(priority), authUser.Username()); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error setting task priority %v: %v", taskID, err.Error()))
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *t)
	return apiTask, err
}

func (r *mutationResolver) SchedulePatch(ctx context.Context, patchID string, reconfigure PatchReconfigure) (*restModel.APIPatch, error) {
	patchUpdateReq := PatchVariantsTasksRequest{}
	patchUpdateReq.BuildFromGqlInput(reconfigure)
	version, err := r.sc.FindVersionById(patchID)
	if err != nil {
		// FindVersionById does not distinguish between nil version err and db err; therefore must check that err
		// does not contain nil version err values before sending InternalServerError
		if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error occured fetching patch `%s`: %s", patchID, err.Error()))
		}
	}
	err, _, _, versionID := SchedulePatch(ctx, patchID, version, patchUpdateReq)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error scheduling patch `%s`: %s", patchID, err))
	}
	scheduledPatch, err := r.sc.FindPatchById(versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting scheduled patch `%s`: %s", patchID, err))
	}
	return scheduledPatch, nil
}

func (r *mutationResolver) ScheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := SetScheduled(ctx, r.sc, taskID, true)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (r *mutationResolver) UnscheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := SetScheduled(ctx, r.sc, taskID, false)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (r *mutationResolver) AbortTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	t, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	p, err := r.sc.FindProjectById(t.Project)
	if p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", t.Project))
	}
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding project by id: %s: %s", t.Project, err.Error()))
	}
	err = model.AbortTask(taskID, gimlet.GetUser(ctx).DisplayName())
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error aborting task %s: %s", taskID, err.Error()))
	}
	if t.Requester == evergreen.MergeTestRequester {
		_, err = commitqueue.RemoveCommitQueueItem(t.Project, p.CommitQueue.PatchType, t.Version, true)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to remove commit queue item for project %s, version %s: %s", taskID, t.Version, err.Error()))
		}
	}
	t, err = r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *t)
	return apiTask, err
}

func (r *mutationResolver) RestartTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	usr := route.MustHaveUser(ctx)
	username := usr.Username()
	if err := model.TryResetTask(taskID, username, evergreen.UIPackage, nil); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error restarting task %s: %s", taskID, err.Error()))
	}
	t, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *t)
	return apiTask, err
}

func (r *mutationResolver) SaveSubscription(ctx context.Context, subscription restModel.APISubscription) (bool, error) {
	usr := route.MustHaveUser(ctx)
	username := usr.Username()

	var id string
	var idType string
	for _, s := range subscription.Selectors {
		if s.Type == nil {
			return false, InputValidationError.Send(ctx, "Found nil for selector type. Selector type must be a string and not nil.")
		}
		switch *s.Type {
		case "object":
			idType = *s.Data
			break
		case "id":
			id = *s.Data
			break
		case "project":
			idType = "project"
			id = *s.Data
			break
		}
	}
	if id == "" || idType == "" {
		return false, InputValidationError.Send(ctx, "Selectors do not indicate a target version, build, project, or task ID")
	}
	switch idType {
	case "task":
		t, err := r.sc.FindTaskById(id)
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", id, err.Error()))
		}
		if t == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", id))
		}
		break
	case "build":
		b, err := r.sc.FindBuildById(id)
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding build by id %s: %s", id, err.Error()))
		}
		if b == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find build with id %s", id))
		}
		break
	case "version":
		v, err := r.sc.FindVersionById(id)
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding version by id %s: %s", id, err.Error()))
		}
		if v == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find version with id %s", id))
		}
		break
	case "project":
		p, err := r.sc.FindProjectById(id)
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding project by id %s: %s", id, err.Error()))
		}
		if p == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find project with id %s", id))
		}
		break
	default:
		return false, InputValidationError.Send(ctx, "Selectors do not indicate a target version, build, project, or task ID")
	}
	err := r.sc.SaveSubscriptions(username, []restModel.APISubscription{subscription})
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("error saving subscription: %s", err.Error()))
	}
	return true, nil
}

func (r *queryResolver) User(ctx context.Context) (*restModel.APIUser, error) {
	usr := route.MustHaveUser(ctx)
	displayName := usr.DisplayName()
	user := restModel.APIUser{
		DisplayName: &displayName,
	}
	return &user, nil
}

type taskResolver struct{ *Resolver }

func (r *taskResolver) PatchNumber(ctx context.Context, obj *restModel.APITask) (*int, error) {
	patch, err := r.sc.FindPatchById(*obj.Version)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error retrieving patch %s: %s", *obj.Version, err.Error()))
	}
	return &patch.PatchNumber, nil
}

func (r *taskResolver) FailedTestCount(ctx context.Context, obj *restModel.APITask) (int, error) {
	failedTestCount, err := r.sc.GetTestCountByTaskIdAndFilters(*obj.Id, "", []string{evergreen.TestFailedStatus}, obj.Execution)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("Error getting tests for failedTestCount: %s", err.Error()))
	}
	return failedTestCount, nil
}

func (r *taskResolver) PatchMetadata(ctx context.Context, obj *restModel.APITask) (*PatchMetadata, error) {
	patch, err := r.sc.FindPatchById(*obj.Version)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error retrieving patch %s: %s", *obj.Version, err.Error()))
	}
	patchMetadata := PatchMetadata{
		Author: *patch.Author,
	}
	return &patchMetadata, nil
}

func (r *taskResolver) BaseTaskMetadata(ctx context.Context, at *restModel.APITask) (*BaseTaskMetadata, error) {
	i, err := at.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *at.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *at.Id))
	}
	baseTask, err := t.FindTaskOnBaseCommit()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s on base commit", *at.Id))
	}
	if baseTask == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find task %s on base commit", *at.Id))
	}

	dur := restModel.NewAPIDuration(baseTask.TimeTaken)
	baseTaskMetadata := BaseTaskMetadata{
		BaseTaskLink:     fmt.Sprintf("/task/%s", baseTask.Id),
		BaseTaskDuration: &dur,
	}
	if baseTask.TimeTaken == 0 {
		baseTaskMetadata.BaseTaskDuration = nil
	}
	return &baseTaskMetadata, nil
}

func (r *taskResolver) SpawnHostLink(ctx context.Context, at *restModel.APITask) (*string, error) {
	host, err := host.FindOne(host.ById(*at.HostId))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding host for task %s", *at.Id))
	}
	if host == nil {
		return nil, nil
	}
	if host.Distro.SpawnAllowed && utility.StringSliceContains(evergreen.ProviderUserSpawnable, host.Distro.Provider) {
		link := fmt.Sprintf("%s/spawn?distro_id=%s&task_id=%s", evergreen.GetEnvironment().Settings().Ui.Url, host.Distro.Id, *at.Id)
		return &link, nil
	}
	return nil, nil
}

// New injects resources into the resolvers, such as the data connector
func New(apiURL string) Config {
	return Config{
		Resolvers: &Resolver{
			sc: &data.DBConnector{URL: apiURL},
		},
	}
}
