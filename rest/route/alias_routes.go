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

type aliasGetHandler struct {
	name string
}

func getAliasRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &aliasGetHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (a *aliasGetHandler) Handler() RequestHandler {
	return &aliasGetHandler{}
}

func (a *aliasGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	a.name = vars["name"]
	return nil
}

func (a *aliasGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	aliases, err := sc.FindProjectAliases(a.name)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	models := make([]model.Model, len(aliases))
	for i, a := range aliases {
		aliasModel := &model.APIAlias{}
		if err := aliasModel.BuildFromService(a); err != nil {
			return ResponseData{}, err
		}
		models[i] = aliasModel
	}

	return ResponseData{
		Result: models,
	}, nil
}
