package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

///////////////////////////////////////////////////////////////////////////////
//
// POST /admin/role_mapping

type addLDAPRoleMappingHandler struct {
	Group  string `json:"group"`
	RoleID string `json:"role_id"`

	sc data.Connector
}

func makeAddLDAPRoleMappingHandler(sc data.Connector) gimlet.RouteHandler {
	return &addLDAPRoleMappingHandler{
		sc: sc,
	}
}

func (h *addLDAPRoleMappingHandler) Factory() gimlet.RouteHandler {
	return &addLDAPRoleMappingHandler{
		sc: h.sc,
	}
}

func (h *addLDAPRoleMappingHandler) Parse(ctx context.Context, r *http.Request) error {
	if err := gimlet.GetJSON(r.Body, h); err != nil {
		return errors.Wrap(err, "problem parsing request")
	}

	return nil
}

func (h *addLDAPRoleMappingHandler) Run(ctx context.Context) gimlet.Responder {
	err := h.sc.MapLDAPGroupToRole(h.Group, h.RoleID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewTextResponse(fmt.Sprintf("successful mapping of %s to %s", h.Group, h.RoleID))
}

///////////////////////////////////////////////////////////////////////////////
//
// DELETE /admin/role_mapping

type removeLDAPRoleMappingHandler struct {
	Group string `json:"group"`

	sc data.Connector
}

func makeRemoveLDAPRoleMappingHandler(sc data.Connector) gimlet.RouteHandler {
	return &removeLDAPRoleMappingHandler{
		sc: sc,
	}
}

func (h *removeLDAPRoleMappingHandler) Factory() gimlet.RouteHandler {
	return &removeLDAPRoleMappingHandler{
		sc: h.sc,
	}
}

func (h *removeLDAPRoleMappingHandler) Parse(ctx context.Context, r *http.Request) error {
	if err := gimlet.GetJSON(r.Body, h); err != nil {
		return errors.Wrap(err, "problem parsing request")
	}

	return nil
}

func (h *removeLDAPRoleMappingHandler) Run(ctx context.Context) gimlet.Responder {
	err := h.sc.UnmapLDAPGroupToRole(h.Group)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewTextResponse(fmt.Sprintf("successful unmapping of %s", h.Group))
}
