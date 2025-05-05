package graphql

import (
	"context"
	"fmt"
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// BannerTheme is the resolver for the bannerTheme field.
func (r *adminSettingsResolver) BannerTheme(ctx context.Context, obj *model.APIAdminSettings) (*evergreen.BannerTheme, error) {
	if obj == nil {
		return nil, InternalServerError.Send(ctx, "Banner theme undefined when attempting to resolve communication method")
	}

	bannerTheme := evergreen.BannerTheme(utility.FromStringPtr(obj.BannerTheme))

	switch bannerTheme {
	case evergreen.Announcement:
		return &bannerTheme, nil
	case evergreen.Information:
		return &bannerTheme, nil
	case evergreen.Warning:
		return &bannerTheme, nil
	case evergreen.Important:
		return &bannerTheme, nil
	case evergreen.Empty:
		return &bannerTheme, nil
	default:
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("Banner theme '%s' is invalid", utility.FromStringPtr(obj.BannerTheme)))
	}
}

// Port is the resolver for the port field.
func (r *containerPoolResolver) Port(ctx context.Context, obj *model.APIContainerPool) (int, error) {
	return int(obj.Port), nil
}

// SecretFields is the resolver for the secretFields field.
func (r *spruceConfigResolver) SecretFields(ctx context.Context, obj *model.APIAdminSettings) ([]string, error) {
	redactedFieldsAsSlice := []string{}
	for field := range redactedFields {
		redactedFieldsAsSlice = append(redactedFieldsAsSlice, field)
	}

	sort.Strings(redactedFieldsAsSlice)

	return redactedFieldsAsSlice, nil
}

// AdminSettings returns AdminSettingsResolver implementation.
func (r *Resolver) AdminSettings() AdminSettingsResolver { return &adminSettingsResolver{r} }

// ContainerPool returns ContainerPoolResolver implementation.
func (r *Resolver) ContainerPool() ContainerPoolResolver { return &containerPoolResolver{r} }

// SpruceConfig returns SpruceConfigResolver implementation.
func (r *Resolver) SpruceConfig() SpruceConfigResolver { return &spruceConfigResolver{r} }

type adminSettingsResolver struct{ *Resolver }
type containerPoolResolver struct{ *Resolver }
type spruceConfigResolver struct{ *Resolver }
