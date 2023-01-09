package data

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
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
func CopyProject(ctx context.Context, env evergreen.Environment, opts CopyProjectOpts) (*restModel.APIProjectRef, error) {
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
	// Project ID will be validated or generated during CreateProject
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
	isAdmin := u.HasPermission(gimlet.PermissionOpts{
		Resource:      oldId,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	})
	if !isAdmin {
		return nil, errors.Errorf("must be admin of project '%s' to duplicate", oldId)
	}
	if err := CreateProject(ctx, env, projectToCopy, u); err != nil {
		return nil, err
	}
	apiProjectRef := &restModel.APIProjectRef{}
	if err := apiProjectRef.BuildFromService(*projectToCopy); err != nil {
		return nil, errors.Wrap(err, "converting project to API model")
	}

	// Copy variables, aliases, and subscriptions
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
	// Set the same admin roles from the old project on the newly copied project.
	if err := model.UpdateAdminRoles(projectToCopy, projectToCopy.Admins, nil); err != nil {
		catcher.Wrapf(err, "updating admins for project '%s'", opts.NewProjectIdentifier)
	}

	// Since this is a new project we want to log all settings that were copied,
	// so we pass in an empty ProjectSettings struct for the original project state.
	if err := model.GetAndLogProjectModified(projectToCopy.Id, u.Id, false, &model.ProjectSettings{}); err != nil {
		catcher.Wrapf(err, "logging project modified")
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

// PromoteVarsToRepo moves variables from an attached project to its repo.
// Promoted vars are removed from the project as part of this operation.
// Variables whose names already appear in the repo settings will be overwritten.
func PromoteVarsToRepo(projectId string, varNames []string, userId string) error {
	project, err := model.GetProjectSettingsById(projectId, false)
	if err != nil {
		return errors.Wrapf(err, "getting project settings for project '%s'", projectId)
	}

	repoId := project.ProjectRef.RepoRefId

	projectVars, err := model.FindOneProjectVars(projectId)
	if err != nil {
		return errors.Wrapf(err, "getting project variables for project '%s'", projectId)
	}

	repo, err := model.GetProjectSettingsById(repoId, true)
	if err != nil {
		return errors.Wrapf(err, "getting repo settings for repo '%s'", repoId)
	}
	repoVars, err := model.FindOneProjectVars(repoId)
	if err != nil {
		return errors.Wrapf(err, "getting repo variables for repo '%s'", repoId)
	}

	// Add each promoted variable to existing repo vars
	apiRepoVars := &restModel.APIProjectVars{}
	apiRepoVars.BuildFromService(*repoVars)
	for _, varName := range varNames {
		// Ignore nonexistent variables
		if _, contains := projectVars.Vars[varName]; !contains {
			continue
		}
		// Variables promoted from projects will overwrite matching repo variables
		apiRepoVars.Vars[varName] = projectVars.Vars[varName]
		if _, contains := projectVars.PrivateVars[varName]; contains {
			apiRepoVars.PrivateVars[varName] = true
		}
		if _, contains := projectVars.AdminOnlyVars[varName]; contains {
			apiRepoVars.AdminOnlyVars[varName] = true
		}
	}

	if err = UpdateProjectVars(repoId, apiRepoVars, true); err != nil {
		return errors.Wrapf(err, "adding variables from project '%s' to repo", projectId)
	}

	// Log repo update
	repoAfter, err := model.GetProjectSettingsById(repoId, true)
	if err != nil {
		return errors.Wrapf(err, "getting settings for repo '%s' after adding promoted variables", repoId)
	}
	if err = model.LogProjectModified(repoId, userId, repo, repoAfter); err != nil {
		return errors.Wrapf(err, "logging repo '%s' modified", repoId)
	}

	// Remove promoted variables from project
	apiProjectVars := &restModel.APIProjectVars{
		Vars:          map[string]string{},
		PrivateVars:   map[string]bool{},
		AdminOnlyVars: map[string]bool{},
	}
	for key, value := range projectVars.Vars {
		if !utility.StringSliceContains(varNames, key) {
			apiProjectVars.Vars[key] = value
		}
	}

	for key := range projectVars.PrivateVars {
		if _, ok := apiProjectVars.Vars[key]; ok {
			apiProjectVars.PrivateVars[key] = true
		}
	}

	for key := range projectVars.AdminOnlyVars {
		if _, ok := apiProjectVars.Vars[key]; ok {
			apiProjectVars.AdminOnlyVars[key] = true
		}
	}

	if err := UpdateProjectVars(projectId, apiProjectVars, true); err != nil {
		return errors.Wrapf(err, "removing promoted project variables from project '%s'", projectId)
	}

	projectAfter, err := model.GetProjectSettingsById(projectId, false)
	if err != nil {
		return errors.Wrapf(err, "getting settings for project '%s' after removing promoted variables", projectId)
	}
	if err = model.LogProjectModified(projectId, userId, project, projectAfter); err != nil {
		return errors.Wrapf(err, "logging project '%s' modified", projectId)
	}

	return nil
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
	mergedSection, err := model.GetProjectRefMergedWithRepo(*newProjectRef)
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
		if mergedSection.Identifier != mergedBeforeRef.Identifier {
			if err = handleIdentifierConflict(mergedSection); err != nil {
				return nil, err
			}
		}

		// Only need to check Github conflicts once so we use else if statements to handle this.
		// Handle conflicts using the ref from the DB, since only general section settings are passed in from the UI.
		if mergedSection.Owner != mergedBeforeRef.Owner || mergedSection.Repo != mergedBeforeRef.Repo {
			if err = handleGithubConflicts(mergedBeforeRef, "Changing owner/repo"); err != nil {
				return nil, err
			}
			// Check if webhook is enabled if the owner/repo has changed.
			// Using the new project ref ensures we update tracking at the end.
			_, err = model.EnableWebhooks(ctx, newProjectRef)
			if err != nil {
				return nil, errors.Wrapf(err, "enabling webhooks for project '%s'", projectId)
			}
			modified = true
		} else if mergedSection.IsEnabled() && !mergedBeforeRef.IsEnabled() {
			if err = handleGithubConflicts(mergedBeforeRef, "Enabling project"); err != nil {
				return nil, err
			}
		} else if mergedSection.Branch != mergedBeforeRef.Branch {
			if err = handleGithubConflicts(mergedBeforeRef, "Changing branch"); err != nil {
				return nil, err
			}
		}

	case model.ProjectPageAccessSection:
		// For any admins that are only in the original settings, remove access.
		// For any admins that are only in the updated settings, give them access.
		adminsToDelete, adminsToAdd := utility.StringSliceSymmetricDifference(mergedBeforeRef.Admins, mergedSection.Admins)
		makeRestricted := !mergedBeforeRef.IsRestricted() && mergedSection.IsRestricted()
		makeUnrestricted := mergedBeforeRef.IsRestricted() && !mergedSection.IsRestricted()
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
		mergedSection.Owner = mergedBeforeRef.Owner
		mergedSection.Repo = mergedBeforeRef.Repo
		mergedSection.Branch = mergedBeforeRef.Branch
		if err = handleGithubConflicts(mergedSection, "Toggling GitHub features"); err != nil {
			return nil, err
		}
		// At project creation we now insert a commit queue, however older projects still may not have one
		// so we need to validate that this exists if the feature is being toggled on.
		if mergedBeforeRef.CommitQueue.IsEnabled() && mergedSection.CommitQueue.IsEnabled() {
			if err = commitqueue.EnsureCommitQueueExistsForProject(mergedSection.Id); err != nil {
				return nil, err
			}
		}
		if err = validateFeaturesHaveAliases(mergedBeforeRef, mergedSection, changes.Aliases); err != nil {
			return nil, err
		}
		modified, err = updateAliasesForSection(projectId, changes.Aliases, before.Aliases, section)
		catcher.Add(err)
	case model.ProjectPagePatchAliasSection:
		for i := range mergedSection.PatchTriggerAliases {
			mergedSection.PatchTriggerAliases[i], err = model.ValidateTriggerDefinition(mergedSection.PatchTriggerAliases[i], projectId)
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
	case model.ProjectPageContainerSection:
		for i := range mergedSection.ContainerSizeDefinitions {
			err = mergedSection.ContainerSizeDefinitions[i].Validate(evergreen.GetEnvironment().Settings().Providers.AWS.Pod.ECS)
			catcher.Add(err)
		}
		if catcher.HasErrors() {
			return nil, errors.Wrap(catcher.Resolve(), "invalid container size definition")
		}
	case model.ProjectPagePeriodicBuildsSection:
		for i := range mergedSection.PeriodicBuilds {
			err = mergedSection.PeriodicBuilds[i].Validate()
			catcher.Add(err)
		}
		if catcher.HasErrors() {
			return nil, errors.Wrap(catcher.Resolve(), "invalid periodic build definition")
		}
	case model.ProjectPageTriggersSection:
		for i := range mergedSection.Triggers {
			err = mergedSection.Triggers[i].Validate(projectId)
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

// getCopiedContainerSecrets gets a copy of an existing set of container
// secrets. It returns the new secrets to create.
func getCopiedContainerSecrets(ctx context.Context, settings *evergreen.Settings, v cocoa.Vault, projectID string, toCopy []model.ContainerSecret) ([]model.ContainerSecret, error) {
	if projectID == "" {
		return nil, errors.New("cannot copy container secrets without a project ID")
	}

	var copied []model.ContainerSecret
	catcher := grip.NewBasicCatcher()

	for _, original := range toCopy {
		if original.ExternalID == "" {
			// It's not possible to replicate a project secret without an
			// external ID to get its value.
			continue
		}
		if original.Type == model.ContainerSecretPodSecret {
			// Generate a new pod secret rather than copy the existing one.
			// Since users don't rely on this directly, it's preferable to have
			// different pod secrets between projects.
			continue
		}

		val, err := v.GetValue(ctx, original.ExternalID)
		if err != nil {
			catcher.Wrapf(err, "getting value for container secret '%s'", original.Name)
			continue
		}

		// Make a new secret that will be stored as a copy of the original.
		updated := original
		updated.Value = val

		copied = append(copied, updated)
	}

	if catcher.HasErrors() {
		return nil, errors.Wrap(catcher.Resolve(), "copying container secrets")
	}

	copied = append(copied, newPodSecret())

	validated, err := model.ValidateContainerSecrets(settings, projectID, nil, copied)
	if err != nil {
		return nil, errors.Wrap(err, "validating new container secrets")
	}

	return validated, nil
}

// newPodSecret returns a new default pod secret with a random value to be
// stored.
func newPodSecret() model.ContainerSecret {
	return model.ContainerSecret{
		Name:  pod.PodSecretEnvVar,
		Type:  model.ContainerSecretPodSecret,
		Value: utility.RandomString(),
	}
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
