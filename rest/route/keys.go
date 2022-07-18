package route

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// XXX: If you are changing the validation in this function, you must also
// update the BASE64REGEX in directives.spawn.js
const keyRegex = "^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{4})$"

type keysGetHandler struct{}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/keys

func makeFetchKeys() gimlet.RouteHandler {
	return &keysGetHandler{}
}

func (h *keysGetHandler) Factory() gimlet.RouteHandler                     { return &keysGetHandler{} }
func (h *keysGetHandler) Parse(ctx context.Context, r *http.Request) error { return nil }

func (h *keysGetHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	resp := gimlet.NewResponseBuilder()

	for _, key := range user.PubKeys {
		apiKey := &model.APIPubKey{}
		apiKey.BuildFromService(key)
		if err := resp.AddData(apiKey); err != nil {
			return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "adding public keys to response"))
		}
	}

	return resp
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/keys

type keysPostHandler struct {
	keyName  string
	keyValue string
}

func makeSetKey() gimlet.RouteHandler {
	return &keysPostHandler{}
}

func (h *keysPostHandler) Factory() gimlet.RouteHandler {
	return &keysPostHandler{}
}

func (h *keysPostHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()

	key := model.APIPubKey{}
	if err := utility.ReadJSON(body, &key); err != nil {
		return errors.Wrap(err, "reading public key from JSON request body")
	}
	h.keyName = utility.FromStringPtr(key.Name)
	if err := validateKeyName(h.keyName); err != nil {
		return errors.Wrap(err, "invalid public key name")
	}

	h.keyValue = utility.FromStringPtr(key.Key)
	if err := validateKeyValue(h.keyValue); err != nil {
		return errors.Wrap(err, "invalid public key")
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
	if err := evergreen.ValidateSSHKey(keyValue); err != nil {
		return errors.Wrapf(err, "invalid public key")
	}

	splitKey := strings.Split(keyValue, " ")
	if len(splitKey) < 2 {
		return errors.New("invalid public key")
	}

	matched, err := regexp.MatchString(keyRegex, splitKey[1])
	if err != nil {
		return errors.Wrap(err, "invalid public key")
	} else if !matched {
		return errors.New("public key contents are invalid")
	}

	return nil
}

func (h *keysPostHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	if _, err := u.GetPublicKey(h.keyName); err == nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Errorf("public key '%s' already exists for user '%s'", h.keyName, u.Username()))
	}

	if err := u.AddPublicKey(h.keyName, h.keyValue); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "adding public key"))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// DELETE /rest/v2/keys/{key_name}

type keysDeleteHandler struct {
	keyName string
}

func makeDeleteKeys() gimlet.RouteHandler {
	return &keysDeleteHandler{}
}

func (h *keysDeleteHandler) Factory() gimlet.RouteHandler {
	return &keysDeleteHandler{}
}

func (h *keysDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.keyName = gimlet.GetVars(r)["key_name"]
	if strings.TrimSpace(h.keyName) == "" {
		return errors.New("public key name cannot be empty")
	}

	return nil
}

func (h *keysDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	if _, err := user.GetPublicKey(h.keyName); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("public key '%s' not found", h.keyName))
	}

	if err := user.DeletePublicKey(h.keyName); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "deleting public key '%s'", h.keyName))
	}

	return gimlet.NewJSONResponse(struct{}{})
}
