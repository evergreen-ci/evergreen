package data

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// This file is used to combine operations across data connectors, to avoid
// duplicated connector usage across the codebase.

type CopyProjectOpts struct {
	ProjectIdToCopy      string
	NewProjectIdentifier string
	NewProjectId         string
}

// CopyProject copies the passed in project with the given project identifier, and returns the new project.
func CopyProject(ctx context.Context, opts CopyProjectOpts) (*restModel.APIProjectRef, error) {
	projectToCopy, err := FindProjectById(opts.ProjectIdToCopy, false, false)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project '%s'", opts.ProjectIdToCopy)
	}
	if projectToCopy == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project '%s' not found", opts.ProjectIdToCopy),
		}
	}

	oldId := projectToCopy.Id
	// project ID will be validated or generated during CreateProject
	if opts.NewProjectId != "" {
		projectToCopy.Id = opts.NewProjectId
	} else {
		projectToCopy.Id = ""
	}

	// Copy project and disable necessary settings.
	oldIdentifier := projectToCopy.Identifier
	projectToCopy.Identifier = opts.NewProjectIdentifier
	disableStartingSettings(projectToCopy)

	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err := CreateProject(ctx, projectToCopy, u); err != nil {
		return nil, err
	}
	apiProjectRef := &restModel.APIProjectRef{}
	if err := apiProjectRef.BuildFromService(*projectToCopy); err != nil {
		return nil, errors.Wrap(err, "converting project to API model")
	}

	// copy variables, aliases, and subscriptions
	catcher := grip.NewBasicCatcher()
	if err := model.CopyProjectVars(oldId, projectToCopy.Id); err != nil {
		catcher.Wrapf(err, "copying project vars from project '%s'", oldIdentifier)
	}
	if err := model.CopyProjectAliases(oldId, projectToCopy.Id); err != nil {
		catcher.Wrapf(err, "copying aliases from project '%s'", oldIdentifier)
	}
	if err := event.CopyProjectSubscriptions(oldId, projectToCopy.Id); err != nil {
		catcher.Wrapf(err, "copying subscriptions from project '%s'", oldIdentifier)
	}
	// set the same admin roles from the old project on the newly copied project.
	if err := model.UpdateAdminRoles(projectToCopy, projectToCopy.Admins, nil); err != nil {
		catcher.Wrapf(err, "updating admins for project '%s'", opts.NewProjectIdentifier)
	}
	// Since the errors above are nonfatal and still permit copying the project, return both the new project and any errors that were encountered.
	return apiProjectRef, catcher.Resolve()
}

func disableStartingSettings(p *model.ProjectRef) {
	p.Enabled = utility.FalsePtr()
	p.PRTestingEnabled = utility.FalsePtr()
	p.ManualPRTestingEnabled = utility.FalsePtr()
	p.GithubChecksEnabled = utility.FalsePtr()
	p.CommitQueue.Enabled = utility.FalsePtr()
}

// SaveProjectSettingsForSection saves the given UI page section and logs it for the given user. If isRepo is true, uses
// RepoRef related functions and collection instead of ProjectRef.
func SaveProjectSettingsForSection(ctx context.Context, projectId string, changes *restModel.APIProjectSettings,
	section model.ProjectPageSection, isRepo bool, userId string) (*restModel.APIProjectSettings, error) {
	before, err := model.GetProjectSettingsById(projectId, isRepo)
	if err != nil {
		return nil, errors.Wrap(err, "getting before project settings event")
	}

	newProjectRef, err := changes.ProjectRef.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting project ref changes to service model")
	}

	// Changes sent to the resolver will not include the RepoRefId for some pages.
	// Fall back on the existing value if none is provided in order to properly merge refs.
	if newProjectRef.RepoRefId == "" {
		newProjectRef.RepoRefId = before.ProjectRef.RepoRefId
	}

	// If the project ref doesn't use the repo, or we're using a repo ref, then this will just be the same as the passed in ref.
	// Used to verify that if something is set to nil, we properly validate using the merged project ref.
	mergedProjectRef, err := model.GetProjectRefMergedWithRepo(*newProjectRef)
	if err != nil {
		return nil, errors.Wrapf(err, "getting merged project ref")
	}
	mergedBeforeRef, err := model.GetProjectRefMergedWithRepo(before.ProjectRef)
	if err != nil {
		return nil, errors.Wrap(err, "getting the original merged project ref")
	}

	catcher := grip.NewBasicCatcher()
	modified := false
	switch section {
	case model.ProjectPageGeneralSection:
		if mergedProjectRef.Identifier != mergedBeforeRef.Identifier {
			if err = handleIdentifierConflict(mergedProjectRef); err != nil {
				return nil, err
			}
		}

		// Only need to check Github conflicts once so we use else if statements to handle this.
		// Handle conflicts using the ref from the DB, since only general section settings are passed in from the UI.
		if mergedProjectRef.Owner != mergedBeforeRef.Owner || mergedProjectRef.Repo != mergedBeforeRef.Repo {
			if err = handleGithubConflicts(mergedBeforeRef, "Changing owner/repo"); err != nil {
				return nil, err
			}
			// Check if webhook is enabled if the owner/repo has changed
			_, err = model.EnableWebhooks(ctx, mergedProjectRef)
			if err != nil {
				return nil, errors.Wrapf(err, "enabling webhooks for project '%s'", projectId)
			}
			modified = true
		} else if mergedProjectRef.IsEnabled() && !mergedBeforeRef.IsEnabled() {
			if err = handleGithubConflicts(mergedBeforeRef, "Enabling project"); err != nil {
				return nil, err
			}
		} else if mergedProjectRef.Branch != mergedBeforeRef.Branch {
			if err = handleGithubConflicts(mergedBeforeRef, "Changing branch"); err != nil {
				return nil, err
			}
		}

	case model.ProjectPageAccessSection:
		// For any admins that are only in the original settings, remove access.
		// For any admins that are only in the updated settings, give them access.
		adminsToDelete, adminsToAdd := utility.StringSliceSymmetricDifference(mergedBeforeRef.Admins, mergedProjectRef.Admins)
		makeRestricted := !mergedBeforeRef.IsRestricted() && mergedProjectRef.IsRestricted()
		makeUnrestricted := mergedBeforeRef.IsRestricted() && !mergedProjectRef.IsRestricted()
		if isRepo {
			modified = true
			// For repos, we need to use the repo ref functions, as they update different scopes/roles.
			repoRef := &model.RepoRef{ProjectRef: *newProjectRef}
			if err = repoRef.UpdateAdminRoles(adminsToAdd, adminsToDelete); err != nil {
				catcher.Wrap(err, "updating repo admin roles")
			}
			newProjectRef.Admins = repoRef.Admins
			branchProjects, err := model.FindMergedProjectRefsForRepo(repoRef)
			if err != nil {
				return nil, errors.Wrapf(err, "finding branch projects for repo")
			}
			if makeRestricted {
				catcher.Wrap(repoRef.MakeRestricted(branchProjects), "making repo restricted")
			}
			if makeUnrestricted {
				catcher.Wrap(repoRef.MakeUnrestricted(branchProjects), "making repo unrestricted")
			}
		} else {
			if modified, err = newProjectRef.UpdateAdminRoles(adminsToAdd, adminsToDelete); err != nil {
				catcher.Wrap(err, "error updating project admin roles")
				if !modified { // return before we save any admin updates to the project ref collection
					return nil, catcher.Resolve()
				}
			}
			if makeRestricted {
				catcher.Wrap(mergedBeforeRef.MakeRestricted(), "making branch restricted")
				modified = true
			}
			if makeUnrestricted {
				catcher.Wrap(mergedBeforeRef.MakeUnrestricted(), "making branch unrestricted")
				modified = true
			}
		}
	case model.ProjectPageVariablesSection:
		for key, value := range before.Vars.Vars {
			// Private variables are redacted in the UI, so re-set to the real value
			// before updating (assuming the value isn't deleted/re-configured).
			if before.Vars.PrivateVars[key] && changes.Vars.IsPrivate(key) && changes.Vars.Vars[key] == "" {
				changes.Vars.Vars[key] = value
			}
		}
		if err = UpdateProjectVars(projectId, &changes.Vars, true); err != nil { // destructively modifies vars
			return nil, errors.Wrapf(err, "updating project variables for project '%s'", projectId)
		}
		modified = true
	case model.ProjectPageGithubAndCQSection:
		mergedProjectRef.Owner = mergedBeforeRef.Owner
		mergedProjectRef.Repo = mergedBeforeRef.Repo
		mergedProjectRef.Branch = mergedBeforeRef.Branch
		if err = handleGithubConflicts(mergedProjectRef, "Toggling GitHub features"); err != nil {
			return nil, err
		}
		// At project creation we now insert a commit queue, however older projects still may not have one
		// so we need to validate that this exists if the feature is being toggled on.
		if mergedBeforeRef.CommitQueue.IsEnabled() && mergedProjectRef.CommitQueue.IsEnabled() {
			if err = commitqueue.EnsureCommitQueueExistsForProject(mergedProjectRef.Id); err != nil {
				return nil, err
			}
		}
		if err = validateFeaturesHaveAliases(mergedProjectRef, changes.Aliases); err != nil {
			return nil, err
		}
		modified, err = updateAliasesForSection(projectId, changes.Aliases, before.Aliases, section)
		catcher.Add(err)
	case model.ProjectPagePatchAliasSection:
		for i := range mergedProjectRef.PatchTriggerAliases {
			mergedProjectRef.PatchTriggerAliases[i], err = model.ValidateTriggerDefinition(mergedProjectRef.PatchTriggerAliases[i], projectId)
			catcher.Add(err)
		}
		if catcher.HasErrors() {
			return nil, errors.Wrap(catcher.Resolve(), "invalid patch trigger aliases")
		}
		modified, err = updateAliasesForSection(projectId, changes.Aliases, before.Aliases, section)
		catcher.Add(err)
	case model.ProjectPageNotificationsSection:
		if err = SaveSubscriptions(projectId, changes.Subscriptions, true); err != nil {
			return nil, errors.Wrapf(err, "saving subscriptions for project '%s'", projectId)
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
		catcher.Wrapf(DeleteSubscriptions(projectId, toDelete), "deleting subscriptions")
	case model.ProjectPagePeriodicBuildsSection:
		for i := range mergedProjectRef.PeriodicBuilds {
			err = mergedProjectRef.PeriodicBuilds[i].Validate()
			catcher.Add(err)
		}
		if catcher.HasErrors() {
			return nil, errors.Wrap(catcher.Resolve(), "invalid periodic build definition")
		}
	case model.ProjectPageTriggersSection:
		for i := range mergedProjectRef.Triggers {
			err = mergedProjectRef.Triggers[i].Validate(projectId)
			catcher.Add(err)
		}
		if catcher.HasErrors() {
			return nil, errors.Wrap(catcher.Resolve(), "invalid project trigger")
		}
	}
	modifiedProjectRef, err := model.SaveProjectPageForSection(projectId, newProjectRef, section, isRepo)
	if err != nil {
		return nil, errors.Wrapf(err, "defaulting project ref to repo for section '%s'", section)
	}
	res := restModel.APIProjectSettings{}
	if modified || modifiedProjectRef {
		after, err := model.GetProjectSettingsById(projectId, isRepo)
		if err != nil {
			catcher.Wrapf(err, "getting after project settings event")
		} else {
			catcher.Add(model.LogProjectModified(projectId, userId, before, after))
			after.Vars = *after.Vars.RedactPrivateVars() // ensure that we're not returning private variables back to the UI
			res, err = restModel.DbProjectSettingsToRestModel(*after)
			if err != nil {
				catcher.Wrapf(err, "converting project settings")
			}
		}
	}
	return &res, errors.Wrapf(catcher.Resolve(), "saving section '%s'", section)
}

func handleIdentifierConflict(pRef *model.ProjectRef) error {
	conflictingRef, err := model.FindBranchProjectRef(pRef.Identifier)
	if err != nil {
		return errors.Wrapf(err, "checking for conflicting project ref")
	}
	if conflictingRef != nil && conflictingRef.Id != pRef.Id {
		return errors.Errorf("identifier '%s' is already being used for another project", conflictingRef.Id)
	}
	return nil
}

// handleGithubConflicts returns an error containing any potential Github project conflicts.
func handleGithubConflicts(pRef *model.ProjectRef, reason string) error {
	if !pRef.IsPRTestingEnabled() && !pRef.CommitQueue.IsEnabled() && !pRef.IsGithubChecksEnabled() {
		return nil // if nothing is toggled on, then there's no reason to look for conflicts
	}
	conflictMsgs := []string{}
	conflicts, err := pRef.GetGithubProjectConflicts()
	if err != nil {
		return errors.Wrapf(err, "getting GitHub project conflicts")
	}
	if pRef.IsPRTestingEnabled() && len(conflicts.PRTestingIdentifiers) > 0 {
		conflictMsgs = append(conflictMsgs, "PR testing")
	}
	if pRef.CommitQueue.IsEnabled() && len(conflicts.CommitQueueIdentifiers) > 0 {
		conflictMsgs = append(conflictMsgs, "the commit queue")
	}
	if pRef.IsGithubChecksEnabled() && len(conflicts.CommitCheckIdentifiers) > 0 {
		conflictMsgs = append(conflictMsgs, "commit checks")
	}

	if len(conflictMsgs) > 0 {
		return errors.Errorf("%s would create conflicts for %s. Please turn off these settings or address conflicts and try again.",
			reason, strings.Join(conflictMsgs, " and "))
	}
	return nil
}

// DeleteContainerSecrets deletes existing container secrets in the project ref
// from the secrets storage service. This returns the remaining secrets after
// deletion.
func DeleteContainerSecrets(ctx context.Context, v cocoa.Vault, pRef *model.ProjectRef, namesToDelete []string) ([]model.ContainerSecret, error) {
	catcher := grip.NewBasicCatcher()
	var remaining []model.ContainerSecret
	for _, secret := range pRef.ContainerSecrets {
		if !utility.StringSliceContains(namesToDelete, secret.Name) {
			remaining = append(remaining, secret)
			continue
		}

		if secret.ExternalID != "" {
			catcher.Wrapf(v.DeleteSecret(ctx, secret.ExternalID), "deleting container secret '%s' with external ID '%s'", secret.Name, secret.ExternalID)
		}
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return remaining, catcher.Resolve()
}

// UpsertContainerSecrets adds new secrets or updates the value of existing
// container secrets in the secrets storage service for a project. Each
// container secret to upsert must already be stored in the project ref.
func UpsertContainerSecrets(ctx context.Context, v cocoa.Vault, updatedSecrets []model.ContainerSecret) error {
	catcher := grip.NewBasicCatcher()
	for _, updatedSecret := range updatedSecrets {
		if updatedSecret.ExternalID == "" {
			// The secret is not yet stored externally, so create it.
			newSecret := cocoa.NewNamedSecret().
				SetName(updatedSecret.ExternalName).
				SetValue(updatedSecret.Value)
			if _, err := v.CreateSecret(ctx, *newSecret); err != nil {
				catcher.Wrapf(err, "adding new container secret '%s'", updatedSecret.Name)
			}

			continue
		}

		if updatedSecret.Value != "" {
			// The secret already exists but needs to be given a new value, so
			// update the existing secret.
			updatedValue := cocoa.NewNamedSecret().
				SetName(updatedSecret.ExternalID).
				SetValue(updatedSecret.Value)
			catcher.Wrapf(v.UpdateValue(ctx, *updatedValue), "updating value for existing container secret '%s'", updatedSecret.Name)
		}
	}

	return catcher.Resolve()
}
