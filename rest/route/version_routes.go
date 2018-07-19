package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// getVersionIdFromRequest is a helpfer function that fetches versionId from
// request.
func getVersionIdFromRequest(r *http.Request) string {
	return gimlet.GetVars(r)["version_id"]
}

type versionHandler struct {
	versionId string
}

func getVersionIdRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &versionHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

// Handler returns a pointer to a new versionHandler.
func (vh *versionHandler) Handler() RequestHandler {
	return &versionHandler{}
}

// ParseAndValidate fetches the versionId from the http request.
func (vh *versionHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vh.versionId = getVersionIdFromRequest(r)

	if vh.versionId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

// Execute calls the data FindVersionById function and returns the version
// from the provider.
func (vh *versionHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundVersion, err := sc.FindVersionById(vh.versionId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}

	versionModel := &model.APIVersion{}
	err = versionModel.BuildFromService(foundVersion)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}
	return ResponseData{
		Result: []model.Model{versionModel},
	}, nil
}

// buildsForVersionHandler is a RequestHandler for fetching all builds for a version
type buildsForVersionHandler struct {
	versionId string
}

func getBuildsForVersionRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &buildsForVersionHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

// Handler returns a pointer to a new buildsForVersionHandler.
func (h *buildsForVersionHandler) Handler() RequestHandler {
	return &buildsForVersionHandler{}
}

// ParseAndValidate fetches the versionId from the http request.
func (h *buildsForVersionHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	h.versionId = getVersionIdFromRequest(r)

	if h.versionId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

// Execute calls the FindVersionById function to find the version by its ID, calls FindBuildById for each
// build variant for the version, and returns the data.
func (h *buildsForVersionHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	// First, find the version by its ID.
	foundVersion, err := sc.FindVersionById(h.versionId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error in finding the version")
	}

	// Then, find each build variant in the found version by its ID.
	buildModels := []model.Model{}
	for _, buildStatus := range foundVersion.BuildVariants {
		foundBuild, err := sc.FindBuildById(buildStatus.BuildId)
		if err != nil {
			return ResponseData{}, errors.Wrap(err, "Database error in finding the build")
		}
		buildModel := &model.APIBuild{}
		err = buildModel.BuildFromService(*foundBuild)
		if err != nil {
			return ResponseData{}, errors.Wrap(err, "API model error")
		}

		buildModels = append(buildModels, buildModel)
	}
	return ResponseData{
		Result: buildModels,
	}, nil
}

// versionAbortHandler is a RequestHandler for aborting all tasks of a version.
type versionAbortHandler struct {
	versionId string
}

func getAbortVersionRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: &versionAbortHandler{},
				MethodType:     http.MethodPost,
			},
		},
		Version: version,
	}
}

// Handler returns a pointer to a new versionAbortHandler.
func (h *versionAbortHandler) Handler() RequestHandler {
	return &versionAbortHandler{}
}

// ParseAndValidate fetches the versionId from the http request.
func (h *versionAbortHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	h.versionId = getVersionIdFromRequest(r)

	if h.versionId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

// Execute calls the data AbortVersion function to abort all tasks of a version.
func (h *versionAbortHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	var userId string

	if u := gimlet.GetUser(ctx); u != nil {
		userId = u.Username()
	}
	err := sc.AbortVersion(h.versionId, userId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error in aborting version:")
	}

	foundVersion, err := sc.FindVersionById(h.versionId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error in finding version:")
	}

	versionModel := &model.APIVersion{}
	err = versionModel.BuildFromService(foundVersion)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}

	return ResponseData{
		Result: []model.Model{versionModel},
	}, err
}

// versionRestartHandler is a RequestHandler for restarting all completed tasks
// of a version.
type versionRestartHandler struct {
	versionId string
}

func getRestartVersionRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: &versionRestartHandler{},
				MethodType:     http.MethodPost,
			},
		},
		Version: version,
	}
}

// Handler returns a pointer to a new versionRestartHandler.
func (h *versionRestartHandler) Handler() RequestHandler {
	return &versionRestartHandler{}
}

// ParseAndValidate fetches the versionId from the http request.
func (h *versionRestartHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	h.versionId = getVersionIdFromRequest(r)

	if h.versionId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

// Execute calls the data RestartVersion function to restart completed tasks of a version.
func (h *versionRestartHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	// Restart the version
	err := sc.RestartVersion(h.versionId, MustHaveUser(ctx).Id)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error in restarting version:")
	}

	// Find the version to return updated status.
	foundVersion, err := sc.FindVersionById(h.versionId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error in finding version:")
	}

	versionModel := &model.APIVersion{}
	err = versionModel.BuildFromService(foundVersion)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}

	return ResponseData{
		Result: []model.Model{versionModel},
	}, err
}
