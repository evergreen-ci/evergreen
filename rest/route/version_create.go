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
)

func makeVersionCreateHandler(sc data.Connector) gimlet.RouteHandler {
	return &versionCreateHandler{sc: sc}
}

type versionCreateHandler struct {
	// Required. This is the project with which the version will be associated, and the code to test will be checked out from the project's branch.
	ProjectID string `json:"project_id"`
	// Optional. A description of the version which will be displayed in the UI
	Message string `json:"message"`
	// Optional. If true, the defined tasks will run immediately. Otherwise, the version will be created and can be activated in the UI
	Active bool `json:"activate"`
	// Optional. If true, the version will be indicated as coming from an ad hoc source and will not display as if it were a patch or commit. If false, it will be assumed to be a commit.
	IsAdHoc bool `json:"is_adhoc"`
	// Required. This is the yml config that will be used for defining tasks, variants, and functions.
	Config []byte `json:"config"`

	sc data.Connector
}

// Factory creates an instance of the handler.
//
//	@Summary		Create a new version
//	@Description	Creates a version and optionally runs it, conceptually similar to a patch. The main difference is that the config yml file is provided in the request, rather than retrieved from the repo.
//	@Tags			versions
//	@Param			{object}	body	versionCreateHandler	true	"parameters"
//	@Router			/versions/ [post]
//	@Security		Api-User || Api-Key
//	@Success		200	{object}	model.APIVersion
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
	projectInfo.IntermediateProject, err = model.LoadProjectInto(ctx, h.Config, opts, projectInfo.Ref.Id, p)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "loading project '%s' from config", h.ProjectID))
	}
	if projectInfo.Ref.IsVersionControlEnabled() {
		projectInfo.Config, err = model.CreateProjectConfig(h.Config, projectInfo.Ref.Id)
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
