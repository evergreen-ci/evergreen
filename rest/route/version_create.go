package route

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

func makeVersionCreateHandler() gimlet.RouteHandler {
	return &versionCreateHandler{}
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
		return errors.Wrap(err, "reading version creation options from JSON request body")
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
	projectInfo.Ref, err = data.FindProjectById(h.ProjectID, true, true)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "finding project '%s'", h.ProjectID))
	}
	if projectInfo.Ref == nil {
		return gimlet.NewJSONErrorResponse(errors.Errorf("project '%s' not found", h.ProjectID))
	}
	p := &model.Project{}
	opts := &model.GetProjectOpts{
		Ref:          projectInfo.Ref,
		ReadFileFrom: model.ReadfromGithub,
	}
	projectInfo.IntermediateProject, err = model.LoadProjectInto(ctx, h.Config, opts, projectInfo.Ref.Id, p)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "loading project from config"))
	}
	if projectInfo.Ref.IsVersionControlEnabled() {
		projectInfo.Config, err = model.CreateProjectConfig(h.Config, projectInfo.Ref.Id)
		if err != nil {
			return gimlet.NewJSONErrorResponse(errors.Wrap(err, "creating project config"))
		}
	}
	projectInfo.Project = p
	newVersion, err := h.sc.CreateVersionFromConfig(ctx, projectInfo, metadata, h.Active)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "creating version from project config"))
	}
	return gimlet.NewJSONResponse(newVersion)
}
