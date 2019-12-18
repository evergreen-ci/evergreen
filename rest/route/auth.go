package route

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
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

	// TODO: remove with PM-1355
	if !evergreen.AclCheckingIsEnabled {
		return gimlet.NewTextResponse(true)
	}

	opts := gimlet.PermissionOpts{
		Resource:      h.resource,
		ResourceType:  h.resourceType,
		Permission:    h.permission,
		RequiredLevel: h.requiredLevel,
	}
	ok, err := u.HasPermission(opts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error checking permission for user"))
	}

	return gimlet.NewTextResponse(ok)
}
