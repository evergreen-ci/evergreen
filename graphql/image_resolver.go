package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
)

// Distros is the resolver for the distros field.
func (r *imageResolver) Distros(ctx context.Context, obj *model.APIImage) ([]*model.APIDistro, error) {
	usr := mustHaveUser(ctx)
	imageID := utility.FromStringPtr(obj.ID)
	distros, err := distro.GetDistrosForImage(ctx, imageID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding distros for image '%s': %s", imageID, err.Error()))
	}

	userHasDistroCreatePermission := usr.HasDistroCreatePermission(ctx)

	apiDistros := []*model.APIDistro{}
	for _, d := range distros {
		// Omit admin-only distros if user lacks permissions.
		if d.AdminOnly && !userHasDistroCreatePermission {
			continue
		}
		apiDistro := model.APIDistro{}
		apiDistro.BuildFromService(d)
		apiDistros = append(apiDistros, &apiDistro)
	}
	return apiDistros, nil
}

// Events is the resolver for the events field.
func (r *imageResolver) Events(ctx context.Context, obj *model.APIImage, limit int, page int) (*ImageEventsPayload, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	opts := thirdparty.EventHistoryOptions{
		Image: utility.FromStringPtr(obj.ID),
		Page:  page,
		Limit: limit,
	}
	imageEvents, err := c.GetEvents(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting events for image '%s': %s", imageEvents, err.Error()))
	}
	apiImageEvents := []*model.APIImageEvent{}
	for _, imageEvent := range imageEvents {
		apiImageEvent := model.APIImageEvent{}
		apiImageEvent.BuildFromService(imageEvent)
		apiImageEvents = append(apiImageEvents, &apiImageEvent)
	}

	return &ImageEventsPayload{
		Count:           len(apiImageEvents),
		EventLogEntries: apiImageEvents,
	}, nil
}

// Files is the resolver for the files field.
func (r *imageResolver) Files(ctx context.Context, obj *model.APIImage, opts thirdparty.FileFilterOptions) (*ImageFilesPayload, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	opts.AMI = utility.FromStringPtr(obj.AMI)
	res, err := c.GetFiles(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting files for image '%s': %s", utility.FromStringPtr(obj.ID), err.Error()))
	}
	apiFiles := []*model.APIImageFile{}
	for _, file := range res.Data {
		apiFile := model.APIImageFile{}
		apiFile.BuildFromService(file)
		apiFiles = append(apiFiles, &apiFile)
	}
	return &ImageFilesPayload{
		Data:          apiFiles,
		FilteredCount: res.FilteredCount,
		TotalCount:    res.TotalCount,
	}, nil
}

// LatestTask is the resolver for the latestTask field.
func (r *imageResolver) LatestTask(ctx context.Context, obj *model.APIImage) (*model.APITask, error) {
	imageID := utility.FromStringPtr(obj.ID)
	latestTask, err := task.GetLatestTaskFromImage(ctx, imageID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting latest task for image '%s': %s", imageID, err.Error()))
	}
	if latestTask == nil {
		return nil, nil
	}
	apiLatestTask := &model.APITask{}
	err = apiLatestTask.BuildFromService(ctx, latestTask, &model.APITaskArgs{
		IncludeAMI: true,
	})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting task '%s' to APITask: %s", latestTask.Id, err.Error()))
	}
	return apiLatestTask, nil
}

// OperatingSystem is the resolver for the operatingSystem field.
func (r *imageResolver) OperatingSystem(ctx context.Context, obj *model.APIImage, opts thirdparty.OSInfoFilterOptions) (*ImageOperatingSystemPayload, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	opts.AMI = utility.FromStringPtr(obj.AMI)
	res, err := c.GetOSInfo(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting operating system information for image '%s': %s", utility.FromStringPtr(obj.ID), err.Error()))
	}
	apiOSData := []*model.APIOSInfo{}
	for _, osInfo := range res.Data {
		apiOSInfo := model.APIOSInfo{}
		apiOSInfo.BuildFromService(osInfo)
		apiOSData = append(apiOSData, &apiOSInfo)
	}
	return &ImageOperatingSystemPayload{
		Data:          apiOSData,
		FilteredCount: res.FilteredCount,
		TotalCount:    res.TotalCount,
	}, nil
}

// Packages is the resolver for the packages field.
func (r *imageResolver) Packages(ctx context.Context, obj *model.APIImage, opts thirdparty.PackageFilterOptions) (*ImagePackagesPayload, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	opts.AMI = utility.FromStringPtr(obj.AMI)
	res, err := c.GetPackages(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting packages for image '%s': %s", utility.FromStringPtr(obj.ID), err.Error()))
	}
	apiPackages := []*model.APIPackage{}
	for _, pkg := range res.Data {
		apiPackage := model.APIPackage{}
		apiPackage.BuildFromService(pkg)
		apiPackages = append(apiPackages, &apiPackage)
	}
	return &ImagePackagesPayload{
		Data:          apiPackages,
		FilteredCount: res.FilteredCount,
		TotalCount:    res.TotalCount,
	}, nil
}

// Toolchains is the resolver for the toolchains field.
func (r *imageResolver) Toolchains(ctx context.Context, obj *model.APIImage, opts thirdparty.ToolchainFilterOptions) (*ImageToolchainsPayload, error) {
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	c := thirdparty.NewRuntimeEnvironmentsClient(config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
	opts.AMI = utility.FromStringPtr(obj.AMI)
	res, err := c.GetToolchains(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting toolchains for image '%s': %s", utility.FromStringPtr(obj.ID), err.Error()))
	}
	apiToolchains := []*model.APIToolchain{}
	for _, toolchain := range res.Data {
		apiToolchain := model.APIToolchain{}
		apiToolchain.BuildFromService(toolchain)
		apiToolchains = append(apiToolchains, &apiToolchain)
	}
	return &ImageToolchainsPayload{
		Data:          apiToolchains,
		FilteredCount: res.FilteredCount,
		TotalCount:    res.TotalCount,
	}, nil
}

// Image returns ImageResolver implementation.
func (r *Resolver) Image() ImageResolver { return &imageResolver{r} }

type imageResolver struct{ *Resolver }
