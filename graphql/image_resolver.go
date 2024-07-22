package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
)

// Distros is the resolver for the distros field.
func (r *imageResolver) Distros(ctx context.Context, obj *thirdparty.Image) ([]*model.APIDistro, error) {
	distros, err := distro.GetDistrosForImage(ctx, obj.Name)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding imageID from distro: '%s'", err.Error()))
	}
	apiDistros := []*restModel.APIDistro{}
	for _, d := range distros {
		apiDistro := restModel.APIDistro{}
		apiDistro.BuildFromService(d)
		apiDistros = append(apiDistros, &apiDistro)
	}
	return apiDistros, nil
}

// Image returns ImageResolver implementation.
func (r *Resolver) Image() ImageResolver { return &imageResolver{r} }

type imageResolver struct{ *Resolver }
