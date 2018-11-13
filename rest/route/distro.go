package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

///////////////////////////////////////////////////////////////////////
//
// PUT /rest/v2/distros/{distro_id}

type distroPutHandler struct {
	distroId string
	body     []byte
	sc       data.Connector
}

func makePutDistro(sc data.Connector) gimlet.RouteHandler {
	return &distroPutHandler{
		sc: sc,
	}
}

func (h *distroPutHandler) Factory() gimlet.RouteHandler {
	return &distroPutHandler{
		sc: h.sc,
	}
}

// Parse fetches the distroId and json payload from the http request.
func (h *distroPutHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroId = gimlet.GetVars(r)["distro_id"]

	body := util.NewRequestReader(r)
	defer body.Close()
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	h.body = b

	return nil
}

// Run either:
// (a) replaces an existing resource with the entity defined in the JSON payload, or
// (b) creates a new resource based on the Request-URI and JSON payload
func (h *distroPutHandler) Run(ctx context.Context) gimlet.Responder {
	original, err := h.sc.FindDistroById(h.distroId)
	if err != nil && err.(gimlet.ErrorResponse).StatusCode != http.StatusNotFound {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", h.distroId))
	}

	apiDistro := &model.APIDistro{Name: model.ToAPIString(h.distroId)}
	if err := json.Unmarshal(h.body, apiDistro); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error while unmarshalling JSON"))
	}

	distro, error := validateToService(ctx, apiDistro, h.distroId, false)
	if error != nil {
		return error
	}

	if original != nil {
		// Existing resource
		if err := h.sc.UpdateDistro(distro); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, fmt.Sprintf("Database error for update() distro with distro id '%s'", h.distroId)))
		}
	} else {
		// New resource
		if err = h.sc.CreateDistro(distro); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, fmt.Sprintf("Database error for insert() distro with distro id '%s'", h.distroId)))
		}
	}

	return gimlet.NewJSONResponse(apiDistro)
}

///////////////////////////////////////////////////////////////////////
//
// DELETE /rest/v2/distros/{distro_id}

type distroIDDeleteHandler struct {
	distroId string
	sc       data.Connector
}

func makeDeleteDistroByID(sc data.Connector) gimlet.RouteHandler {
	return &distroIDDeleteHandler{
		sc: sc,
	}
}

func (h *distroIDDeleteHandler) Factory() gimlet.RouteHandler {
	return &distroIDDeleteHandler{
		sc: h.sc,
	}
}

// Parse fetches the distroId from the http request.
func (h *distroIDDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroId = gimlet.GetVars(r)["distro_id"]

	return nil
}

// Run deletes a distro by id.
func (h *distroIDDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	_, err := h.sc.FindDistroById(h.distroId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", h.distroId))
	}

	err = h.sc.DeleteDistroById(h.distroId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for remove() by distro id '%s'", h.distroId))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/distros/{distro_id}

type distroIDPatchHandler struct {
	distroId string
	body     []byte
	sc       data.Connector
}

func makePatchDistroByID(sc data.Connector) gimlet.RouteHandler {
	return &distroIDPatchHandler{
		sc: sc,
	}
}

func (h *distroIDPatchHandler) Factory() gimlet.RouteHandler {
	return &distroIDPatchHandler{
		sc: h.sc,
	}
}

// Parse fetches the distroId from the http request.
func (h *distroIDPatchHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroId = gimlet.GetVars(r)["distro_id"]

	body := util.NewRequestReader(r)
	defer body.Close()
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	h.body = b

	return nil
}

// Run updates a distro by id.
func (h *distroIDPatchHandler) Run(ctx context.Context) gimlet.Responder {
	d, err := h.sc.FindDistroById(h.distroId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", h.distroId))
	}

	apiDistro := &model.APIDistro{}
	if err = apiDistro.BuildFromService(*d); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API Error converting from distro.Expansion to model.APIExpansion"))
	}

	if err = json.Unmarshal(h.body, apiDistro); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error while unmarshalling JSON"))
	}

	d, error := validateToService(ctx, apiDistro, h.distroId, false)
	if error != nil {
		return error
	}

	if err = h.sc.UpdateDistro(d); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for update() by distro id '%s'", h.distroId))
	}

	return gimlet.NewJSONResponse(apiDistro)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/distros/{distro_id}

type distroIDGetHandler struct {
	distroId string
	sc       data.Connector
}

func makeGetDistroByID(sc data.Connector) gimlet.RouteHandler {
	return &distroIDGetHandler{
		sc: sc,
	}
}

func (h *distroIDGetHandler) Factory() gimlet.RouteHandler {
	return &distroIDGetHandler{
		sc: h.sc,
	}
}

// Parse fetches the distroId from the http request.
func (h *distroIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroId = gimlet.GetVars(r)["distro_id"]

	return nil
}

// Run calls the data FindDistroById function and returns the distro from the provider.
func (h *distroIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	foundDistro, err := h.sc.FindDistroById(h.distroId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", h.distroId))
	}

	distroModel := &model.APIDistro{}
	if err = distroModel.BuildFromService(*foundDistro); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API Error converting from distro.Distro to model.APIDistro"))
	}

	return gimlet.NewJSONResponse(distroModel)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/distros

type distroGetHandler struct {
	sc data.Connector
}

func makeDistroRoute(sc data.Connector) gimlet.RouteHandler {
	return &distroGetHandler{
		sc: sc,
	}
}

func (h *distroGetHandler) Factory() gimlet.RouteHandler {
	return &distroGetHandler{
		sc: h.sc,
	}
}

func (h *distroGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *distroGetHandler) Run(ctx context.Context) gimlet.Responder {
	distros, err := h.sc.FindAllDistros()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error for find() all distros"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	for _, d := range distros {
		distroModel := &model.APIDistro{}
		if err = distroModel.BuildFromService(d); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		err = resp.AddData(distroModel)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}

////////////////////////////////////////////////////////////////////////

func validateToService(ctx context.Context, apiDistro *model.APIDistro, resourceId string, isNewDistro bool) (*distro.Distro, gimlet.Responder) {
	i, err := apiDistro.ToService()
	if err != nil {
		return nil, gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API Error converting from model.APIDistro to distro.Distro"))
	}
	d := i.(*distro.Distro)

	id := model.FromAPIString(apiDistro.Name)
	if resourceId != id {
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Distro name is immutable; cannot rename distro resource '%s'", resourceId),
		})
	}

	vErrors, err := validator.CheckDistro(ctx, d, &evergreen.Settings{}, isNewDistro)
	if err != nil {
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
	}
	if len(vErrors) != 0 {
		errors := []string{}
		for _, v := range vErrors {
			errors = append(errors, v.Message)
		}
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    strings.Join(errors, ", "),
		})
	}

	return d, nil
}
