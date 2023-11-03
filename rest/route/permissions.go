package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
)

type permissionsGetHandler struct{}

// Factory creates an instance of the handler.
//
//	@Summary		Get permissions
//	@Description	Returns a static list of project and distro permissions that can be granted to users.
//	@Tags			info
//	@Router			/permissions [get]
//	@Security		Api-User || Api-Key
//	@Success		200	{object}	model.APIPermissions
func (p *permissionsGetHandler) Factory() gimlet.RouteHandler                     { return p }
func (p *permissionsGetHandler) Parse(ctx context.Context, r *http.Request) error { return nil }

func (p *permissionsGetHandler) getPermissions(permissions []string) []model.APIPermission {
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

func (p *permissionsGetHandler) getAllPermissions() *model.APIPermissions {
	out := &model.APIPermissions{
		ProjectPermissions: p.getPermissions(evergreen.ProjectPermissions),
		DistroPermissions:  p.getPermissions(evergreen.DistroPermissions),
	}
	return out
}

func (p *permissionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	return gimlet.NewJSONResponse(p.getAllPermissions())
}
