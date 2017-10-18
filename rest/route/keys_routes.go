package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
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

	userKeys := make([]model.Model, len(user.PubKeys))
	for _, key := range user.PubKeys {
		apiKey := &model.APIPubKey{}
		err := apiKey.BuildFromService(key)
		if err != nil {
			return ResponseData{}, errors.Wrap(err, "error marshalling public key to api")
		}

		userKeys = append(userKeys, apiKey)
	}

	return ResponseData{
		Result: userKeys,
	}, nil
}
