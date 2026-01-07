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

// SecretFields is the resolver for the secretFields field.
func (r *spruceConfigResolver) SecretFields(ctx context.Context, obj *restModel.APIAdminSettings) ([]string, error) {
	redactedFieldsAsSlice := []string{}
	for field := range redactedFields {
		redactedFieldsAsSlice = append(redactedFieldsAsSlice, field)
	}

	sort.Strings(redactedFieldsAsSlice)

	return redactedFieldsAsSlice, nil
}

// UserServiceFlags is the resolver for the userServiceFlags field.
func (r *spruceConfigResolver) UserServiceFlags(ctx context.Context, obj *restModel.APIAdminSettings) (*restModel.APIServiceFlags, error) {
	return obj.ServiceFlags, nil
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
