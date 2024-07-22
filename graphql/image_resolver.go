package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/grip"
)

// Distros is the resolver for the distros field.
func (r *imageResolver) Distros(ctx context.Context, obj *thirdparty.Image) ([]*model.APIDistro, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting evergreen configuration: '%s'", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	OSInfo, err := c.GetOSInfo(ctx, thirdparty.OSInfoFilterOptions{AMI: obj.AMI})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding OSInfo from AMI: '%s'", err.Error()))
	}
	if len(OSInfo) == 0 {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find OSInfo from AMI '%s'", obj.AMI))
	}
	grip.Debug(OSInfo[0].ImageID)
	distros, err := distro.GetDistrosForImage(ctx, OSInfo[0].ImageID)
	grip.Debug(distros)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding distros from imageID: '%s'", err.Error()))
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
