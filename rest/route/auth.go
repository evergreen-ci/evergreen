package route

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/gimlet"
)

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/auth

type authPermissionGetHandler struct {
	resource      string
	resourceType  string
	permission    string
	requiredLevel int
}

func (h *authPermissionGetHandler) Factory() gimlet.RouteHandler { return h }
func (h *authPermissionGetHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error

	vals := r.URL.Query()
	h.resource = vals.Get("resource")
	h.resourceType = vals.Get("resource_type")
	h.permission = vals.Get("permission")
	h.requiredLevel, err = strconv.Atoi(vals.Get("required_level"))

	return err
}

func (h *authPermissionGetHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	opts := gimlet.PermissionOpts{
		Resource:      h.resource,
		ResourceType:  h.resourceType,
		Permission:    h.permission,
		RequiredLevel: h.requiredLevel,
	}
	return gimlet.NewTextResponse(u.HasPermission(ctx, opts))
}
