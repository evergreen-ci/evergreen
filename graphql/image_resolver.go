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
	if obj.ImageID == "" {
		return nil, ResourceNotFound.Send(ctx, "unable to find imageID from provided Image")
	}
	distros, err := distro.GetDistrosForImage(ctx, obj.ImageID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding distros from imageID '%s': '%s'", obj.ImageID, err.Error()))
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
