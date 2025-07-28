package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// CustomFields is the resolver for the customFields field.
func (r *jiraNotificationsConfigResolver) CustomFields(ctx context.Context, obj *model.APIJIRANotificationsConfig) ([]*JiraNotificationsProjectEntry, error) {
	if obj == nil || obj.CustomFields == nil {
		return []*JiraNotificationsProjectEntry{}, nil
	}

	var entries []*JiraNotificationsProjectEntry
	for projectName, project := range obj.CustomFields {
		entry := &JiraNotificationsProjectEntry{
			Project:    projectName,
			Fields:     project.Fields,
			Components: project.Components,
			Labels:     project.Labels,
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// CustomFields is the resolver for the customFields field.
func (r *jiraNotificationsConfigInputResolver) CustomFields(ctx context.Context, obj *model.APIJIRANotificationsConfig, data []*JiraNotificationsProjectEntryInput) error {
	if obj == nil {
		return nil
	}

	if obj.CustomFields == nil {
		obj.CustomFields = make(map[string]model.APIJIRANotificationsProject)
	}

	for _, entry := range data {
		if entry != nil {
			obj.CustomFields[entry.Project] = model.APIJIRANotificationsProject{
				Fields:     entry.Fields,
				Components: entry.Components,
				Labels:     entry.Labels,
			}
		}
	}
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
