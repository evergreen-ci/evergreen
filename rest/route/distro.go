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

// Parse() fetches the distroId from the http request.
func (h *distroIDDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroId = gimlet.GetVars(r)["distro_id"]

	if h.distroId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

func (dgh *distroIDDeleteHandler) Run(ctx context.Context) gimlet.Responder {

	return nil
	// distros, err := dgh.sc.FindAllDistros()
	// if err != nil {
	// 	return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	// }
	//
	// resp := gimlet.NewResponseBuilder()
	// if err = resp.SetFormat(gimlet.JSON); err != nil {
	// 	return gimlet.MakeJSONErrorResponder(err)
	// }
	//
	// for _, d := range distros {
	// 	distroModel := &model.APIDistro{}
	//
	// 	if err = distroModel.BuildFromService(d); err != nil {
	// 		return gimlet.MakeJSONErrorResponder(err)
	// 	}
	//
	// 	err = resp.AddData(distroModel)
	// 	if err != nil {
	// 		return gimlet.MakeJSONErrorResponder(err)
	// 	}
	// }
	//
	// return resp
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

// Parse() fetches the distroId and json payload from the http request.
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

// Run() finds a distro by id; validates its patched state, which is updated
// and returned if valid.
func (h *distroIDPatchHandler) Run(ctx context.Context) gimlet.Responder {
	d, err := h.sc.FindDistroById(h.distroId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error for find() by distro id"))
	}

	apiDistro := &model.APIDistro{}
	if err = apiDistro.BuildFromService(*d); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API Error converting from distro.Expansion to model.APIExpansion"))
	}

	if err = json.Unmarshal(h.body, apiDistro); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error while unmarshalling JSON"))
	}

	i, err := apiDistro.ToService()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API Error converting from model.APIExpansion to distro.Expansion"))
	}
	d = i.(*distro.Distro)

	if d.Id != h.distroId {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Distro name is immutable; cannot rename distro '%s'", h.distroId),
		})
	}

	vErrors, err := validator.CheckDistro(ctx, d, &evergreen.Settings{}, false)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
	}
	if len(vErrors) != 0 {
		errors := []string{}
		for _, v := range vErrors {
			errors = append(errors, v.Message)
		}
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    strings.Join(errors, ", "),
		})
	}

	if err = h.sc.UpdateDistro(d); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error for update() by distro id"))
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

// Parse() fetches the distroId from the http request.
func (h *distroIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroId = gimlet.GetVars(r)["distro_id"]

	if h.distroId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

// Run() calls the data FindDistroById function and returns the distro
// from the provider.
func (h *distroIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	foundDistro, err := h.sc.FindDistroById(h.distroId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Database error"))
	}

	distroModel := &model.APIDistro{}
	if err = distroModel.BuildFromService(*foundDistro); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(distroModel)
}

type distroGetHandler struct {
	sc data.Connector
}

func makeDistroRoute(sc data.Connector) gimlet.RouteHandler {
	return &distroGetHandler{
		sc: sc,
	}
}

func (dgh *distroGetHandler) Factory() gimlet.RouteHandler {
	return &distroGetHandler{
		sc: dgh.sc,
	}
}

func (dgh *distroGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (dgh *distroGetHandler) Run(ctx context.Context) gimlet.Responder {
	distros, err := dgh.sc.FindAllDistros()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
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
