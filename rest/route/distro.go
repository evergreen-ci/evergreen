package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

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

// ParseAndValidate fetches the distroId from the http request.
func (h *distroIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.distroId = gimlet.GetVars(r)["distro_id"]

	if h.distroId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

// Execute calls the data FindDistroById function and returns the distro
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
