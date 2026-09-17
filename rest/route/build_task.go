package route

import (
	"cmp"
	"context"
	"net/http"
	"slices"

	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type tasksByBuildHandler struct {
	buildId            string
	status             string
	fetchAllExecutions bool
	fetchParentIds     bool
	limit              int
	key                string

	parsleyURL string
}

func makeFetchTasksByBuild(parsleyURL string) gimlet.RouteHandler {
	return &tasksByBuildHandler{
		limit:      defaultLimit,
		parsleyURL: parsleyURL,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		List tasks by build
//	@Description	List all tasks within a specific build.
//	@Tags			tasks
//	@Router			/builds/{build_id}/tasks [get]
//	@Security		Api-User || Api-Key
//	@Param			build_id				path	string	true	"the build ID"
//	@Param			start_at				query	string	false	"The identifier of the task to start at in the pagination"
//	@Param			limit					query	int		false	"The number of tasks to be returned per page of pagination. Defaults to 100"
//	@Param			fetch_all_executions	query	boolean	false	"Fetches previous executions of tasks if they are available"
//	@Param			fetch_parent_ids		query	boolean	false	"Fetches the parent display task ID for each returned execution task"
//	@Success		200						{array}	model.APITask
func (tbh *tasksByBuildHandler) Factory() gimlet.RouteHandler {
	return &tasksByBuildHandler{
		limit:      tbh.limit,
		parsleyURL: tbh.parsleyURL,
	}
}

func (tbh *tasksByBuildHandler) Parse(ctx context.Context, r *http.Request) error {
	vals := r.URL.Query()
	tbh.buildId = gimlet.GetVars(r)["build_id"]
	if tbh.buildId == "" {
		return errors.New("build ID cannot be empty")
	}

	tbh.status = vals.Get("status")
	tbh.key = vals.Get("start_at")

	var err error
	tbh.limit, err = getLimit(vals)
	if err != nil {
		return errors.Wrap(err, "getting limit")
	}

	tbh.fetchAllExecutions = vals.Get("fetch_all_executions") == "true"
	tbh.fetchParentIds = vals.Get("fetch_parent_ids") == "true"

	return nil
}

func (tbh *tasksByBuildHandler) Run(ctx context.Context) gimlet.Responder {
	// Fetch all of the tasks to be returned in this page plus the tasks used for
	// calculating information about the next page. Here the limit is multiplied
	// by two to fetch the next page.
	tasks, err := data.FindTasksByBuildId(ctx, tbh.buildId, tbh.key, tbh.status, tbh.limit+1, 1)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding tasks for build '%s'", tbh.buildId))
	}

	resp := gimlet.NewResponseBuilder()
	lastIndex := len(tasks)
	if len(tasks) > tbh.limit {
		lastIndex = tbh.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         GetURL(ctx),
				Key:             tasks[tbh.limit].Id,
				Limit:           tbh.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
		}
	}

	tasks = tasks[:lastIndex]

	// Fetch the archived executions of the whole page up front so that the
	// artifact, host, and project identifier lookups below can batch over
	// them instead of querying per task.
	oldTasksByTaskID := map[string][]task.Task{}
	if tbh.fetchAllExecutions {
		oldTasksByTaskID, err = getOldTasksForTasks(ctx, tasks)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding archived tasks for build '%s'", tbh.buildId))
		}
	}
	allTasksToSearch := tasks
	for _, oldTasks := range oldTasksByTaskID {
		allTasksToSearch = append(allTasksToSearch, oldTasks...)
	}

	artifactsCache, err := getArtifactsForTasks(ctx, allTasksToSearch)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding artifacts for tasks in build '%s'", tbh.buildId))
	}
	amisByHostID, err := getAMIsForTasks(ctx, allTasksToSearch)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding hosts for tasks in build '%s'", tbh.buildId))
	}
	projectIdentifier, foundProjectIdentifier := getProjectIdentifierForTasks(ctx, tasks)

	prevExecArgs := &model.APITaskArgs{
		IncludeArtifacts: true,
		ArtifactsCache:   artifactsCache,
		LogURL:           GetURL(ctx),
		ParsleyLogURL:    tbh.parsleyURL,
	}
	if !foundProjectIdentifier {
		prevExecArgs.IncludeProjectIdentifier = true
	}

	for i := range tasks {
		taskModel := &model.APITask{}

		if err = taskModel.BuildFromService(ctx, &tasks[i], &model.APITaskArgs{
			IncludeArtifacts: true,
			ArtifactsCache:   artifactsCache,
			LogURL:           GetURL(ctx),
			ParsleyLogURL:    tbh.parsleyURL,
		}); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting task '%s' to API model", tasks[i].Id))
		}
		if foundProjectIdentifier {
			taskModel.ProjectIdentifier = utility.ToStringPtr(projectIdentifier)
		}
		if ami := amisByHostID[tasks[i].HostId]; ami != "" {
			taskModel.AMI = utility.ToStringPtr(ami)
		}

		if tbh.fetchAllExecutions {
			oldTasks := oldTasksByTaskID[tasks[i].Id]
			if err = taskModel.BuildPreviousExecutions(ctx, oldTasks, prevExecArgs); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "adding previous task executions to API model"))
			}
			for j := range taskModel.PreviousExecutions {
				if foundProjectIdentifier {
					taskModel.PreviousExecutions[j].ProjectIdentifier = utility.ToStringPtr(projectIdentifier)
				}
				if ami := amisByHostID[oldTasks[j].HostId]; ami != "" {
					taskModel.PreviousExecutions[j].AMI = utility.ToStringPtr(ami)
				}
			}
		}

		if tbh.fetchParentIds {
			if tasks[i].IsPartOfDisplay(ctx) {
				taskModel.ParentTaskId = utility.FromStringPtr(tasks[i].DisplayTaskId)
			}
		}

		if err = resp.AddData(taskModel); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "adding response data"))
		}
	}

	return resp
}

// getOldTasksForTasks fetches the archived executions of every task in the
// page in one query.
func getOldTasksForTasks(ctx context.Context, tasks []task.Task) (map[string][]task.Task, error) {
	oldTasksByTaskID := map[string][]task.Task{}
	if len(tasks) == 0 {
		return oldTasksByTaskID, nil
	}
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.Id)
	}
	oldTasks, err := task.FindOldWithDisplayTasks(ctx, task.ByOldTaskIDs(ids))
	if err != nil {
		return nil, errors.Wrap(err, "finding archived tasks")
	}
	for _, oldTask := range oldTasks {
		oldTasksByTaskID[oldTask.OldTaskId] = append(oldTasksByTaskID[oldTask.OldTaskId], oldTask)
	}

	// Sort old tasks so they are always in the same order.
	for taskID := range oldTasksByTaskID {
		slices.SortFunc(oldTasksByTaskID[taskID], func(a, b task.Task) int {
			return cmp.Compare(a.Execution, b.Execution)
		})
	}
	return oldTasksByTaskID, nil
}

// getArtifactsForTasks fetches the artifact entries for a page of tasks in one
// query. Display tasks store their artifacts under their execution tasks.
// Archived tasks store them under the ID they ran as, not their archived _id.
func getArtifactsForTasks(ctx context.Context, tasks []task.Task) (map[artifact.TaskIDAndExecution][]artifact.Entry, error) {
	var pairs []artifact.TaskIDAndExecution
	for _, t := range tasks {
		if t.DisplayOnly {
			for _, execTaskID := range t.ExecutionTasks {
				pairs = append(pairs, artifact.TaskIDAndExecution{TaskID: execTaskID, Execution: t.Execution})
			}
			continue
		}
		taskID := t.Id
		if t.OldTaskId != "" {
			taskID = t.OldTaskId
		}
		pairs = append(pairs, artifact.TaskIDAndExecution{TaskID: taskID, Execution: t.Execution})
	}

	artifactsByTask := map[artifact.TaskIDAndExecution][]artifact.Entry{}
	if len(pairs) == 0 {
		return artifactsByTask, nil
	}
	entries, err := artifact.FindAll(ctx, artifact.ByTaskIdsAndExecutions(pairs))
	if err != nil {
		return nil, errors.Wrap(err, "finding artifacts")
	}
	for _, entry := range entries {
		key := artifact.TaskIDAndExecution{TaskID: entry.TaskId, Execution: entry.Execution}
		artifactsByTask[key] = append(artifactsByTask[key], entry)
	}
	return artifactsByTask, nil
}

// getAMIsForTasks fetches the AMI of every host running a task in the page.
func getAMIsForTasks(ctx context.Context, tasks []task.Task) (map[string]string, error) {
	var hostIDs []string
	seen := map[string]bool{}
	for _, t := range tasks {
		if t.HostId == "" || seen[t.HostId] {
			continue
		}
		seen[t.HostId] = true
		hostIDs = append(hostIDs, t.HostId)
	}

	amisByHostID := map[string]string{}
	if len(hostIDs) == 0 {
		return amisByHostID, nil
	}
	hosts, err := host.Find(ctx, host.ByIds(hostIDs))
	if err != nil {
		return nil, errors.Wrap(err, "finding hosts")
	}
	for _, h := range hosts {
		if ami := h.GetAMI(); ami != "" {
			amisByHostID[h.Id] = ami
		}
	}
	return amisByHostID, nil
}

// getProjectIdentifierForTasks resolves the project identifier shared by a page
// of tasks. Project ref with an empty identifier still sets the field rather than leaving it null
func getProjectIdentifierForTasks(ctx context.Context, tasks []task.Task) (string, bool) {
	if len(tasks) == 0 || tasks[0].Project == "" {
		return "", false
	}
	identifier, err := dbModel.GetIdentifierForProjectSecondary(ctx, tasks[0].Project)
	return identifier, err == nil
}
