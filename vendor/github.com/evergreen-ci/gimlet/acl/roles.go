package acl

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/gimlet"
)

type updateRoleHandler struct {
	manager gimlet.RoleManager
	role    *gimlet.Role
}

func NewUpdateRoleHandler(m gimlet.RoleManager) gimlet.RouteHandler {
	return &updateRoleHandler{
		manager: m,
	}
}

func (h *updateRoleHandler) Factory() gimlet.RouteHandler {
	return &updateRoleHandler{
		manager: h.manager,
	}
}

func (h *updateRoleHandler) Parse(ctx context.Context, r *http.Request) error {
	h.role = &gimlet.Role{}
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, h.role)
	if err != nil {
		return err
	}
	return nil
}

func (h *updateRoleHandler) Run(ctx context.Context) gimlet.Responder {
	err := h.manager.UpdateRole(*h.role)
	if err != nil {
		return gimlet.NewJSONErrorResponse(err)
	}

	return gimlet.NewJSONResponse(h.role)
}

type getAllRolesHandler struct {
	manager gimlet.RoleManager
}

func NewGetAllRolesHandler(m gimlet.RoleManager) gimlet.RouteHandler {
	return &getAllRolesHandler{
		manager: m,
	}
}

func (h *getAllRolesHandler) Factory() gimlet.RouteHandler {
	return &getAllRolesHandler{
		manager: h.manager,
	}
}

func (h *getAllRolesHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *getAllRolesHandler) Run(ctx context.Context) gimlet.Responder {
	roles, err := h.manager.GetAllRoles()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse(roles)
}
