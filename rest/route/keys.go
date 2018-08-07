package route

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// XXX: If you are changing the validation in this function, you must also
// update the BASE64REGEX in directives.spawn.js
const keyRegex = "^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{4})$"

type keysGetHandler struct {
	sc data.Connector
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/keys

func makeFetchKeys(sc data.Connector) gimlet.RouteHandler {
	return &keysGetHandler{
		sc: sc,
	}
}

func (h *keysGetHandler) Factory() gimlet.RouteHandler                     { return &keysGetHandler{sc: h.sc} }
func (h *keysGetHandler) Parse(ctx context.Context, r *http.Request) error { return nil }

func (h *keysGetHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	resp := gimlet.NewResponseBuilder()
	if err := resp.SetStatus(http.StatusOK); err != nil {
		return gimlet.NewJSONErrorResponse(err)
	}

	if err := resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.NewJSONErrorResponse(err)
	}

	for _, key := range user.PubKeys {
		apiKey := &model.APIPubKey{}
		if err := apiKey.BuildFromService(key); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error marshalling public key to api"))
		}
		if err := resp.AddData(apiKey); err != nil {
			return gimlet.NewJSONResponse(err)
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
	sc       data.Connector
}

func makeSetKey(sc data.Connector) gimlet.RouteHandler {
	return &keysPostHandler{
		sc: sc,
	}
}

func (h *keysPostHandler) Factory() gimlet.RouteHandler {
	return &keysPostHandler{sc: h.sc}
}

func (h *keysPostHandler) Parse(ctx context.Context, r *http.Request) error {
	body := util.NewRequestReader(r)
	defer body.Close()

	key := model.APIPubKey{}
	if err := util.ReadJSONInto(body, &key); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("failed to unmarshal public key: %s", err),
		}
	}
	h.keyName = model.FromAPIString(key.Name)
	if err := validateKeyName(h.keyName); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("invalid public key name: %s", err),
		}
	}

	h.keyValue = model.FromAPIString(key.Key)
	if err := validateKeyValue(h.keyValue); err != nil {
		return gimlet.ErrorResponse{
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

func (h *keysPostHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	if _, err := u.GetPublicKey(h.keyName); err == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "a public key with this name already exists for user",
		})
	}

	if err := h.sc.AddPublicKey(u, h.keyName, h.keyValue); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "failed to add key"))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// DELETE /rest/v2/keys/{key_name}

type keysDeleteHandler struct {
	keyName string
	sc      data.Connector
}

func makeDeleteKeys(sc data.Connector) gimlet.RouteHandler {
	return &keysDeleteHandler{
		sc: sc,
	}
}

func (h *keysDeleteHandler) Factory() gimlet.RouteHandler {
	return &keysDeleteHandler{sc: h.sc}
}

func (h *keysDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.keyName = gimlet.GetVars(r)["key_name"]
	if strings.TrimSpace(h.keyName) == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "empty key name",
		}
	}

	return nil
}

func (h *keysDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	if _, err := user.GetPublicKey(h.keyName); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("key with name '%s' does not exist", h.keyName),
		})
	}

	if err := h.sc.DeletePublicKey(user, h.keyName); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.New("couldn't delete key"))
	}

	return gimlet.NewJSONResponse(struct{}{})
}
