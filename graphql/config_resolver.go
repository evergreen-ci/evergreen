package graphql

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// BannerTheme is the resolver for the bannerTheme field.
func (r *adminSettingsResolver) BannerTheme(ctx context.Context, obj *restModel.APIAdminSettings) (*evergreen.BannerTheme, error) {
	if obj == nil {
		return nil, InternalServerError.Send(ctx, "admin settings object undefined when attempting to resolve banner theme")
	}
	themeString := strings.ToUpper(utility.FromStringPtr(obj.BannerTheme))
	valid, theme := evergreen.IsValidBannerTheme(themeString)
	if !valid {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid banner theme '%s'", themeString))
	}
	return &theme, nil
}

// Port is the resolver for the port field.
func (r *containerPoolResolver) Port(ctx context.Context, obj *restModel.APIContainerPool) (int, error) {
	return int(obj.Port), nil
}

// SecretFields is the resolver for the secretFields field.
func (r *spruceConfigResolver) SecretFields(ctx context.Context, obj *restModel.APIAdminSettings) ([]string, error) {
	redactedFieldsAsSlice := []string{}
	for field := range redactedFields {
		redactedFieldsAsSlice = append(redactedFieldsAsSlice, field)
	}

	sort.Strings(redactedFieldsAsSlice)

	return redactedFieldsAsSlice, nil
}

// BannerTheme is the resolver for the bannerTheme field.
func (r *adminSettingsInputResolver) BannerTheme(ctx context.Context, obj *restModel.APIAdminSettings, data *evergreen.BannerTheme) error {
	if data == nil {
		return nil
	}
	themeString := string(*data)
	obj.BannerTheme = utility.ToStringPtr(themeString)
	return nil
}

// IncludeSystemFailed is the resolver for the includeSystemFailed field.
func (r *restartAdminTasksOptionsResolver) IncludeSystemFailed(ctx context.Context, obj *model.RestartOptions, data bool) error {
	if obj == nil {
		return InternalServerError.Send(ctx, "restart options object undefined")
	}
	obj.IncludeSysFailed = data
	return nil
}

// AdminSettings returns AdminSettingsResolver implementation.
func (r *Resolver) AdminSettings() AdminSettingsResolver { return &adminSettingsResolver{r} }

// ContainerPool returns ContainerPoolResolver implementation.
func (r *Resolver) ContainerPool() ContainerPoolResolver { return &containerPoolResolver{r} }

// SpruceConfig returns SpruceConfigResolver implementation.
func (r *Resolver) SpruceConfig() SpruceConfigResolver { return &spruceConfigResolver{r} }

// AdminSettingsInput returns AdminSettingsInputResolver implementation.
func (r *Resolver) AdminSettingsInput() AdminSettingsInputResolver {
	return &adminSettingsInputResolver{r}
}

// RestartAdminTasksOptions returns RestartAdminTasksOptionsResolver implementation.
func (r *Resolver) RestartAdminTasksOptions() RestartAdminTasksOptionsResolver {
	return &restartAdminTasksOptionsResolver{r}
}

type adminSettingsResolver struct{ *Resolver }
type containerPoolResolver struct{ *Resolver }
type spruceConfigResolver struct{ *Resolver }
type adminSettingsInputResolver struct{ *Resolver }
type restartAdminTasksOptionsResolver struct{ *Resolver }
