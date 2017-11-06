package route

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// XXX: If you are changing the validation in this function, you must also
// update the BASE64REGEX in directives.spawn.js
const keyRegex = "^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{4})$"

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
	h.keyName = string(key.Name)
	if err := validateKeyName(h.keyName); err != nil {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("invalid public key name: %s", err),
		}
	}

	h.keyValue = string(key.Key)
	if err := validateKeyValue(h.keyValue); err != nil {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}

	return nil
}

// XXX: If you are changing the validation in either validateKey* function,
// you must also update keyBaseValid in directives.spawn.js
func validateKeyName(keyName string) error {
	if strings.TrimSpace(keyName) == "" {
		return errors.New("empty key name")
	}

	return nil
}

func validateKeyValue(keyValue string) error {
	if !strings.HasPrefix(keyValue, "ssh-rsa") && !strings.HasPrefix(keyValue, "ssh-dss") {
		return errors.New("invalid public key")
	}

	splitKey := strings.Split(keyValue, " ")
	if len(splitKey) < 2 {
		return errors.New("invalid public key")
	}

	matched, err := regexp.MatchString(keyRegex, splitKey[1])
	if err != nil {
		return errors.Wrap(err, "invalid public key")
	} else if !matched {
		return errors.New("invalid public key: key contents invalid")
	}

	return nil
}

func (h *keysPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)

	if _, err := u.GetPublicKey(h.keyName); err == nil {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "duplicate key name",
		}
	}

	if err := sc.AddPublicKey(u, h.keyName, h.keyValue); err != nil {
		return ResponseData{}, errors.Wrap(err, "failed to add key")
	}

	return ResponseData{}, nil
}
