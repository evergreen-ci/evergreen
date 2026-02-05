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

// !!! WARNING !!!
// The code below was going to be deleted when updating resolvers. It has been copied here so you have
// one last chance to move it out of harms way if you want. There are two reasons this happens:
//  - When renaming or deleting a resolver the old code will be put in here. You can safely delete
//    it when you're done.
//  - You have helper methods in this file. Move them out to keep these resolver files clean.
/*
	func (r *adminSettingsResolver) DebugSpawnHosts(ctx context.Context, obj *restModel.APIAdminSettings) (*DebugSpawnHostsConfig, error) {
	panic(fmt.Errorf("not implemented: DebugSpawnHosts - debugSpawnHosts"))
}
func (r *adminSettingsInputResolver) DebugSpawnHosts(ctx context.Context, obj *restModel.APIAdminSettings, data *DebugSpawnHostsConfigInput) error {
	panic(fmt.Errorf("not implemented: DebugSpawnHosts - debugSpawnHosts"))
}
*/
