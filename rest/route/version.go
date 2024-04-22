package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
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

// Factory creates an instance of the handler.
//
//	@Summary		Fetch version by ID
//	@Description	Fetches a single version using its ID
//	@Tags			versions
//	@Router			/versions/{version_id} [get]
//	@Security		Api-User || Api-Key
//	@Param			version_id	path		string	true	"version ID"
//	@Success		200			{object}	model.APIVersion
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
	versionModel.BuildFromService(*foundVersion)
	return gimlet.NewJSONResponse(versionModel)
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/versions/{version_id}

type versionPatchHandler struct {
	// Will activate the version if true and deactivate if false.
	Activated *bool `json:"activated" validate:"required"`

	versionId string
}

func makePatchVersion() gimlet.RouteHandler {
	return &versionPatchHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Activate or deactivate a version
//	@Description	Activate or deactivates a given version. Does not return the version.
//	@Tags			versions
//	@Router			/versions/{version_id} [patch]
//	@Security		Api-User || Api-Key
//	@Param			version_id	path	string				true	"the version ID"
//	@Param			{object}	body	versionPatchHandler	true	"parameters"
//	@Success		200
func (vh *versionPatchHandler) Factory() gimlet.RouteHandler {
	return &versionPatchHandler{}
}

// Parse fetches the versionId from the http request.
func (vh *versionPatchHandler) Parse(ctx context.Context, r *http.Request) error {
	if err := utility.ReadJSON(r.Body, vh); err != nil {
		return errors.Wrap(err, "reading body")
	}
	if vh.Activated == nil {
		return gimlet.ErrorResponse{
			Message:    "Must set 'activated'",
			StatusCode: http.StatusBadRequest,
		}
	}

	vh.versionId = gimlet.GetVars(r)["version_id"]
	if vh.versionId == "" {
		return errors.New("missing version id")
	}
	return nil
}

// Run calls the data model.SetVersionActivation function
func (vh *versionPatchHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	if err := dbModel.SetVersionActivation(ctx, vh.versionId, utility.FromBoolPtr(vh.Activated), u.Id); err != nil {
		state := "inactive"
		if utility.FromBoolPtr(vh.Activated) {
			state = "active"
		}
		return gimlet.MakeJSONErrorResponder(
			errors.Wrapf(err, "marking version '%v' as '%v'", vh.versionId, state),
		)
	}
	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/versions/{version_id}/builds

// buildsForVersionHandler is a RequestHandler for fetching all builds for a version
type buildsForVersionHandler struct {
	versionId       string
	variant         string
	includeTaskInfo bool
	env             evergreen.Environment
}

func makeGetVersionBuilds(env evergreen.Environment) gimlet.RouteHandler {
	return &buildsForVersionHandler{env: env}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get builds from a version
//	@Description	Fetches a list of builds associated with a version
//	@Tags			builds
//	@Router			/versions/{version_id}/builds [get]
//	@Security		Api-User || Api-Key
//	@Param			version_id			path	string	true	"the version ID"
//	@Param			variant				query	string	false	"Only return the build with this variant (using Distro identifier)."
//	@Param			include_task_info	query	string	false	"if set, include additional information about tasks in each build (this is expensive)"
//	@Success		200					{array}	model.APIBuild
func (h *buildsForVersionHandler) Factory() gimlet.RouteHandler {
	return &buildsForVersionHandler{env: h.env}
}

// ParseAndValidate fetches the versionId from the http request.
func (h *buildsForVersionHandler) Parse(ctx context.Context, r *http.Request) error {
	h.versionId = gimlet.GetVars(r)["version_id"]

	if h.versionId == "" {
		return errors.New("missing version ID")
	}
	vars := r.URL.Query()
	h.variant = vars.Get("variant")
	includeTaskInfoStr := vars.Get("include_task_info")
	if includeTaskInfoStr != "" {
		includeTaskInfo, err := strconv.ParseBool(includeTaskInfoStr)
		if err != nil {
			return errors.Wrap(err, "invalid include_task_info parameter")
		}
		h.includeTaskInfo = includeTaskInfo
	}
	return nil
}

// Run returns the variants for a version, filtered by variant if specified.
func (h *buildsForVersionHandler) Run(ctx context.Context) gimlet.Responder {
	var builds []build.Build
	var err error

	if h.variant == "" {
		builds, err = build.Find(build.ByVersion(h.versionId))
	} else {
		builds, err = build.Find(build.ByVersionAndVariant(h.versionId, h.variant))
	}
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "getting builds"))
	}

	v, err := dbModel.VersionFindOneId(h.versionId)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "getting version '%s'", h.versionId))
	}
	var pp *dbModel.ParserProject
	if v != nil {
		pp, err = dbModel.ParserProjectFindOneByID(ctx, h.env.Settings(), v.ProjectStorageMethod, v.Id)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting project info"))
		}
	}

	buildModels := []model.APIBuild{}
	for _, b := range builds {
		buildModel := model.APIBuild{}
		buildModel.BuildFromService(b, pp)
		if h.includeTaskInfo {
			if err := setBuildTaskCache(&b, &buildModel); err != nil {
				return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "setting task cache for build '%s'", b.Id))
			}
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

// Factory creates an instance of the handler.
//
//	@Summary		Abort a version
//	@Description	Aborts a single version using its ID then returns the version
//	@Tags			versions
//	@Router			/versions/{version_id}/abort [post]
//	@Security		Api-User || Api-Key
//	@Param			version_id	path		string	true	"version ID"
//	@Success		200			{object}	model.APIVersion
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

// Execute calls the data AbortVersionTasks function to abort all tasks of a version.
func (h *versionAbortHandler) Run(ctx context.Context) gimlet.Responder {
	if err := task.AbortVersionTasks(h.versionId, task.AbortInfo{User: h.userId}); err != nil {
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
	versionModel.BuildFromService(*foundVersion)
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

// Factory creates an instance of the handler.
//
//	@Summary		Restart a version
//	@Description	Restarts a single version using its ID then returns the version
//	@Tags			versions
//	@Router			/versions/{version_id}/restart [post]
//	@Security		Api-User || Api-Key
//	@Param			version_id	path		string	true	"version ID"
//	@Success		200			{object}	model.APIVersion
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
	err := dbModel.RestartVersion(ctx, h.versionId, nil, true, MustHaveUser(ctx).Id)
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
	versionModel.BuildFromService(*foundVersion)
	return gimlet.NewJSONResponse(versionModel)
}
