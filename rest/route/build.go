package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// Handler for fetching build by id
//
//    /builds/{build_id}

type buildGetHandler struct {
	buildId string
	env     evergreen.Environment
}

func makeGetBuildByID(env evergreen.Environment) gimlet.RouteHandler {
	return &buildGetHandler{env: env}
}

// Factory creates an instance of the handler.
//
//	@Summary		Fetch build by ID
//	@Description	Fetches a single build using its ID
//	@Tags			builds
//	@Router			/builds/{build_id} [get]
//	@Security		Api-User || Api-Key
//	@Param			build_id	path		string	true	"the build ID"
//	@Success		200			{object}	model.APIBuild
func (b *buildGetHandler) Factory() gimlet.RouteHandler {
	return &buildGetHandler{env: b.env}
}

func (b *buildGetHandler) Parse(ctx context.Context, r *http.Request) error {
	b.buildId = gimlet.GetVars(r)["build_id"]
	return nil
}

func (b *buildGetHandler) Run(ctx context.Context) gimlet.Responder {
	foundBuild, err := build.FindOneId(b.buildId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding build '%s'", b.buildId))
	}
	if foundBuild == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("build '%s' not found", b.buildId),
		})
	}

	v, err := serviceModel.VersionFindOneId(foundBuild.Version)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding version '%s'", foundBuild.Version))
	}
	if v == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version '%s' not found", foundBuild.Version),
		})
	}

	pp, err := serviceModel.ParserProjectFindOneByID(ctx, b.env.Settings(), v.ProjectStorageMethod, v.Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting project info"))
	}

	buildModel := &model.APIBuild{}
	buildModel.BuildFromService(*foundBuild, pp)
	if err := setBuildTaskCache(foundBuild, buildModel); err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "setting task cache for build"))
	}

	return gimlet.NewJSONResponse(buildModel)
}

func setBuildTaskCache(b *build.Build, apiBuild *model.APIBuild) error {
	taskIDs := make([]string, 0, len(b.Tasks))
	for _, t := range b.Tasks {
		taskIDs = append(taskIDs, t.Id)
	}
	if len(taskIDs) == 0 {
		return nil
	}

	tasks, err := task.FindAll(db.Query(task.ByIds(taskIDs)))
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding tasks").Error(),
		}
	}

	apiBuild.SetTaskCache(tasks)

	return nil
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /builds/{build_id}

type buildChangeStatusHandler struct {
	Activated *bool  `json:"activated"`
	Priority  *int64 `json:"priority"`

	buildId string
}

func makeChangeStatusForBuild() gimlet.RouteHandler {
	return &buildChangeStatusHandler{}

}

// Factory creates an instance of the handler.
//
//	@Summary		Change a build's execution status
//	@Description	Change the current execution status of a build. Accepts a JSON body with the new build status to be set.
//	@Tags			builds
//	@Router			/builds/{build_id} [patch]
//	@Security		Api-User || Api-Key
//	@Param			{object}	body		buildChangeStatusHandler	true	"parameters"
//	@Success		200			{object}	model.APIBuild
func (b *buildChangeStatusHandler) Factory() gimlet.RouteHandler {
	return &buildChangeStatusHandler{}
}

func (b *buildChangeStatusHandler) Parse(ctx context.Context, r *http.Request) error {
	b.buildId = gimlet.GetVars(r)["build_id"]
	body := utility.NewRequestReader(r)
	defer body.Close()

	if err := utility.ReadJSON(body, b); err != nil {
		return errors.Wrap(err, "parsing JSON request body")
	}

	if b.Activated == nil && b.Priority == nil {
		return gimlet.ErrorResponse{
			Message:    "must set 'activated' or 'priority'",
			StatusCode: http.StatusBadRequest,
		}
	}

	return nil
}

func (b *buildChangeStatusHandler) Run(ctx context.Context) gimlet.Responder {
	user := gimlet.GetUser(ctx)
	foundBuild, err := build.FindOneId(b.buildId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding build '%s'", b.buildId))
	}
	if foundBuild == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("build '%s' not found", b.buildId),
		})
	}

	if b.Priority != nil {
		priority := *b.Priority
		if ok := validPriority(priority, foundBuild.Project, user); !ok {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message: fmt.Sprintf("insufficient privilege to set priority to %d, "+
					"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
				StatusCode: http.StatusForbidden,
			})
		}

		if err = serviceModel.SetBuildPriority(ctx, b.buildId, priority, user.Username()); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting build priority"))
		}
	}

	if b.Activated != nil {
		if err = serviceModel.ActivateBuildsAndTasks(ctx, []string{b.buildId}, *b.Activated, user.Username()); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting build activation"))
		}
	}

	updatedBuild, err := build.FindOneId(b.buildId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "finding updated build '%s'", b.buildId))
	}
	if updatedBuild == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("build '%s' not found", b.buildId),
		})
	}

	buildModel := &model.APIBuild{}
	buildModel.BuildFromService(*updatedBuild, nil)
	return gimlet.NewJSONResponse(buildModel)
}

////////////////////////////////////////////////////////////////////////
//
// Handler for aborting build by id
//
//    /builds/{build_id}/abort

type buildAbortHandler struct {
	buildId string
}

func makeAbortBuild() gimlet.RouteHandler {
	return &buildAbortHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Abort a build
//	@Description	Abort the build of the given ID. Can only be performed if the build is in progress.
//	@Tags			builds
//	@Router			/builds/{build_id}/abort [post]
//	@Security		Api-User || Api-Key
//	@Param			build_id	path		string	true	"build ID"
//	@Success		200			{object}	model.APIBuild
func (b *buildAbortHandler) Factory() gimlet.RouteHandler {
	return &buildAbortHandler{}
}

func (b *buildAbortHandler) Parse(ctx context.Context, r *http.Request) error {
	b.buildId = gimlet.GetVars(r)["build_id"]
	return nil
}

func (b *buildAbortHandler) Run(ctx context.Context) gimlet.Responder {
	usr := MustHaveUser(ctx)
	if err := serviceModel.AbortBuild(b.buildId, usr.Id); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "aborting build '%s'", b.buildId))
	}

	foundBuild, err := build.FindOneId(b.buildId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding updated build '%s'", b.buildId))
	}
	if foundBuild == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("build '%s' not found", b.buildId),
		})
	}

	buildModel := &model.APIBuild{}
	buildModel.BuildFromService(*foundBuild, nil)
	return gimlet.NewJSONResponse(buildModel)
}

////////////////////////////////////////////////////////////////////////
//
// Handler for restarting build by id
//
//    /builds/{build_id}/restart

type buildRestartHandler struct {
	buildId string
}

func makeRestartBuild() gimlet.RouteHandler {
	return &buildRestartHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Restart a build
//	@Description	Restarts the build of the given ID. Can only be performed if the build is finished.
//	@Tags			builds
//	@Router			/builds/{build_id}/restart [post]
//	@Security		Api-User || Api-Key
//	@Param			build_id	path		string	true	"build ID"
//	@Success		200			{object}	model.APIBuild
func (b *buildRestartHandler) Factory() gimlet.RouteHandler {
	return &buildRestartHandler{}
}

func (b *buildRestartHandler) Parse(ctx context.Context, r *http.Request) error {
	b.buildId = gimlet.GetVars(r)["build_id"]
	return nil
}

func (b *buildRestartHandler) Run(ctx context.Context) gimlet.Responder {
	usr := MustHaveUser(ctx)
	taskIds, err := task.FindAllTaskIDsFromBuild(b.buildId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting tasks for build '%s'", b.buildId))
	}
	foundBuild, err := build.FindOneId(b.buildId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding build '%s'", b.buildId))
	}
	if foundBuild == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("build '%s' not found", b.buildId),
		})
	}

	err = serviceModel.RestartBuild(ctx, foundBuild, taskIds, true, usr.Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "restarting all tasks in build '%s'", b.buildId))
	}

	updatedBuild, err := build.FindOneId(b.buildId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding build '%s'", b.buildId))
	}
	if updatedBuild == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("build '%s' not found", b.buildId),
		})
	}

	buildModel := &model.APIBuild{}
	buildModel.BuildFromService(*updatedBuild, nil)
	return gimlet.NewJSONResponse(buildModel)
}
