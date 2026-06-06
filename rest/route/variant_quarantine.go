package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// projectFromContext returns the project _id and identifier from the
// middleware-loaded context, falling back to _id when no identifier is set.
func projectFromContext(ctx context.Context) (projectID, projectIdentifier string, err error) {
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.ProjectRef == nil {
		return "", "", gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "project not found",
		}
	}
	identifier := projCtx.ProjectRef.Identifier
	if identifier == "" {
		identifier = projCtx.ProjectRef.Id
	}
	return projCtx.ProjectRef.Id, identifier, nil
}

func buildVariantQuarantineResponse(ctx context.Context, projectID, projectIdentifier, variantName string) gimlet.Responder {
	tasks, err := data.GetVariantQuarantineStatus(ctx, projectID, variantName)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting variant quarantine status for '%s' on project '%s'", variantName, projectIdentifier))
	}
	apiStatus := &model.APIVariantQuarantineStatus{}
	apiStatus.BuildFromService(model.VariantQuarantineStatusBuildArgs{
		ProjectIdentifier: projectIdentifier,
		BuildVariant:      variantName,
		Tasks:             tasks,
	})
	return gimlet.NewJSONResponse(apiStatus)
}

func quarantineVariant(ctx context.Context, projectID, projectIdentifier, variantName string, isManuallyQuarantined bool) gimlet.Responder {
	u := MustHaveUser(ctx)
	if err := data.SetVariantQuarantined(ctx, projectID, variantName, isManuallyQuarantined); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting quarantine state to '%t' for build variant '%s' on project '%s'", isManuallyQuarantined, variantName, projectIdentifier))
	}
	grip.Info(ctx, message.Fields{
		"message":                 "build variant quarantine state changed",
		"user":                    u.Username(),
		"project":                 projectID,
		"project_identifier":      projectIdentifier,
		"build_variant":           variantName,
		"is_manually_quarantined": isManuallyQuarantined,
	})
	return buildVariantQuarantineResponse(ctx, projectID, projectIdentifier, variantName)
}

type variantQuarantineHandler struct {
	variantName string
}

func makeVariantQuarantineHandler() gimlet.RouteHandler {
	return &variantQuarantineHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Quarantine a build variant
//	@Description	Marks every known test of every known task in the given build variant as manually quarantined in the test selection service.
//	@Tags			projects
//	@Router			/projects/{project_id}/variants/{variant_name}/quarantine [post]
//	@Security		Api-User || Api-Key
//	@Param			project_id		path		string	true	"the project ID or identifier"
//	@Param			variant_name	path		string	true	"the build variant name"
//	@Success		200				{object}	model.APIVariantQuarantineStatus
func (h *variantQuarantineHandler) Factory() gimlet.RouteHandler {
	return &variantQuarantineHandler{}
}

func (h *variantQuarantineHandler) Parse(ctx context.Context, r *http.Request) error {
	h.variantName = gimlet.GetVars(r)["variant_name"]
	return nil
}

func (h *variantQuarantineHandler) Run(ctx context.Context) gimlet.Responder {
	projectID, projectIdentifier, err := projectFromContext(ctx)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return quarantineVariant(ctx, projectID, projectIdentifier, h.variantName, true)
}

type variantUnquarantineHandler struct {
	variantName string
}

func makeVariantUnquarantineHandler() gimlet.RouteHandler {
	return &variantUnquarantineHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Unquarantine a build variant
//	@Description	Marks every known test of every known task in the given build variant as no longer manually quarantined in the test selection service.
//	@Tags			projects
//	@Router			/projects/{project_id}/variants/{variant_name}/unquarantine [post]
//	@Security		Api-User || Api-Key
//	@Param			project_id		path		string	true	"the project ID or identifier"
//	@Param			variant_name	path		string	true	"the build variant name"
//	@Success		200				{object}	model.APIVariantQuarantineStatus
func (h *variantUnquarantineHandler) Factory() gimlet.RouteHandler {
	return &variantUnquarantineHandler{}
}

func (h *variantUnquarantineHandler) Parse(ctx context.Context, r *http.Request) error {
	h.variantName = gimlet.GetVars(r)["variant_name"]
	return nil
}

func (h *variantUnquarantineHandler) Run(ctx context.Context) gimlet.Responder {
	projectID, projectIdentifier, err := projectFromContext(ctx)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return quarantineVariant(ctx, projectID, projectIdentifier, h.variantName, false)
}

type variantQuarantineStatusHandler struct {
	variantName string
}

func makeVariantQuarantineStatusHandler() gimlet.RouteHandler {
	return &variantQuarantineStatusHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get build variant quarantine status
//	@Description	Returns the current manual-quarantine state for every known test of every known task in the given build variant.
//	@Tags			projects
//	@Router			/projects/{project_id}/variants/{variant_name}/quarantine_status [get]
//	@Security		Api-User || Api-Key
//	@Param			project_id		path		string	true	"the project ID or identifier"
//	@Param			variant_name	path		string	true	"the build variant name"
//	@Success		200				{object}	model.APIVariantQuarantineStatus
func (h *variantQuarantineStatusHandler) Factory() gimlet.RouteHandler {
	return &variantQuarantineStatusHandler{}
}

func (h *variantQuarantineStatusHandler) Parse(ctx context.Context, r *http.Request) error {
	h.variantName = gimlet.GetVars(r)["variant_name"]
	return nil
}

func (h *variantQuarantineStatusHandler) Run(ctx context.Context) gimlet.Responder {
	projectID, projectIdentifier, err := projectFromContext(ctx)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return buildVariantQuarantineResponse(ctx, projectID, projectIdentifier, h.variantName)
}
