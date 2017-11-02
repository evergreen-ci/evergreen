package route

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
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
			MethodHandler{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &keysPostHandler{},
				MethodType:        http.MethodPost,
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
	for i, key := range user.PubKeys {
		apiKey := &model.APIPubKey{}
		err := apiKey.BuildFromService(key)
		if err != nil {
			return ResponseData{}, errors.Wrap(err, "error marshalling public key to api")
		}

		userKeys[i] = apiKey
	}

	return ResponseData{
		Result: userKeys,
	}, nil
}

type keysPostHandler struct {
	keyName  string
	keyValue string
}

func (h *keysPostHandler) Handler() RequestHandler {
	return &keysPostHandler{}
}

func (h *keysPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	body := util.NewRequestReader(r)
	defer body.Close()

	key := model.APIPubKey{}
	if err := util.ReadJSONInto(body, &key); err != nil {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("failed to unmarshal public key: %s", err),
		}
	}

	if err := h.validatePublicKey(key); err != nil {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("invalid public key: %s", err),
		}
	}

	return nil
}

func (h *keysPostHandler) validatePublicKey(key model.APIPubKey) error {
	h.keyName = string(key.Name)
	h.keyValue = string(key.Key)
	if strings.TrimSpace(h.keyName) == "" || strings.TrimSpace(h.keyValue) == "" {
		return errors.New("empty key or value")
	}

	return nil
}

func (h *keysPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)

	if err := sc.AddPublicKey(u, h.keyName, h.keyValue); err != nil {
		return ResponseData{}, errors.Wrap(err, "failed to add key")
	}

	return ResponseData{}, nil
}
