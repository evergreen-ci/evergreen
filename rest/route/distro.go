package route

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

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
// and returned if valid
func (h *distroIDPatchHandler) Run(ctx context.Context) gimlet.Responder {
	distro, err := h.sc.FindDistroById(h.distroId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	initial := &model.APIDistro{}
	if err = initial.BuildFromService(*distro); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API model error"))
	}

	if err = json.Unmarshal(h.body, initial); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API model error"))
	}

	//initial.ToService()

	// vErrors, err := validator.CheckDistro(ctx, distro, &evergreen.Settings{}, false)
	// if err != nil {
	// 	return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
	// 		StatusCode: http.StatusBadRequest,
	// 		Message:    "Database error",
	// 	})
	// }
	// if len(vErrors) != 0 {
	// 	var message strings.Builder
	// 	for _, v := range vErrors {
	// 		message.WriteString(v.Message + ", ")
	// 	}
	// 	return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
	// 		StatusCode: http.StatusBadRequest,
	// 		Message:    message.String(),
	// 	})
	// }
	//
	// if err = h.sc.UpdateDistro(distro); err != nil {
	// 	return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	// }

	distroModel := &model.APIDistro{}
	if err = distroModel.BuildFromService(*distro); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(distroModel)
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
