package route

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/validate

type validateProjectHandler struct {
	input validator.ValidationInput
}

func makeValidateProject() gimlet.RouteHandler {
	return &validateProjectHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Validate a project
//	@Description	Validate a project configuration file.
//	@Tags			projects
//	@Router			/validate [post]
//	@Security		Api-User || Api-Key
//	@Param			{object}	body		validator.ValidationInput	true	"parameters"
//	@Success		200			{object}	validator.ValidationErrors
func (v *validateProjectHandler) Factory() gimlet.RouteHandler {
	return &validateProjectHandler{}
}

func (v *validateProjectHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()

	bytes, err := io.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Error reading request body")
	}

	if !json.Valid(bytes) {
		return errors.New("Invalid JSON format detected; potential incomplete or corrupted data.")
	}

	input := validator.ValidationInput{}

	if err := json.Unmarshal(bytes, &input); err != nil {
		return errors.Wrap(err, "Unmarshaling request body")
	}

	v.input = input
	return nil
}

func (v *validateProjectHandler) Run(ctx context.Context) gimlet.Responder {

	project := &model.Project{}
	var projectConfig *model.ProjectConfig
	var err error

	opts := &model.GetProjectOpts{
		ReadFileFrom: model.ReadFromLocal,
	}
	validationErr := validator.ValidationError{}
	if _, err = model.LoadProjectInto(ctx, v.input.ProjectYaml, opts, v.input.ProjectID, project); err != nil {
		validationErr.Message = err.Error()
		return gimlet.NewJSONErrorResponse(validator.ValidationErrors{validationErr})
	}
	if projectConfig, err = model.CreateProjectConfig(v.input.ProjectYaml, ""); err != nil {
		validationErr.Message = err.Error()
		gimlet.NewJSONErrorResponse(validator.ValidationErrors{validationErr})
	}

	projectRef, err := model.FindMergedProjectRef(ctx, v.input.ProjectID, "", false)
	errs := validator.CheckProject(ctx, project, projectConfig, projectRef, v.input.ProjectID, err)

	if v.input.Quiet {
		errs = errs.AtLevel(validator.Error)
	}
	if len(errs) > 0 {
		return gimlet.NewJSONErrorResponse(errs)

	}
	return gimlet.NewJSONResponse(validator.ValidationErrors{})
}
