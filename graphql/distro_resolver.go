package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// Image is the resolver for the image field.
func (r *containerIsolationSettingsResolver) Image(ctx context.Context, obj *model.APIContainerIsolationSettings) (string, error) {
	// Image is nil for distros that have never had container isolation
	// configured (rest/model.APIContainerIsolationSettings.BuildFromService
	// intentionally omits it so the REST API's omitempty works). Treat unset
	// as an empty string rather than a GraphQL null, since the field is
	// non-nullable in the schema.
	return utility.FromStringPtr(obj.Image), nil
}

// AvailableRegions is the resolver for the availableRegions field.
func (r *distroResolver) AvailableRegions(ctx context.Context, obj *model.APIDistro) ([]string, error) {
	settings := evergreen.GetEnvironment().Settings()
	d := obj.ToService()
	availableRegions := d.GetRegionsList(settings.Providers.AWS.AllowedRegions)
	return availableRegions, nil
}

// ProviderSettingsList is the resolver for the providerSettingsList field.
func (r *distroResolver) ProviderSettingsList(ctx context.Context, obj *model.APIDistro) ([]map[string]any, error) {
	settings := []map[string]any{}
	for _, entry := range obj.ProviderSettingsList {
		settings = append(settings, entry.ExportMap())
	}

	return settings, nil
}

// ProviderSettingsList is the resolver for the providerSettingsList field.
func (r *distroInputResolver) ProviderSettingsList(ctx context.Context, obj *model.APIDistro, data []map[string]any) error {
	settings := []*birch.Document{}
	for _, entry := range data {
		newEntry, err := json.Marshal(entry)
		if err != nil {
			return InternalServerError.Send(ctx, fmt.Sprintf("marshalling provider settings entry: %s", err.Error()))
		}
		doc := &birch.Document{}
		if err = json.Unmarshal(newEntry, doc); err != nil {
			return InternalServerError.Send(ctx, fmt.Sprintf("converting map to birch: %s", err.Error()))
		}
		settings = append(settings, doc)
	}
	obj.ProviderSettingsList = settings
	return nil
}

// AcceptableHostIdleTime is the resolver for the acceptableHostIdleTime field.
func (r *hostAllocatorSettingsInputResolver) AcceptableHostIdleTime(ctx context.Context, obj *model.APIHostAllocatorSettings, data int) error {
	obj.AcceptableHostIdleTime = model.NewAPIDuration(time.Duration(data) * time.Millisecond)
	return nil
}

// TargetTime is the resolver for the targetTime field.
func (r *plannerSettingsInputResolver) TargetTime(ctx context.Context, obj *model.APIPlannerSettings, data int) error {
	obj.TargetTime = model.NewAPIDuration(time.Duration(data) * time.Millisecond)
	return nil
}

// ContainerIsolationSettings returns ContainerIsolationSettingsResolver implementation.
func (r *Resolver) ContainerIsolationSettings() ContainerIsolationSettingsResolver {
	return &containerIsolationSettingsResolver{r}
}

// Distro returns DistroResolver implementation.
func (r *Resolver) Distro() DistroResolver { return &distroResolver{r} }

// DistroInput returns DistroInputResolver implementation.
func (r *Resolver) DistroInput() DistroInputResolver { return &distroInputResolver{r} }

// HostAllocatorSettingsInput returns HostAllocatorSettingsInputResolver implementation.
func (r *Resolver) HostAllocatorSettingsInput() HostAllocatorSettingsInputResolver {
	return &hostAllocatorSettingsInputResolver{r}
}

// PlannerSettingsInput returns PlannerSettingsInputResolver implementation.
func (r *Resolver) PlannerSettingsInput() PlannerSettingsInputResolver {
	return &plannerSettingsInputResolver{r}
}

type containerIsolationSettingsResolver struct{ *Resolver }
type distroResolver struct{ *Resolver }
type distroInputResolver struct{ *Resolver }
type hostAllocatorSettingsInputResolver struct{ *Resolver }
type plannerSettingsInputResolver struct{ *Resolver }
