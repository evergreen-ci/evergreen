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

func getPermissions(permissions []string) []model.APIPermission {
	out := []model.APIPermission{}

	for _, key := range permissions {
		out = append(out, model.APIPermission{
			Key:    key,
			Name:   evergreen.GetDisplayNameForPermissionKey(key),
			Levels: evergreen.GetPermissionLevelsForPermissionKey(key),
		})
	}

	return out
}

func getAllPermissions() *model.APIPermissions {
	out := &model.APIPermissions{}

	out.ProjectPermissions = getPermissions(evergreen.ProjectPermissions)
	out.DistroPermissions = getPermissions(evergreen.DistroPermissions)

	return out
}

func (p *permissionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	return gimlet.NewJSONResponse(getAllPermissions())
}
