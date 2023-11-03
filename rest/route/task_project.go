package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// taskByProjectHandler implements the GET /projects/{project_id}/revisions/{commit_hash}/tasks.
// It fetches the associated tasks and returns them to the user.
type tasksByProjectHandler struct {
	project      string
	commitHash   string
	taskName     string
	variant      string
	variantRegex string
	status       string
	limit        int
	key          string
	url          string
	parsleyURL   string
}

func makeTasksByProjectAndCommitHandler(parsleyURL, url string) gimlet.RouteHandler {
	return &tasksByProjectHandler{
		url:        url,
		parsleyURL: parsleyURL,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		List tasks by project and commit
//	@Description	List all tasks within a mainline commit of a given project (excludes patch tasks)
//	@Tags			tasks
//	@Router			/projects/{project_name}/revisions/{commit_hash}/tasks [get]
//	@Security		Api-User || Api-Key
//	@Param			project_name	path	string	true	"project name"
//	@Param			commit_hash		path	string	true	"commit hash"
//	@Param			start_at		query	string	false	"The identifier of the task to start at in the pagination"
//	@Param			limit			query	int		false	"The number of tasks to be returned per page of pagination. Defaults to 100"
//	@Param			variant			query	string	false	"Only return tasks within this variant"
//	@Param			variant_regex	query	string	false	"Only return tasks within variants that match this regex"
//	@Param			task_name		query	string	false	"Only return tasks with this display name"
//	@Param			status			query	string	false	"Only return tasks with this status"
//	@Success		200				{array}	model.APITask
func (tph *tasksByProjectHandler) Factory() gimlet.RouteHandler {
	return &tasksByProjectHandler{
		url:        tph.url,
		parsleyURL: tph.parsleyURL}
}

// Parse fetches the project context and task status from the request
// and loads them into the arguments to be used by the execution.
func (tph *tasksByProjectHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	vals := r.URL.Query()

	tph.project = vars["project_id"]
	tph.commitHash = vars["commit_hash"]
	tph.status = vals.Get("status")
	tph.key = vals.Get("start_at")
	tph.variant = vals.Get("variant")
	tph.variantRegex = vals.Get("variant_regex")
	tph.taskName = vals.Get("task_name")

	if tph.project == "" {
		return gimlet.ErrorResponse{
			Message:    "project_id cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	if tph.variant != "" && tph.variantRegex != "" {
		return gimlet.ErrorResponse{
			Message:    "variant and variant regex cannot be used together",
			StatusCode: http.StatusBadRequest,
		}
	}

	if tph.commitHash == "" {
		return gimlet.ErrorResponse{
			Message:    "commit_hash cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	var err error
	tph.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (tph *tasksByProjectHandler) Run(ctx context.Context) gimlet.Responder {
	opts := task.GetTasksByProjectAndCommitOptions{
		Project:        tph.project,
		CommitHash:     tph.commitHash,
		StartingTaskId: tph.key,
		Status:         tph.status,
		VariantName:    tph.variant,
		VariantRegex:   tph.variantRegex,
		TaskName:       tph.taskName,
		Limit:          tph.limit + 1,
	}
	tasks, err := data.FindTasksByProjectAndCommit(opts)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "Database error"))
	}

	resp := gimlet.NewResponseBuilder()
	lastIndex := len(tasks)
	if len(tasks) > tph.limit {
		lastIndex = tph.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         tph.url,
				Key:             tasks[tph.limit].Id,
				Limit:           tph.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	tasks = tasks[:lastIndex]

	for _, t := range tasks {
		taskModel := &model.APITask{}
		err = taskModel.BuildFromService(ctx, &t, &model.APITaskArgs{
			IncludeAMI:               true,
			IncludeProjectIdentifier: true,
			IncludeArtifacts:         true,
			LogURL:                   tph.url,
			ParsleyLogURL:            tph.parsleyURL,
		})
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		err = resp.AddData(taskModel)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}
