package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// CustomFields is the resolver for the customFields field.
func (r *jiraNotificationsConfigResolver) CustomFields(ctx context.Context, obj *model.APIJIRANotificationsConfig) (JIRANotificationsProjectMap, error) {
	if obj == nil || obj.CustomFields == nil {
		return JIRANotificationsProjectMap{}, nil
	}
	return JIRANotificationsProjectMap(obj.CustomFields), nil
}

// CustomFields is the resolver for the customFields field.
func (r *jiraNotificationsConfigInputResolver) CustomFields(ctx context.Context, obj *model.APIJIRANotificationsConfig, data JIRANotificationsProjectMap) error {
	if obj == nil {
		return nil
	}
	obj.CustomFields = map[string]model.APIJIRANotificationsProject(data)
	return nil
}

// JiraNotificationsConfig returns JiraNotificationsConfigResolver implementation.
func (r *Resolver) JiraNotificationsConfig() JiraNotificationsConfigResolver {
	return &jiraNotificationsConfigResolver{r}
}

// JiraNotificationsConfigInput returns JiraNotificationsConfigInputResolver implementation.
func (r *Resolver) JiraNotificationsConfigInput() JiraNotificationsConfigInputResolver {
	return &jiraNotificationsConfigInputResolver{r}
}

type jiraNotificationsConfigResolver struct{ *Resolver }
type jiraNotificationsConfigInputResolver struct{ *Resolver }
