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
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// Version is the resolver for the version field.
func (r *dispatcherSettingsResolver) Version(ctx context.Context, obj *model.APIDispatcherSettings) (DispatcherVersion, error) {
	if obj == nil {
		return "", InternalServerError.Send(ctx, "distro undefined when attempting to resolve dispatcher version")
	}

	switch utility.FromStringPtr(obj.Version) {
	case evergreen.DispatcherVersionRevised:
		return DispatcherVersionRevised, nil
	case evergreen.DispatcherVersionRevisedWithDependencies:
		return DispatcherVersionRevisedWithDependencies, nil
	default:
		return "", InternalServerError.Send(ctx, fmt.Sprintf("dispatcher version '%s' is invalid", utility.FromStringPtr(obj.Version)))
	}
}

// CloneMethod is the resolver for the cloneMethod field.
func (r *distroResolver) CloneMethod(ctx context.Context, obj *model.APIDistro) (CloneMethod, error) {
	if obj == nil {
		return "", InternalServerError.Send(ctx, "distro undefined when attempting to resolve clone method")
	}

	switch utility.FromStringPtr(obj.CloneMethod) {
	case evergreen.CloneMethodLegacySSH:
		return CloneMethodLegacySSH, nil
	case evergreen.CloneMethodOAuth:
		return CloneMethodOauth, nil
	default:
		return "", InternalServerError.Send(ctx, fmt.Sprintf("clone method '%s' is invalid", *obj.CloneMethod))
	}
}

// ProviderSettingsList is the resolver for the providerSettingsList field.
func (r *distroResolver) ProviderSettingsList(ctx context.Context, obj *model.APIDistro) ([]map[string]interface{}, error) {
	settings := []map[string]interface{}{}
	for _, entry := range obj.ProviderSettingsList {
		settings = append(settings, entry.ExportMap())
	}

	return settings, nil
}

// Version is the resolver for the version field.
func (r *finderSettingsResolver) Version(ctx context.Context, obj *model.APIFinderSettings) (FinderVersion, error) {
	if obj == nil {
		return "", InternalServerError.Send(ctx, "distro undefined when attempting to resolve finder version")
	}

	switch utility.FromStringPtr(obj.Version) {
	case evergreen.FinderVersionLegacy:
		return FinderVersionLegacy, nil
	case evergreen.FinderVersionParallel:
		return FinderVersionParallel, nil
	case evergreen.FinderVersionPipeline:
		return FinderVersionPipeline, nil
	case evergreen.FinderVersionAlternate:
		return FinderVersionAlternate, nil
	default:
		return "", InternalServerError.Send(ctx, fmt.Sprintf("finder version '%s' is invalid", utility.FromStringPtr(obj.Version)))
	}
}

// Version is the resolver for the version field.
func (r *plannerSettingsResolver) Version(ctx context.Context, obj *model.APIPlannerSettings) (PlannerVersion, error) {
	if obj == nil {
		return "", InternalServerError.Send(ctx, "distro undefined when attempting to resolve planner version")
	}

	switch utility.FromStringPtr(obj.Version) {
	case evergreen.PlannerVersionLegacy:
		return PlannerVersionLegacy, nil
	case evergreen.PlannerVersionTunable:
		return PlannerVersionTunable, nil
	default:
		return "", InternalServerError.Send(ctx, fmt.Sprintf("planner version '%s' is invalid", utility.FromStringPtr(obj.Version)))
	}
}

// Version is the resolver for the version field.
func (r *dispatcherSettingsInputResolver) Version(ctx context.Context, obj *model.APIDispatcherSettings, data DispatcherVersion) error {
	switch data {
	case DispatcherVersionRevised:
		obj.Version = utility.ToStringPtr(evergreen.DispatcherVersionRevised)
	case DispatcherVersionRevisedWithDependencies:
		obj.Version = utility.ToStringPtr(evergreen.DispatcherVersionRevisedWithDependencies)
	default:
		return InputValidationError.Send(ctx, fmt.Sprintf("dispatcher version '%s' is invalid", data))
	}
	return nil
}

// CloneMethod is the resolver for the cloneMethod field.
func (r *distroInputResolver) CloneMethod(ctx context.Context, obj *model.APIDistro, data CloneMethod) error {
	switch data {
	case CloneMethodLegacySSH:
		obj.CloneMethod = utility.ToStringPtr(evergreen.CloneMethodLegacySSH)
	case CloneMethodOauth:
		obj.CloneMethod = utility.ToStringPtr(evergreen.CloneMethodOAuth)
	default:
		return InputValidationError.Send(ctx, fmt.Sprintf("Clone method '%s' is invalid", data))
	}
	return nil
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

// Version is the resolver for the version field.
func (r *finderSettingsInputResolver) Version(ctx context.Context, obj *model.APIFinderSettings, data FinderVersion) error {
	switch data {
	case FinderVersionLegacy:
		obj.Version = utility.ToStringPtr(evergreen.FinderVersionLegacy)
	case FinderVersionParallel:
		obj.Version = utility.ToStringPtr(evergreen.FinderVersionParallel)
	case FinderVersionPipeline:
		obj.Version = utility.ToStringPtr(evergreen.FinderVersionPipeline)
	case FinderVersionAlternate:
		obj.Version = utility.ToStringPtr(evergreen.FinderVersionAlternate)
	default:
		return InputValidationError.Send(ctx, fmt.Sprintf("finder version '%s' is invalid", data))
	}
	return nil
}

// AcceptableHostIdleTime is the resolver for the acceptableHostIdleTime field.
func (r *hostAllocatorSettingsInputResolver) AcceptableHostIdleTime(ctx context.Context, obj *model.APIHostAllocatorSettings, data int) error {
	obj.AcceptableHostIdleTime = model.NewAPIDuration(time.Duration(data))
	return nil
}

// TargetTime is the resolver for the targetTime field.
func (r *plannerSettingsInputResolver) TargetTime(ctx context.Context, obj *model.APIPlannerSettings, data int) error {
	obj.TargetTime = model.NewAPIDuration(time.Duration(data))
	return nil
}

// Version is the resolver for the version field.
func (r *plannerSettingsInputResolver) Version(ctx context.Context, obj *model.APIPlannerSettings, data PlannerVersion) error {
	switch data {
	case PlannerVersionLegacy:
		obj.Version = utility.ToStringPtr(evergreen.PlannerVersionLegacy)
	case PlannerVersionTunable:
		obj.Version = utility.ToStringPtr(evergreen.PlannerVersionTunable)
	default:
		return InputValidationError.Send(ctx, fmt.Sprintf("planner version '%s' is invalid", data))
	}
	return nil
}

// DispatcherSettings returns DispatcherSettingsResolver implementation.
func (r *Resolver) DispatcherSettings() DispatcherSettingsResolver {
	return &dispatcherSettingsResolver{r}
}

// Distro returns DistroResolver implementation.
func (r *Resolver) Distro() DistroResolver { return &distroResolver{r} }

// FinderSettings returns FinderSettingsResolver implementation.
func (r *Resolver) FinderSettings() FinderSettingsResolver { return &finderSettingsResolver{r} }

// PlannerSettings returns PlannerSettingsResolver implementation.
func (r *Resolver) PlannerSettings() PlannerSettingsResolver { return &plannerSettingsResolver{r} }

// DispatcherSettingsInput returns DispatcherSettingsInputResolver implementation.
func (r *Resolver) DispatcherSettingsInput() DispatcherSettingsInputResolver {
	return &dispatcherSettingsInputResolver{r}
}

// DistroInput returns DistroInputResolver implementation.
func (r *Resolver) DistroInput() DistroInputResolver { return &distroInputResolver{r} }

// FinderSettingsInput returns FinderSettingsInputResolver implementation.
func (r *Resolver) FinderSettingsInput() FinderSettingsInputResolver {
	return &finderSettingsInputResolver{r}
}

// HostAllocatorSettingsInput returns HostAllocatorSettingsInputResolver implementation.
func (r *Resolver) HostAllocatorSettingsInput() HostAllocatorSettingsInputResolver {
	return &hostAllocatorSettingsInputResolver{r}
}

// PlannerSettingsInput returns PlannerSettingsInputResolver implementation.
func (r *Resolver) PlannerSettingsInput() PlannerSettingsInputResolver {
	return &plannerSettingsInputResolver{r}
}

type dispatcherSettingsResolver struct{ *Resolver }
type distroResolver struct{ *Resolver }
type finderSettingsResolver struct{ *Resolver }
type plannerSettingsResolver struct{ *Resolver }
type dispatcherSettingsInputResolver struct{ *Resolver }
type distroInputResolver struct{ *Resolver }
type finderSettingsInputResolver struct{ *Resolver }
type hostAllocatorSettingsInputResolver struct{ *Resolver }
type plannerSettingsInputResolver struct{ *Resolver }
