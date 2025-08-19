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

// ProjectRefs is the resolver for the projectRefs field.
func (r *adminSettingsResolver) ProjectRefs(ctx context.Context, obj *restModel.APIAdminSettings) ([]*ProjectRefData, error) {
	projectRefs, err := model.FindAllProjectRefs(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error fetching project refs: %s", err.Error()))
	}

	result := make([]*ProjectRefData, len(projectRefs))
	for i, ref := range projectRefs {
		result[i] = &ProjectRefData{
			ID:          ref.Id,
			DisplayName: ref.Identifier,
		}
	}
	return result, nil
}

// RepoRefs is the resolver for the repoRefs field.
func (r *adminSettingsResolver) RepoRefs(ctx context.Context, obj *restModel.APIAdminSettings) ([]*RepoRefData, error) {
	repoRefs, err := model.FindAllRepoRefs(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error fetching repo refs: %s", err.Error()))
	}

	result := make([]*RepoRefData, len(repoRefs))
	for i, ref := range repoRefs {
		result[i] = &RepoRefData{
			ID:          ref.Id,
			DisplayName: ref.Repo,
		}
	}
	return result, nil
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
