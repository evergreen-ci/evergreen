package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
)

type permissionsGetHandler struct{}

func (p *permissionsGetHandler) Factory() gimlet.RouteHandler                     { return p }
func (p *permissionsGetHandler) Parse(ctx context.Context, r *http.Request) error { return nil }

func (p *permissionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	out := &model.APIPermissions{}

	for _, key := range evergreen.AllPermissions {
		out.Permissions = append(out.Permissions, model.APIPermission{
			Key:    key,
			Name:   evergreen.MapPermissionKeyToName(key),
			Levels: evergreen.MapPermissionKeyToPermissionLevels(key),
		})
	}

	return gimlet.NewJSONResponse(out)
}
