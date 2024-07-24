package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
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

// Packages is the resolver for the packages field.
func (r *imageResolver) Packages(ctx context.Context, obj *thirdparty.Image, opts PackageOpts) ([]*thirdparty.Package, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting evergreen configuration: '%s'", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	optsPackages := thirdparty.PackageFilterOptions{
		AMI:     obj.AMI,
		Manager: *opts.Manager,
		Name:    *opts.Name,
		Page:    *opts.Page,
		Limit:   *opts.Limit,
	}
	packages, err := c.GetPackages(ctx, optsPackages)
	if err != nil {
		return nil, InternalServerError.Send(ctx, "getting packages")
	}
	packagesCleaned := []*thirdparty.Package{}
	for _, p := range packages {
		packagesCleaned = append(packagesCleaned, &p)
	}
	return packagesCleaned, nil
}

// Image returns ImageResolver implementation.
func (r *Resolver) Image() ImageResolver { return &imageResolver{r} }

type imageResolver struct{ *Resolver }
