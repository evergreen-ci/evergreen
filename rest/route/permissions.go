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

func GetPermissions([]string permissions) []model.APIPermission {
	out := []model.APIPermission{}

	for _, key := range permissions {
		out = append(out, model.APIPermission{
			Key:    key,
			Name:   evergreen.MapPermissionKeyToName(key),
			Levels: evergreen.MapPermissionKeyToPermissionLevels(key),
		})
	}

	return out
}

func (p *permissionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	permissions := &model.APIPermissions{}
	permissions.ProjectPermissions := GetPermissions(evergreen.ProjectPermissions)
	permissions.DistroPermissions := GetPermissions(evergreen.DistroPermissions)

	return gimlet.NewJSONResponse(permissions)
}
