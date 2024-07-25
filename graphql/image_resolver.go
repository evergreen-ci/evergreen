package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
)

// Distros is the resolver for the distros field.
func (r *imageResolver) Distros(ctx context.Context, obj *thirdparty.Image) ([]*model.APIDistro, error) {
	if obj == nil {
		return nil, InternalServerError.Send(ctx, "image undefined when attempting to find corresponding distros")
	}
	distros, err := distro.GetDistrosForImage(ctx, obj.ID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding distros for image '%s': '%s'", obj.ID, err.Error()))
	}
	apiDistros := []*model.APIDistro{}
	for _, d := range distros {
		apiDistro := model.APIDistro{}
		apiDistro.BuildFromService(d)
		apiDistros = append(apiDistros, &apiDistro)
	}
	return apiDistros, nil
}

// Image returns ImageResolver implementation.
func (r *Resolver) Image() ImageResolver { return &imageResolver{r} }

type imageResolver struct{ *Resolver }
