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
	imageID := utility.FromStringPtr(obj.ID)
	distros, err := distro.GetDistrosForImage(ctx, imageID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding distros for image '%s': '%s'", imageID, err.Error()))
	}
	apiDistros := []*model.APIDistro{}
	for _, d := range distros {
		apiDistro := model.APIDistro{}
		apiDistro.BuildFromService(d)
		apiDistros = append(apiDistros, &apiDistro)
	}
	return apiDistros, nil
}

// Toolchains is the resolver for the toolchains field.
func (r *imageResolver) Toolchains(ctx context.Context, obj *model.APIImage, opts thirdparty.ToolchainFilterOptions) ([]*model.APIToolchain, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting evergreen configuration: '%s'", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	opts.AMI = utility.FromStringPtr(obj.AMI)
	toolchains, err := c.GetToolchains(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting toolchains for image '%s': '%s'", utility.FromStringPtr(obj.ID), err.Error()))
	}
	apiToolchains := []*model.APIToolchain{}
	for _, toolchain := range toolchains {
		apiToolchain := model.APIToolchain{}
		apiToolchain.BuildFromService(toolchain)
		apiToolchains = append(apiToolchains, &apiToolchain)
	}
	return apiToolchains, nil
}

// Packages is the resolver for the packages field.
func (r *imageResolver) Packages(ctx context.Context, obj *model.APIImage, opts thirdparty.PackageFilterOptions) ([]*model.APIPackage, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting evergreen configuration: '%s'", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	opts.AMI = utility.FromStringPtr(obj.AMI)
	packages, err := c.GetPackages(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting packages for image '%s': '%s'", utility.FromStringPtr(obj.ID), err.Error()))
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
