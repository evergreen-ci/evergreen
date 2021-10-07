package data

import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// This file is used to combine operations across data connectors, to avoid
// duplicated connector usage across the codebase.

func (sc *DBConnector) CopyProject(ctx context.Context, projectToCopy *model.ProjectRef, newProject string) (*restModel.APIProjectRef, error) {
	// copy project, disable necessary settings
	oldId := projectToCopy.Id
	oldIdentifier := projectToCopy.Identifier
	projectToCopy.Id = "" // this will be regenerated during Create
	projectToCopy.Identifier = newProject
	projectToCopy.Enabled = utility.FalsePtr()
	projectToCopy.PRTestingEnabled = nil
	projectToCopy.CommitQueue.Enabled = nil
	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err := sc.CreateProject(projectToCopy, u); err != nil {
		return nil, errors.Wrapf(err, "Database error creating project for id '%s'", newProject)
	}
	apiProjectRef := &restModel.APIProjectRef{}
	if err := apiProjectRef.BuildFromService(*projectToCopy); err != nil {
		return nil, errors.Wrap(err, "error building API project from service")
	}

	// copy variables, aliases, and subscriptions
	if err := sc.CopyProjectVars(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying project vars from project '%s'", oldIdentifier)
	}
	if err := sc.CopyProjectAliases(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying aliases from project '%s'", oldIdentifier)
	}
	if err := sc.CopyProjectSubscriptions(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying subscriptions from project '%s'", oldIdentifier)
	}
	// set the same admin roles from the old project on the newly copied project.
	if err := sc.UpdateAdminRoles(projectToCopy, projectToCopy.Admins, nil); err != nil {
		return nil, errors.Wrapf(err, "Database error updating admins for project '%s'", newProject)
	}
	return apiProjectRef, nil
}

func (sc *DBConnector) SaveProjectSettingsForSection(ctx context.Context, projectId string, changes *restModel.APIProjectSettings,
	section model.ProjectPageSection, isRepo bool, userId string) (*restModel.APIProjectSettings, error) {
	// TODO: this function should only be called after project setting changes have been validated in the resolver or by the front end
	before, err := model.GetProjectSettingsById(projectId, isRepo)
	if err != nil {
		return nil, errors.Wrap(err, "error getting before project settings event")
	}
	v, err := changes.ProjectRef.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "error converting project ref")
	}
	newProjectRef := v.(*model.ProjectRef)
	// If the project ref doesn't use the repo, or we're using a repo ref, then this will just be the same as newProjectRef.
	// Used to verify that if something is set to nil in the request, we properly validate using the merged project ref.
	mergedProjectRef, err := model.GetProjectRefMergedWithRepo(*newProjectRef)
	if err != nil {
		return nil, errors.Wrapf(err, "error merging project ref")
	}

	catcher := grip.NewBasicCatcher()
	modified := false
	switch section {
	case model.ProjectPageGeneralSection:
		// check if webhook is enabled if the owner/repo has changed
		if mergedProjectRef.Owner != before.ProjectRef.Owner || mergedProjectRef.Repo != before.ProjectRef.Repo {
			_, err = sc.EnableWebhooks(ctx, mergedProjectRef)
			if err != nil {
				return nil, errors.Wrapf(err, "Error enabling webhooks for project '%s'", projectId)
			}
		}
		modified = true
	case model.ProjectPageAccessSection:
		// For any admins that are only in the original settings, remove access.
		// For any admins that are only in the updated settings, give them access.
		adminsToDelete, adminsToAdd := utility.StringSliceSymmetricDifference(before.ProjectRef.Admins, mergedProjectRef.Admins)
		makeRestricted := !before.ProjectRef.IsRestricted() && mergedProjectRef.IsRestricted()
		makeUnrestricted := before.ProjectRef.IsRestricted() && !mergedProjectRef.IsRestricted()
		modified = true
		if isRepo {
			// For repos, we need to use the repo ref functions, as they update different scopes/roles.
			repoRef := &model.RepoRef{ProjectRef: *newProjectRef}
			if err = repoRef.UpdateAdminRoles(adminsToAdd, adminsToDelete); err != nil {
				catcher.Wrap(err, "error updating repo admin roles")
			}
			newProjectRef.Admins = repoRef.Admins
			branchProjects, err := model.FindMergedProjectRefsForRepo(repoRef)
			if err != nil {
				return nil, errors.Wrapf(err, "error finding branch projects for repo")
			}
			if makeRestricted {
				catcher.Wrap(repoRef.MakeRestricted(branchProjects), "error making repo restricted")
			}
			if makeUnrestricted {
				catcher.Wrap(repoRef.MakeUnrestricted(branchProjects), "error making repo unrestricted")
			}
		} else {
			if err = newProjectRef.UpdateAdminRoles(adminsToAdd, adminsToDelete); err != nil {
				catcher.Wrap(err, "error updating project admin roles")
			}
			if makeRestricted {
				catcher.Wrap(before.ProjectRef.MakeRestricted(), "error making branch restricted")
			}
			if makeUnrestricted {
				catcher.Wrap(before.ProjectRef.MakeUnrestricted(), "error making branch unrestricted")
			}
		}
	case model.ProjectPageVariablesSection:
		// Remove any variables that only exist in the original settings.
		toDelete := []string{}
		for key, _ := range before.Vars.Vars {
			if _, ok := changes.Vars.Vars[key]; !ok {
				toDelete = append(toDelete, key)
			}
		}
		changes.Vars.VarsToDelete = toDelete
		if err = sc.UpdateProjectVars(projectId, &changes.Vars, false); err != nil { // destructively modifies vars
			return nil, errors.Wrapf(err, "Database error updating variables for project '%s'", projectId)
		}
		modified = true
	case model.ProjectPageGithubAndCQSection, model.ProjectPagePatchAliasSection:
		modified, err = sc.UpdateAliasesForSection(projectId, changes.Aliases, before.Aliases, section)
		catcher.Add(err)
	case model.ProjectPageNotificationsSection:
		if err = sc.SaveSubscriptions(projectId, changes.Subscriptions, true); err != nil {
			return nil, errors.Wrapf(err, "Database error saving subscriptions for project '%s'", projectId)
		}
		modified = true
		subscriptionsToKeep := []string{}
		for _, s := range changes.Subscriptions {
			subscriptionsToKeep = append(subscriptionsToKeep, utility.FromStringPtr(s.ID))
		}
		// Remove any subscriptions that only existed in the original state.
		toDelete := []string{}
		for _, originalSub := range before.Subscriptions {
			if !utility.StringSliceContains(subscriptionsToKeep, originalSub.ID) {
				modified = true
				toDelete = append(toDelete, originalSub.ID)
			}
		}
		catcher.Wrapf(sc.DeleteSubscriptions(projectId, toDelete), "Database error deleting subscriptions")
	}

	modifiedProjectRef, err := model.SaveProjectPageForSection(projectId, newProjectRef, section, isRepo)
	if err != nil {
		return nil, errors.Wrapf(err, "error defaulting project ref to repo for section '%s'", section)
	}
	res := restModel.APIProjectSettings{}
	if modified || modifiedProjectRef {
		after, err := model.GetProjectSettingsById(projectId, isRepo)
		if err != nil {
			catcher.Wrapf(err, "error getting after project settings event")
		} else {
			catcher.Add(model.LogProjectModified(projectId, userId, before, after))
			res, err = restModel.DbProjectSettingsToRestModel(*after)
			if err != nil {
				catcher.Wrapf(err, "error converting project settings")
			}
		}
	}

	return &res, errors.Wrapf(catcher.Resolve(), "error saving section '%s'", section)
}

func (sc *MockConnector) SaveProjectSettingsForSection(ctx context.Context, projectId string, changes *restModel.APIProjectSettings,
	section model.ProjectPageSection, isRepo bool, userId string) (*restModel.APIProjectSettings, error) {
	return nil, nil
}

func (sc *MockConnector) CopyProject(ctx context.Context, projectToCopy *model.ProjectRef, newProject string) (*restModel.APIProjectRef, error) {
	// copy project, disable necessary settings
	oldId := projectToCopy.Id
	oldIdentifier := projectToCopy.Identifier
	projectToCopy.Id = "" // this will be regenerated during Create
	projectToCopy.Identifier = newProject
	projectToCopy.Enabled = utility.FalsePtr()
	projectToCopy.PRTestingEnabled = nil
	projectToCopy.CommitQueue.Enabled = nil
	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err := sc.CreateProject(projectToCopy, u); err != nil {
		return nil, errors.Wrapf(err, "Database error creating project for id '%s'", newProject)
	}
	apiProjectRef := &restModel.APIProjectRef{}
	if err := apiProjectRef.BuildFromService(*projectToCopy); err != nil {
		return nil, errors.Wrap(err, "error building API project from service")
	}

	// copy variables, aliases, and subscriptions
	if err := sc.CopyProjectVars(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying project vars from project '%s'", oldIdentifier)
	}
	if err := sc.CopyProjectAliases(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying aliases from project '%s'", oldIdentifier)
	}
	if err := sc.CopyProjectSubscriptions(oldId, projectToCopy.Id); err != nil {
		return nil, errors.Wrapf(err, "error copying subscriptions from project '%s'", oldIdentifier)
	}
	// set the same admin roles from the old project on the newly copied project.
	if err := sc.UpdateAdminRoles(projectToCopy, projectToCopy.Admins, nil); err != nil {
		return nil, errors.Wrapf(err, "Database error updating admins for project '%s'", newProject)
	}
	return apiProjectRef, nil
}
