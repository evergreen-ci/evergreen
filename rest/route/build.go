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
}

func makeGetBuildByID() gimlet.RouteHandler {
	return &buildGetHandler{}
}

func (b *buildGetHandler) Factory() gimlet.RouteHandler {
	return &buildGetHandler{}
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
	taskIDs := make([]string, 0, len(foundBuild.Tasks))
	for _, t := range foundBuild.Tasks {
		taskIDs = append(taskIDs, t.Id)
	}
	var tasks []task.Task
	if len(taskIDs) > 0 {
		tasks, err = task.FindAll(db.Query(task.ByIds(taskIDs)))
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding tasks in build '%s'", b.buildId))
		}
	}
	pp, err := serviceModel.ParserProjectFindOneById(foundBuild.Version)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting project info"))
	}

	buildModel := &model.APIBuild{}
	buildModel.BuildFromService(*foundBuild, pp)
	buildModel.SetTaskCache(tasks)

	return gimlet.NewJSONResponse(buildModel)
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

		if err = serviceModel.SetBuildPriority(b.buildId, priority, user.Username()); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting build priority"))
		}
	}

	if b.Activated != nil {
		if err = serviceModel.ActivateBuildsAndTasks([]string{b.buildId}, *b.Activated, user.Username()); err != nil {
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

func (b *buildRestartHandler) Factory() gimlet.RouteHandler {
	return &buildRestartHandler{}
}

func (b *buildRestartHandler) Parse(ctx context.Context, r *http.Request) error {
	b.buildId = gimlet.GetVars(r)["build_id"]
	return nil
}

func (b *buildRestartHandler) Run(ctx context.Context) gimlet.Responder {
	usr := MustHaveUser(ctx)
	err := serviceModel.RestartAllBuildTasks(b.buildId, usr.Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "restarting all tasks in build '%s'", b.buildId))
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

	buildModel := &model.APIBuild{}
	buildModel.BuildFromService(*foundBuild, nil)
	return gimlet.NewJSONResponse(buildModel)
}
