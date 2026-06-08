package graphql

import (
	"context"
	"fmt"
	"sort"

	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
)

// CustomFields is the resolver for the customFields field.
func (r *jiraNotificationsConfigResolver) CustomFields(ctx context.Context, obj *restModel.APIJIRANotificationsConfig) ([]*JiraNotificationsProjectEntry, error) {
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

// DisplayName is the resolver for the displayName field.
func (r *projectTasksPairResolver) DisplayName(ctx context.Context, obj *restModel.APIProjectTasksPair) (string, error) {
	project, err := model.FindBranchProjectRefSecondary(ctx, obj.ProjectID)
	if err != nil {
		return "", InternalServerError.Send(ctx, fmt.Sprintf("finding project '%s': %s", obj.ProjectID, err.Error()))
	}
	if project != nil {
		displayName := fmt.Sprintf("%s (Project)", util.CoalesceString(project.DisplayName, project.Identifier))
		return displayName, nil
	}
	repo, err := model.FindOneRepoRef(ctx, obj.ProjectID)
	if err != nil {
		return "", InternalServerError.Send(ctx, fmt.Sprintf("finding repo '%s': %s", obj.ProjectID, err.Error()))
	}
	if repo != nil {
		displayName := fmt.Sprintf("%s (Repo)", util.CoalesceString(repo.DisplayName, fmt.Sprintf("%s/%s", repo.Owner, repo.Repo)))
		return displayName, nil
	}
	return "", InternalServerError.Send(ctx, fmt.Sprintf("no project or repo found with ID '%s'", obj.ProjectID))
}

// CustomFields is the resolver for the customFields field.
func (r *jiraNotificationsConfigInputResolver) CustomFields(ctx context.Context, obj *restModel.APIJIRANotificationsConfig, data []*JiraNotificationsProjectEntryInput) error {
	if obj == nil {
		return nil
	}

	if obj.CustomFields == nil {
		obj.CustomFields = make(map[string]restModel.APIJIRANotificationsProject)
	}

	for _, entry := range data {
		if entry != nil {
			obj.CustomFields[entry.Project] = restModel.APIJIRANotificationsProject{
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

// ProjectTasksPair returns ProjectTasksPairResolver implementation.
func (r *Resolver) ProjectTasksPair() ProjectTasksPairResolver { return &projectTasksPairResolver{r} }

// JiraNotificationsConfigInput returns JiraNotificationsConfigInputResolver implementation.
func (r *Resolver) JiraNotificationsConfigInput() JiraNotificationsConfigInputResolver {
	return &jiraNotificationsConfigInputResolver{r}
}

type jiraNotificationsConfigResolver struct{ *Resolver }
type projectTasksPairResolver struct{ *Resolver }
type jiraNotificationsConfigInputResolver struct{ *Resolver }
