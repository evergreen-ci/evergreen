package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

func makeVersionCreateHandler(sc data.Connector) gimlet.RouteHandler {
	return &versionCreateHandler{sc: sc}
}

type versionCreateHandler struct {
	ProjectID string    `json:"project_id"`
	Message   string    `json:"message"`
	Active    bool      `json:"activate"`
	IsAdHoc   bool      `json:"is_adhoc"`
	Config    yaml.Node `json:"config"`

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
		Message:  h.Message,
		IsAdHoc:  h.IsAdHoc,
		User:     u,
		Activate: h.Active,
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
		ReadFileFrom: model.ReadFromGithub,
	}
	var data []byte
	err = h.Config.Decode(&data)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "decoding config for project '%s'", h.ProjectID))
	}
	projectInfo.IntermediateProject, err = model.LoadProjectInto(ctx, data, opts, projectInfo.Ref.Id, p)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "loading project '%s' from config", h.ProjectID))
	}
	if projectInfo.Ref.IsVersionControlEnabled() {
		projectInfo.Config, err = model.CreateProjectConfig(data, projectInfo.Ref.Id)
		if err != nil {
			return gimlet.NewJSONErrorResponse(errors.Wrapf(err, "creating config for project '%s'", h.ProjectID))
		}
	}
	projectInfo.Project = p
	newVersion, err := h.sc.CreateVersionFromConfig(ctx, projectInfo, metadata)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "creating version from config for project '%s'", h.ProjectID))
	}
	return gimlet.NewJSONResponse(newVersion)
}
