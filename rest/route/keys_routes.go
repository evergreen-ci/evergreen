package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
)

type keysGetHandler struct{}

func getKeysRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			MethodHandler{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &keysGetHandler{},
				MethodType:        http.MethodGet,
			},
		},
		Version: version,
	}
}

func (h *keysGetHandler) Handler() RequestHandler {
	return &keysGetHandler{}
}

func (h *keysGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *keysGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	user := MustHaveUser(ctx)

	keysModel := make([]model.Model, len(user.PubKeys))
	for i, key := range user.PubKeys {
		apiKey := &model.APIPubKey{}
		err := apiKey.BuildFromService(key)
		if err != nil {
			return ResponseData{}, err
		}

		keysModel[i] = apiKey
	}

	return ResponseData{
		Result: keysModel,
	}, nil
}
