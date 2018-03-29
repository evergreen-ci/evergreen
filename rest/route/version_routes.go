package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

// getVersionIdFromRequest is a helpfer function that fetches versionId from
// request.
func getVersionIdFromRequest(r *http.Request) string {
	return mux.Vars(r)["version_id"]
}

// getProjectIdFromRequest is a helpfer function
// that fetches projectId from request.
func getProjectIdFromRequest(r *http.Request) string {
	return mux.Vars(r)["project_id"]
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
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	versionModel := &model.APIVersion{}
	err = versionModel.BuildFromService(foundVersion)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{versionModel},
	}, nil
}

// activeVersionsHandler is a RequestHandler for fetchhing
// all active versions for given projectId
type activeVersionsHandler struct {
	projectId string
}

func getActiveVersionsRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &activeVersionsHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

// Handler returns a pointer to a new activeVersionsHandler.
func (avh *activeVersionsHandler) Handler() RequestHandler {
	return &activeVersionsHandler{}
}

// ParseAndValidate fetches the versionId from the http request.
func (avh *activeVersionsHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	avh.projectId = getProjectIdFromRequest(r)

	if avh.projectId == "" {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Missing projectId",
		}
	}

	return nil
}

// Execute calls the data FindActivatedVersionsByProjectId
// function and returns the version from the provider.
func (avh *activeVersionsHandler) Execute(
	ctx context.Context,
	sc data.Connector,
) (ResponseData, error) {
	versions, err := sc.FindActivatedVersionsByProjectId(avh.projectId)

	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	versionModels := []model.Model{}

	for _, version := range versions {
		versionModel := &model.APIVersion{}
		err = versionModel.BuildFromService(&version)

		if err != nil {
			if _, ok := err.(*rest.APIError); !ok {
				err = errors.Wrap(err, "API model error")
			}
			return ResponseData{}, err
		}

		versionModels = append(versionModels, versionModel)
	}

	return ResponseData{
		Result: versionModels,
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
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error in finding the version")
		}
		return ResponseData{}, err
	}

	// Then, find each build variant in the found version by its ID.
	buildModels := []model.Model{}
	for _, buildStatus := range foundVersion.BuildVariants {
		foundBuild, err := sc.FindBuildById(buildStatus.BuildId)
		if err != nil {
			if _, ok := err.(*rest.APIError); !ok {
				err = errors.Wrap(err, "Database error in finding the build")
			}
			return ResponseData{}, err
		}
		buildModel := &model.APIBuild{}
		err = buildModel.BuildFromService(*foundBuild)
		if err != nil {
			if _, ok := err.(*rest.APIError); !ok {
				err = errors.Wrap(err, "API model error")
			}
			return ResponseData{}, err
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
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &versionAbortHandler{},
				MethodType:        http.MethodPost,
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
	err := sc.AbortVersion(h.versionId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error in aborting version:")
		}
		return ResponseData{}, err
	}

	foundVersion, err := sc.FindVersionById(h.versionId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error in finding version:")
		}
		return ResponseData{}, err
	}

	versionModel := &model.APIVersion{}
	err = versionModel.BuildFromService(foundVersion)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
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
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &versionRestartHandler{},
				MethodType:        http.MethodPost,
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
	err := sc.RestartVersion(h.versionId, GetUser(ctx).Id)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error in restarting version:")
		}
		return ResponseData{}, err
	}

	// Find the version to return updated status.
	foundVersion, err := sc.FindVersionById(h.versionId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error in finding version:")
		}
		return ResponseData{}, err
	}

	versionModel := &model.APIVersion{}
	err = versionModel.BuildFromService(foundVersion)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{versionModel},
	}, err
}
