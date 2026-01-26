package graphql

import (
	"context"
	"fmt"
	"sort"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// S3Cost is the resolver for the s3Cost field.
func (r *costConfigResolver) S3Cost(ctx context.Context, obj *model.APICostConfig) (*S3CostConfig, error) {
	panic(fmt.Errorf("not implemented: S3Cost - s3Cost"))
}

// CustomFields is the resolver for the customFields field.
func (r *jiraNotificationsConfigResolver) CustomFields(ctx context.Context, obj *model.APIJIRANotificationsConfig) ([]*JiraNotificationsProjectEntry, error) {
	if obj == nil || obj.CustomFields == nil {
		return []*JiraNotificationsProjectEntry{}, nil
	}

	var entries []*JiraNotificationsProjectEntry

	// Get project names and sort them alphabetically to guarantee consistent order.
	projectNames := make([]string, 0, len(obj.CustomFields))
	for projectName := range obj.CustomFields {
		projectNames = append(projectNames, projectName)
	}
	sort.Strings(projectNames)

	for _, projectName := range projectNames {
		project := obj.CustomFields[projectName]

		// Sort fields, components, and labels to guarantee consistent order.
		var sortedFields map[string]string
		if project.Fields != nil {
			fieldKeys := make([]string, 0, len(project.Fields))
			for key := range project.Fields {
				fieldKeys = append(fieldKeys, key)
			}
			sort.Strings(fieldKeys)

			sortedFields = make(map[string]string)
			for _, key := range fieldKeys {
				sortedFields[key] = project.Fields[key]
			}
		}
		sort.Strings(project.Components)
		sort.Strings(project.Labels)

		entry := &JiraNotificationsProjectEntry{
			Project:    projectName,
			Fields:     sortedFields,
			Components: project.Components,
			Labels:     project.Labels,
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// S3Cost is the resolver for the s3Cost field.
func (r *costConfigInputResolver) S3Cost(ctx context.Context, obj *model.APICostConfig, data *S3CostConfigInput) error {
	panic(fmt.Errorf("not implemented: S3Cost - s3Cost"))
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

// CostConfig returns CostConfigResolver implementation.
func (r *Resolver) CostConfig() CostConfigResolver { return &costConfigResolver{r} }

// JiraNotificationsConfig returns JiraNotificationsConfigResolver implementation.
func (r *Resolver) JiraNotificationsConfig() JiraNotificationsConfigResolver {
	return &jiraNotificationsConfigResolver{r}
}

// CostConfigInput returns CostConfigInputResolver implementation.
func (r *Resolver) CostConfigInput() CostConfigInputResolver { return &costConfigInputResolver{r} }

// JiraNotificationsConfigInput returns JiraNotificationsConfigInputResolver implementation.
func (r *Resolver) JiraNotificationsConfigInput() JiraNotificationsConfigInputResolver {
	return &jiraNotificationsConfigInputResolver{r}
}

type costConfigResolver struct{ *Resolver }
type jiraNotificationsConfigResolver struct{ *Resolver }
type costConfigInputResolver struct{ *Resolver }
type jiraNotificationsConfigInputResolver struct{ *Resolver }
