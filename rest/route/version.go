package route

import (
	"context"
	"fmt"
	"net/http"

	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/versions/{version_id}

type versionHandler struct {
	versionId string
}

func makeGetVersionByID() gimlet.RouteHandler {
	return &versionHandler{}
}

// Handler returns a pointer to a new versionHandler.
func (vh *versionHandler) Factory() gimlet.RouteHandler {
	return &versionHandler{}
}

// ParseAndValidate fetches the versionId from the http request.
func (vh *versionHandler) Parse(ctx context.Context, r *http.Request) error {
	vh.versionId = gimlet.GetVars(r)["version_id"]

	if vh.versionId == "" {
		return errors.New("missing version ID")
	}

	return nil
}

// Execute calls the data model.VersionFindOneId function and returns the version
// from the provider.
func (vh *versionHandler) Run(ctx context.Context) gimlet.Responder {
	foundVersion, err := dbModel.VersionFindOneId(vh.versionId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding version '%s'", vh.versionId))
	}
	if foundVersion == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version '%s' not found", vh.versionId),
		})
	}

	versionModel := &model.APIVersion{}

	if err = versionModel.BuildFromService(foundVersion); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting version '%s' to API model", foundVersion.Id))
	}
	return gimlet.NewJSONResponse(versionModel)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/versions/{version_id}/builds

// buildsForVersionHandler is a RequestHandler for fetching all builds for a version
type buildsForVersionHandler struct {
	versionId string
	variant   string
}

func makeGetVersionBuilds() gimlet.RouteHandler {
	return &buildsForVersionHandler{}
}

// Handler returns a pointer to a new buildsForVersionHandler.
func (h *buildsForVersionHandler) Factory() gimlet.RouteHandler {
	return &buildsForVersionHandler{}
}

// ParseAndValidate fetches the versionId from the http request.
func (h *buildsForVersionHandler) Parse(ctx context.Context, r *http.Request) error {
	h.versionId = gimlet.GetVars(r)["version_id"]

	if h.versionId == "" {
		return errors.New("missing version ID")
	}
	vars := r.URL.Query()
	h.variant = vars.Get("variant")
	return nil
}

// Execute calls the model.VersionFindOneId function to find the version by its ID, calls build.FindOneId for each
// build variant for the version, and returns the data.
func (h *buildsForVersionHandler) Run(ctx context.Context) gimlet.Responder {
	// First, find the version by its ID.
	foundVersion, err := dbModel.VersionFindOneId(h.versionId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding version '%s'", h.versionId))
	}
	if foundVersion == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version '%s' not found", h.versionId),
		})
	}

	// Then, find each build variant in the found version by its ID.
	buildModels := []model.Model{}
	for _, buildStatus := range foundVersion.BuildVariants {
		// If a variant was specified, only retrieve that variant
		if h.variant != "" && buildStatus.BuildVariant != h.variant {
			continue
		}
		foundBuild, err := build.FindOneId(buildStatus.BuildId)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding build '%s'", buildStatus.BuildId))
		}
		if foundBuild == nil {
			return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("build '%s' not found", buildStatus.BuildId),
			})
		}
		buildModel := &model.APIBuild{}
		err = buildModel.BuildFromService(*foundBuild)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting build '%s' to API model", foundBuild.Id))
		}

		buildModels = append(buildModels, buildModel)
	}
	return gimlet.NewJSONResponse(buildModels)
}

// versionAbortHandler is a RequestHandler for aborting all tasks of a version.
type versionAbortHandler struct {
	versionId string
	userId    string
}

func makeAbortVersion() gimlet.RouteHandler {
	return &versionAbortHandler{}
}

// Handler returns a pointer to a new versionAbortHandler.
func (h *versionAbortHandler) Factory() gimlet.RouteHandler {
	return &versionAbortHandler{}
}

// ParseAndValidate fetches the versionId from the http request.
func (h *versionAbortHandler) Parse(ctx context.Context, r *http.Request) error {
	h.versionId = gimlet.GetVars(r)["version_id"]

	if h.versionId == "" {
		return errors.New("missing version ID")
	}

	if u := gimlet.GetUser(ctx); u != nil {
		h.userId = u.Username()
	}

	return nil
}

// Execute calls the data AbortVersion function to abort all tasks of a version.
func (h *versionAbortHandler) Run(ctx context.Context) gimlet.Responder {
	if err := task.AbortVersion(h.versionId, task.AbortInfo{User: h.userId}); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "aborting version '%s'", h.versionId))
	}

	foundVersion, err := dbModel.VersionFindOneId(h.versionId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding version '%s'", h.versionId))
	}
	if foundVersion == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version '%s' not found", h.versionId),
		})
	}

	versionModel := &model.APIVersion{}
	if err = versionModel.BuildFromService(foundVersion); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting version '%s' to API model", foundVersion.Id))
	}

	return gimlet.NewJSONResponse(versionModel)
}

// versionRestartHandler is a RequestHandler for restarting all completed tasks
// of a version.
type versionRestartHandler struct {
	versionId string
}

func makeRestartVersion() gimlet.RouteHandler {
	return &versionRestartHandler{}
}

// Handler returns a pointer to a new versionRestartHandler.
func (h *versionRestartHandler) Factory() gimlet.RouteHandler {
	return &versionRestartHandler{}
}

// ParseAndValidate fetches the versionId from the http request.
func (h *versionRestartHandler) Parse(ctx context.Context, r *http.Request) error {
	h.versionId = gimlet.GetVars(r)["version_id"]

	if h.versionId == "" {
		return errors.New("missing version ID")
	}

	return nil
}

// Execute calls the data RestartVersion function to restart completed tasks of a version.
func (h *versionRestartHandler) Run(ctx context.Context) gimlet.Responder {
	// RestartAction the version
	err := dbModel.RestartTasksInVersion(h.versionId, true, MustHaveUser(ctx).Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "restarting tasks in version '%s'", h.versionId))
	}

	// Find the version to return updated status.
	foundVersion, err := dbModel.VersionFindOneId(h.versionId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding version '%s'", h.versionId))
	}
	if foundVersion == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version '%s' not found", h.versionId),
		})
	}

	versionModel := &model.APIVersion{}
	err = versionModel.BuildFromService(foundVersion)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting version '%s' to API model", foundVersion.Id))
	}

	return gimlet.NewJSONResponse(versionModel)
}
