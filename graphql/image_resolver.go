package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
		AMI: obj.AMI,
	}
	if opts.Manager != nil {
		optsPackages.Manager = *(opts.Manager)
	}
	if opts.Name != nil {
		optsPackages.Name = *(opts.Name)
	}
	if opts.Page != nil {
		optsPackages.Page = *(opts.Page)
	}
	if opts.Limit != nil {
		optsPackages.Limit = *(opts.Limit)
	}
	packages, err := c.GetPackages(ctx, optsPackages)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting packages for image '%s': '%s'", obj.ID, err.Error()))
	}
	grip.Debug(message.Fields{
		"packages": packages,
	})
	packagesPtr := make([]*thirdparty.Package, 0, len(packages))
	for i := range packages {
		packagesPtr = append(packagesPtr, &packages[i])
	}
	return packagesPtr, nil
}

// Image returns ImageResolver implementation.
func (r *Resolver) Image() ImageResolver { return &imageResolver{r} }

type imageResolver struct{ *Resolver }
