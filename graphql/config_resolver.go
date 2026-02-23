package graphql

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/evergreen-ci/evergreen"
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

// ServiceFlagsList is the resolver for the serviceFlagsList field.
func (r *adminSettingsResolver) ServiceFlagsList(ctx context.Context, obj *restModel.APIAdminSettings) ([]*ServiceFlag, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting service flags: %s", err.Error()))
	}
	entries := flags.ToSlice()
	items := make([]*ServiceFlag, 0, len(entries))
	for _, e := range entries {
		items = append(items, &ServiceFlag{Name: e.Name, Enabled: e.Enabled})
	}
	return items, nil
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

// AdminSettings returns AdminSettingsResolver implementation.
func (r *Resolver) AdminSettings() AdminSettingsResolver { return &adminSettingsResolver{r} }

// SpruceConfig returns SpruceConfigResolver implementation.
func (r *Resolver) SpruceConfig() SpruceConfigResolver { return &spruceConfigResolver{r} }

// AdminSettingsInput returns AdminSettingsInputResolver implementation.
func (r *Resolver) AdminSettingsInput() AdminSettingsInputResolver {
	return &adminSettingsInputResolver{r}
}

type adminSettingsResolver struct{ *Resolver }
type spruceConfigResolver struct{ *Resolver }
type adminSettingsInputResolver struct{ *Resolver }
