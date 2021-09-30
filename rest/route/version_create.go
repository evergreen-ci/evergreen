package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

func makeVersionCreateHandler(sc data.Connector) gimlet.RouteHandler {
	return &versionCreateHandler{sc: sc}
}

type versionCreateHandler struct {
	ProjectID string          `json:"project_id"`
	Message   string          `json:"message"`
	Active    bool            `json:"activate"`
	IsAdHoc   bool            `json:"is_adhoc"`
	Config    json.RawMessage `json:"config"`

	sc data.Connector
}

func (h *versionCreateHandler) Factory() gimlet.RouteHandler {
	return &versionCreateHandler{sc: h.sc}
}

func (h *versionCreateHandler) Parse(ctx context.Context, r *http.Request) error {
	err := utility.ReadJSON(r.Body, h)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("error parsing request body: %s", err.Error()),
		}
	}
	return nil
}

func (h *versionCreateHandler) Run(ctx context.Context) gimlet.Responder {
	u := gimlet.GetUser(ctx).(*user.DBUser)
	metadata := model.VersionMetadata{
		Message: h.Message,
		IsAdHoc: h.IsAdHoc,
		User:    u,
	}
	projectInfo := &model.ProjectInfo{}
	var err error
	projectInfo.Ref, err = h.sc.FindProjectById(h.ProjectID, true)
	if err != nil {
		return gimlet.NewJSONErrorResponse(err)
	}
	if projectInfo.Ref == nil {
		return gimlet.NewJSONErrorResponse(errors.Errorf("project '%s' doesn't exist", h.ProjectID))
	}
	p := &model.Project{}
	opts := &model.GetProjectOpts{
		Ref:          projectInfo.Ref,
		ReadFileFrom: model.ReadfromGithub,
	}
	projectInfo.IntermediateProject, err = model.LoadProjectInto(ctx, h.Config, opts, projectInfo.Ref.Id, p)
	if err != nil {
		return gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "unable to unmarshal yaml config").Error(),
		})
	}
	projectInfo.Project = p
	newVersion, err := h.sc.CreateVersionFromConfig(ctx, projectInfo, metadata, h.Active)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(err)
	}
	return gimlet.NewJSONResponse(newVersion)
}
