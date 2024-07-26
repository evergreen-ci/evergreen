package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
)

// Distros is the resolver for the distros field.
func (r *imageResolver) Distros(ctx context.Context, obj *model.APIImage) ([]*model.APIDistro, error) {
	if obj == nil {
		return nil, InternalServerError.Send(ctx, "image undefined when attempting to find corresponding distros")
	}
	id := utility.FromStringPtr(obj.ID)
	distros, err := distro.GetDistrosForImage(ctx, id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding distros for image '%s': '%s'", id, err.Error()))
	}
	apiDistros := []*model.APIDistro{}
	for _, d := range distros {
		apiDistro := model.APIDistro{}
		apiDistro.BuildFromService(d)
		apiDistros = append(apiDistros, &apiDistro)
	}
	return apiDistros, nil
}

// Packages is the resolver for the packages field.
func (r *imageResolver) Packages(ctx context.Context, obj *model.APIImage, opts thirdparty.PackageFilterOptions) ([]*model.APIPackage, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting evergreen configuration: '%s'", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	packages, err := c.GetPackages(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting packages for image '%s': '%s'", *obj.ID, err.Error()))
	}
	apiPackages := []*model.APIPackage{}
	for _, pkg := range packages {
		apiPackage := model.APIPackage{}
		apiPackage.BuildFromService(pkg)
		apiPackages = append(apiPackages, &apiPackage)
	}
	return apiPackages, nil
}

// Image returns ImageResolver implementation.
func (r *Resolver) Image() ImageResolver { return &imageResolver{r} }

type imageResolver struct{ *Resolver }
