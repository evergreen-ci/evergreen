package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.31

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen/rest/model"
)

// ProviderSettingsList is the resolver for the providerSettingsList field.
func (r *distroResolver) ProviderSettingsList(ctx context.Context, obj *model.APIDistro) ([]map[string]interface{}, error) {
	settings := []map[string]interface{}{}
	for _, entry := range obj.ProviderSettingsList {
		settings = append(settings, entry.ExportMap())
	}

	return settings, nil
}

// ProviderSettingsList is the resolver for the providerSettingsList field.
func (r *distroInputResolver) ProviderSettingsList(ctx context.Context, obj *model.APIDistro, data []map[string]interface{}) error {
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
	obj.AcceptableHostIdleTime = model.NewAPIDuration(time.Duration(obj.AcceptableHostIdleTime))
	return nil
}

// TargetTime is the resolver for the targetTime field.
func (r *plannerSettingsInputResolver) TargetTime(ctx context.Context, obj *model.APIPlannerSettings, data int) error {
	obj.TargetTime = model.NewAPIDuration(time.Duration(obj.TargetTime))
	return nil
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

type distroResolver struct{ *Resolver }
type distroInputResolver struct{ *Resolver }
type hostAllocatorSettingsInputResolver struct{ *Resolver }
type plannerSettingsInputResolver struct{ *Resolver }
