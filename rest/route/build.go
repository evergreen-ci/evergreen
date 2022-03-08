package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
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
	dc := data.DBBuildConnector{}
	foundBuild, err := dc.FindBuildById(b.buildId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Database error"))
	}
	taskIDs := make([]string, 0, len(foundBuild.Tasks))
	for _, t := range foundBuild.Tasks {
		taskIDs = append(taskIDs, t.Id)
	}
	tc := data.DBTaskConnector{}
	tasks, err := tc.FindTasksByIds(taskIDs)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Database error"))
	}

	buildModel := &model.APIBuild{}
	err = buildModel.BuildFromService(*foundBuild)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}
	buildModel.SetTaskCache(tasks)

	return gimlet.NewJSONResponse(buildModel)
}

////////////////////////////////////////////////////////////////////////
//
// PATH /builds/{build_id}

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
		return errors.Wrap(err, "Argument read error")
	}

	if b.Activated == nil && b.Priority == nil {
		return gimlet.ErrorResponse{
			Message:    "Must set 'activated' or 'priority'",
			StatusCode: http.StatusBadRequest,
		}
	}

	return nil
}

func (b *buildChangeStatusHandler) Run(ctx context.Context) gimlet.Responder {
	user := gimlet.GetUser(ctx)
	dc := data.DBBuildConnector{}
	foundBuild, err := dc.FindBuildById(b.buildId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	if b.Priority != nil {
		priority := *b.Priority
		if ok := validPriority(priority, foundBuild.Project, user); !ok {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message: fmt.Sprintf("Insufficient privilege to set priority to %d, "+
					"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
				StatusCode: http.StatusForbidden,
			})
		}

		if err = dc.SetBuildPriority(b.buildId, priority, user.Username()); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
		}
	}

	if b.Activated != nil {
		if err = dc.SetBuildActivated(b.buildId, user.Username(), *b.Activated); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
		}
	}

	buildModel := &model.APIBuild{}

	if err = buildModel.BuildFromService(*foundBuild); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

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
	dc := data.DBBuildConnector{}
	if err := dc.AbortBuild(b.buildId, usr.Id); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Abort error"))
	}

	foundBuild, err := dc.FindBuildById(b.buildId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	buildModel := &model.APIBuild{}

	if err = buildModel.BuildFromService(*foundBuild); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

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
	dc := data.DBBuildConnector{}
	err := dc.RestartBuild(b.buildId, usr.Id)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Restart error"))
	}

	foundBuild, err := dc.FindBuildById(b.buildId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	buildModel := &model.APIBuild{}
	if err = buildModel.BuildFromService(*foundBuild); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(buildModel)
}
