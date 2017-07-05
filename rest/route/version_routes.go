package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

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
				MethodType:     evergreen.MethodGet,
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
	vh.versionId = mux.Vars(r)["version_id"]

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
				MethodType:     evergreen.MethodGet,
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
	h.versionId = mux.Vars(r)["version_id"]

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
