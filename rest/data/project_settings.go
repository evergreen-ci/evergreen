package data

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// This file is used to combine operations across data connectors, to avoid
// duplicated connector usage across the codebase.

// CopyProject copies the passed in project with the given project identifier, and returns the new project.
func CopyProject(ctx context.Context, env evergreen.Environment, opts restModel.CopyProjectOpts) (*restModel.APIProjectRef, error) {
	projectToCopy, err := FindProjectById(ctx, opts.ProjectIdToCopy, false, false)
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

	catcher := grip.NewBasicCatcher()
	u := gimlet.GetUser(ctx).(*user.DBUser)
	created, err := CreateProject(ctx, env, projectToCopy, u)
	if err != nil {
		if !created {
			return nil, err
		}
		catcher.Add(err)
	}
	apiProjectRef := &restModel.APIProjectRef{}
	if err := apiProjectRef.BuildFromService(ctx, *projectToCopy); err != nil {
		return nil, errors.Wrap(err, "converting project to API model")
	}

	// Copy variables, aliases, and subscriptions
	if err := model.CopyProjectVars(ctx, oldId, projectToCopy.Id); err != nil {
		catcher.Wrapf(err, "copying project vars from project '%s'", oldIdentifier)
	}
	if err := model.CopyProjectAliases(ctx, oldId, projectToCopy.Id); err != nil {
		catcher.Wrapf(err, "copying aliases from project '%s'", oldIdentifier)
	}
	if err := event.CopyProjectSubscriptions(ctx, oldId, projectToCopy.Id); err != nil {
		catcher.Wrapf(err, "copying subscriptions from project '%s'", oldIdentifier)
	}
	// Set the same admin roles from the old project on the newly copied project.
	if err := model.UpdateAdminRoles(ctx, projectToCopy, projectToCopy.Admins, nil); err != nil {
		catcher.Wrapf(err, "updating admins for project '%s'", opts.NewProjectIdentifier)
	}

	// Since this is a new project we want to log all settings that were copied,
	// so we pass in an empty ProjectSettings struct for the original project state.
	if err := model.GetAndLogProjectModified(ctx, projectToCopy.Id, u.Id, false, &model.ProjectSettings{}); err != nil {
		catcher.Wrapf(err, "logging project modified")
	}
	// Since the errors above are nonfatal and still permit copying the project, return both the new project and any errors that were encountered.
	return apiProjectRef, catcher.Resolve()
}

func disableStartingSettings(p *model.ProjectRef) {
	p.Enabled = false
	p.PRTestingEnabled = utility.FalsePtr()
	p.ManualPRTestingEnabled = utility.FalsePtr()
	p.GithubChecksEnabled = utility.FalsePtr()
	p.CommitQueue.Enabled = utility.FalsePtr()
}

// PromoteVarsToRepo moves variables from an attached project to its repo.
// Promoted vars are removed from the project as part of this operation.
// Variables whose names already appear in the repo settings will be overwritten.
func PromoteVarsToRepo(ctx context.Context, projectIdentifier string, varNames []string, userId string) error {
	project, err := model.GetProjectSettingsById(ctx, projectIdentifier, false)
	if err != nil {
		return errors.Wrapf(err, "getting project settings for project '%s'", projectIdentifier)
	}

	projectId := project.ProjectRef.Id
	repoId := project.ProjectRef.RepoRefId

	projectVars, err := model.FindOneProjectVars(ctx, projectId)
	if err != nil {
		return errors.Wrapf(err, "getting project variables for project '%s'", projectIdentifier)
	}

	repo, err := model.GetProjectSettingsById(ctx, repoId, true)
	if err != nil {
		return errors.Wrapf(err, "getting repo settings for repo '%s'", repoId)
	}
	repoVars, err := model.FindOneProjectVars(ctx, repoId)
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

	if err = UpdateProjectVars(ctx, repoId, apiRepoVars, true); err != nil {
		return errors.Wrapf(err, "adding variables from project '%s' to repo", projectIdentifier)
	}

	// Log repo update
	repoAfter, err := model.GetProjectSettingsById(ctx, repoId, true)
	if err != nil {
		return errors.Wrapf(err, "getting settings for repo '%s' after adding promoted variables", repoId)
	}
	if err = model.LogProjectModified(ctx, repoId, userId, repo, repoAfter); err != nil {
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

	if err := UpdateProjectVars(ctx, projectId, apiProjectVars, true); err != nil {
		return errors.Wrapf(err, "removing promoted project variables from project '%s'", projectIdentifier)
	}

	projectAfter, err := model.GetProjectSettingsById(ctx, projectId, false)
	if err != nil {
		return errors.Wrapf(err, "getting settings for project '%s' after removing promoted variables", projectIdentifier)
	}
	if err = model.LogProjectModified(ctx, projectId, userId, project, projectAfter); err != nil {
		return errors.Wrapf(err, "logging project '%s' modified", projectIdentifier)
	}

	return nil
}

// SaveProjectSettingsForSection saves the given UI page section and logs it for the given user. If isRepo is true, uses
// RepoRef related functions and collection instead of ProjectRef.
func SaveProjectSettingsForSection(ctx context.Context, projectId string, changes *restModel.APIProjectSettings,
	section model.ProjectPageSection, isRepo bool, userId string) (*restModel.APIProjectSettings, error) {
	before, err := model.GetProjectSettingsById(ctx, projectId, isRepo)
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
	mergedSection, err := model.GetProjectRefMergedWithRepo(ctx, *newProjectRef)
	if err != nil {
		return nil, errors.Wrapf(err, "getting merged project ref")
	}

	// mergedBeforeRef represents the original merged project ref (i.e. the project ref without any edits).
	mergedBeforeRef, err := model.GetProjectRefMergedWithRepo(ctx, before.ProjectRef)
	if err != nil {
		return nil, errors.Wrap(err, "getting the original merged project ref")
	}
	if mergedSection.IsHidden() {
		return nil, errors.New("can't update a hidden project")
	}

	catcher := grip.NewBasicCatcher()
	modified := false
	switch section {
	case model.ProjectPageGeneralSection:
		if mergedSection.Identifier != mergedBeforeRef.Identifier {
			if err = validateModifiedIdentifier(ctx, mergedSection); err != nil {
				return nil, err
			}
		}

		if err = mergedSection.ValidateEnabledRepotracker(); err != nil {
			return nil, err
		}
		// Validate owner/repo if the project is enabled or owner/repo is populated.
		// This validation is cheap so it makes sense to be strict about this.
		if mergedSection.Enabled || (mergedSection.Owner != "" && mergedSection.Repo != "") {
			config, err := evergreen.GetConfig(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "getting evergreen config")
			}
			if err = mergedSection.ValidateOwnerAndRepo(config.GithubOrgs); err != nil {
				return nil, errors.Wrap(err, "validating new owner/repo")
			}
		}
		// Only need to check GitHub conflicts once so we use else if statements to handle this.
		// Handle conflicts using the ref from the DB, since only general section settings are passed in from the UI.
		if mergedSection.Owner != mergedBeforeRef.Owner || mergedSection.Repo != mergedBeforeRef.Repo {
			if err = handleGithubConflicts(ctx, mergedBeforeRef, "Changing owner/repo"); err != nil {
				return nil, err
			}
			// Check if webhook is enabled if the owner/repo has changed.
			// Using the new project ref ensures we update tracking at the end.
			_, err = model.SetTracksPushEvents(ctx, newProjectRef)
			if err != nil {
				return nil, errors.Wrapf(err, "setting project tracks push events for project '%s' in '%s/%s'", projectId, newProjectRef.Owner, newProjectRef.Repo)
			}
			modified = true
		} else if mergedSection.Enabled && !mergedBeforeRef.Enabled {
			if err = handleGithubConflicts(ctx, mergedBeforeRef, "Enabling project"); err != nil {
				return nil, err
			}
		} else if mergedSection.Branch != mergedBeforeRef.Branch {
			if err = handleGithubConflicts(ctx, mergedBeforeRef, "Changing branch"); err != nil {
				return nil, err
			}
		}

		if mergedSection.Enabled {
			// Branches are not defined for repos, so only check that enabled projects have one specified.
			if mergedSection.Branch == "" && !isRepo {
				return nil, errors.New("branch not set on enabled project")
			}

			config, err := evergreen.GetConfig(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "getting evergreen config")
			}
			_, err = model.ValidateEnabledProjectsLimit(ctx, config, mergedBeforeRef, mergedSection)
			if err != nil {
				return nil, errors.Wrap(err, "validating project creation")
			}
		}
	case model.ProjectPagePluginSection:
		for _, link := range mergedSection.ExternalLinks {
			if link.DisplayName != "" && link.URLTemplate != "" {
				// check length of link display name
				if len(link.DisplayName) > 40 {
					catcher.Add(errors.New(fmt.Sprintf("link display name, %s, must be 40 characters or less", link.DisplayName)))
				}
				// validate url template
				formattedURL := strings.Replace(link.URLTemplate, "{version_id}", "version_id", -1)
				if _, err := url.ParseRequestURI(formattedURL); err != nil {
					catcher.Add(err)
				}
			}
		}
		if catcher.HasErrors() {
			return nil, errors.Wrapf(catcher.Resolve(), "validating external links")
		}

		// If we are trying to enable the performance plugin but the project's id and identifier are
		// different, we should error. The performance plugin requires matching id and identifier.
		if !mergedBeforeRef.IsPerfEnabled() && mergedSection.IsPerfEnabled() {
			if projectId != mergedBeforeRef.Identifier {
				return nil, errors.Errorf("cannot enable performance plugin for project '%s' because project ID and identifier do not match", mergedBeforeRef.Identifier)
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
			if err = repoRef.UpdateAdminRoles(ctx, adminsToAdd, adminsToDelete); err != nil {
				catcher.Wrap(err, "updating repo admin roles")
			}
			newProjectRef.Admins = repoRef.Admins
			branchProjects, err := model.FindMergedProjectRefsForRepo(ctx, repoRef)
			if err != nil {
				return nil, errors.Wrapf(err, "finding branch projects for repo")
			}
			if makeRestricted {
				catcher.Wrap(repoRef.MakeRestricted(ctx, branchProjects), "making repo restricted")
			}
			if makeUnrestricted {
				catcher.Wrap(repoRef.MakeUnrestricted(ctx, branchProjects), "making repo unrestricted")
			}
		} else {
			if modified, err = newProjectRef.UpdateAdminRoles(ctx, adminsToAdd, adminsToDelete); err != nil {
				catcher.Wrap(err, "error updating project admin roles")
				if !modified { // return before we save any admin updates to the project ref collection
					return nil, catcher.Resolve()
				}
			}
			if makeRestricted {
				catcher.Wrap(mergedBeforeRef.MakeRestricted(ctx), "making branch restricted")
				modified = true
			}
			if makeUnrestricted {
				catcher.Wrap(mergedBeforeRef.MakeUnrestricted(ctx), "making branch unrestricted")
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
		if err = UpdateProjectVars(ctx, projectId, &changes.Vars, true); err != nil { // destructively modifies vars
			return nil, errors.Wrapf(err, "updating project variables for project '%s'", projectId)
		}
		modified = true
	case model.ProjectPageGithubAndCQSection:
		mergedSection.Owner = mergedBeforeRef.Owner
		mergedSection.Repo = mergedBeforeRef.Repo
		mergedSection.Branch = mergedBeforeRef.Branch
		if err = handleGithubConflicts(ctx, mergedSection, "Toggling GitHub features"); err != nil {
			return nil, err
		}

		if err = validateFeaturesHaveAliases(ctx, mergedBeforeRef, mergedSection, changes.Aliases); err != nil {
			return nil, err
		}
		if err = mergedSection.ValidateGitHubPermissionGroupsByRequester(); err != nil {
			return nil, err
		}
		modified, err = updateAliasesForSection(ctx, projectId, changes.Aliases, before.Aliases, section)
		catcher.Add(err)
	case model.ProjectPagePatchAliasSection:
		for i := range mergedSection.PatchTriggerAliases {
			mergedSection.PatchTriggerAliases[i], err = model.ValidateTriggerDefinition(ctx, mergedSection.PatchTriggerAliases[i], projectId)
			catcher.Add(err)
		}
		if catcher.HasErrors() {
			return nil, errors.Wrap(catcher.Resolve(), "invalid patch trigger aliases")
		}
		modified, err = updateAliasesForSection(ctx, projectId, changes.Aliases, before.Aliases, section)
		catcher.Add(err)
	case model.ProjectPageNotificationsSection:
		// Some subscription values are redacted like webhook secret and 'Authorization' header.
		// Before saving to the database, we should unredact all of these, referencing the before
		// as the unredacted values.
		unredactedSubscriptions, err := getUnredactedSubscriptions(before.Subscriptions, changes.Subscriptions)
		if err != nil {
			return nil, errors.Wrap(err, "unredacting subscriptions")
		}
		if err = SaveSubscriptions(ctx, projectId, unredactedSubscriptions, true); err != nil {
			return nil, errors.Wrapf(err, "saving subscriptions for project '%s'", projectId)
		}
		modified = true
		subscriptionsToKeep := []string{}
		for _, s := range unredactedSubscriptions {
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
		catcher.Wrapf(DeleteSubscriptions(ctx, projectId, toDelete), "deleting subscriptions")
	case model.ProjectPagePeriodicBuildsSection:
		for i := range mergedSection.PeriodicBuilds {
			err = mergedSection.PeriodicBuilds[i].Validate()
			catcher.Add(err)
		}
		if catcher.HasErrors() {
			return nil, errors.Wrap(catcher.Resolve(), "invalid periodic build definition")
		}
	case model.ProjectPageTriggersSection:
		if !isRepo { // Check this for project refs only, as repo projects won't have last version information stored.
			repository, err := model.FindRepository(ctx, projectId)
			if err != nil {
				return nil, errors.Wrapf(err, "finding repository for project '%s'", projectId)
			}
			if repository == nil {
				catcher.New("project must have existing versions in order to trigger versions")
			}
		}

		for i := range mergedSection.Triggers {
			catcher.Add(mergedSection.Triggers[i].Validate(ctx, projectId))
		}
		if catcher.HasErrors() {
			return nil, errors.Wrap(catcher.Resolve(), "invalid project trigger")
		}
	case model.ProjectPageViewsAndFiltersSection:
		if err = parsley.ValidateFilters(mergedSection.ParsleyFilters); err != nil {
			return nil, errors.Wrap(err, "invalid Parsley filters")
		}
	// This section does not support repo-level at this time.
	case model.ProjectPageGithubAppSettingsSection:
		mergedSection.Id = mergedBeforeRef.Id

		appIDChanged := changes.GithubAppAuth.AppID != int(before.GitHubAppAuth.AppID)
		privateKeyChanged := utility.FromStringPtr(changes.GithubAppAuth.PrivateKey) != string(before.GitHubAppAuth.PrivateKey)
		privateKeyRedacted := utility.FromStringPtr(changes.GithubAppAuth.PrivateKey) == evergreen.RedactedValue

		if appIDChanged || privateKeyChanged {
			// The UI only ever sees the private key as the {REDACTED} string and will include it in calls to
			// this function. To avoid overwriting the actual private key, we should not update the credentials
			// if the private key is equal to the {REDACTED} placeholder.
			if !privateKeyRedacted {
				if err = mergedSection.SetGithubAppCredentials(ctx, int64(changes.GithubAppAuth.AppID), []byte(utility.FromStringPtr(changes.GithubAppAuth.PrivateKey))); err != nil {
					return nil, errors.Wrap(err, "updating GitHub app credentials")
				}
			}
		}
		mergedSection.GitHubDynamicTokenPermissionGroups = mergedBeforeRef.GitHubDynamicTokenPermissionGroups
		if err = mergedSection.ValidateGitHubPermissionGroupsByRequester(); err != nil {
			return nil, errors.Wrap(err, "invalid GitHub permission group by requester")
		}
		modified = true
	case model.ProjectPageGithubPermissionsSection:
		if err = mergedSection.ValidateGitHubPermissionGroups(); err != nil {
			return nil, errors.Wrap(err, "invalid GitHub permission groups")
		}
		modified = true
	}

	modifiedProjectRef, err := model.SaveProjectPageForSection(ctx, projectId, newProjectRef, section, isRepo)
	if err != nil {
		return nil, errors.Wrapf(err, "saving project for section '%s'", section)
	}
	res := restModel.APIProjectSettings{}
	if modified || modifiedProjectRef {
		after, err := model.GetProjectSettingsById(ctx, projectId, isRepo)
		if err != nil {
			catcher.Wrapf(err, "getting after project settings event")
		} else {
			catcher.Add(model.LogProjectModified(ctx, projectId, userId, before, after))
			after.Vars = *after.Vars.RedactPrivateVars() // ensure that we're not returning private variables back to the UI
			after.GitHubAppAuth = *after.GitHubAppAuth.RedactPrivateKey()
			res, err = restModel.DbProjectSettingsToRestModel(ctx, *after)
			if err != nil {
				catcher.Wrapf(err, "converting project settings")
			}
		}
	}

	// If we're just enabled the project, we should run the repotracker to kick things off.
	if modifiedProjectRef && mergedSection.Enabled && !mergedBeforeRef.Enabled {
		ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
		j := units.NewRepotrackerJob(fmt.Sprintf("project-enabled-%s", ts), projectId)

		queue := evergreen.GetEnvironment().RemoteQueue()
		if err := amboy.EnqueueUniqueJob(ctx, queue, j); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message": "problem enqueueing repotracker job for enabled project",
				"project": projectId,
				"owner":   mergedSection.Owner,
				"repo":    mergedSection.Repo,
				"branch":  mergedSection.Branch,
			}))
		}
	}

	debugSpawnHostsJustDisabled := modifiedProjectRef &&
		section == model.ProjectPageGeneralSection &&
		utility.FromBoolTPtr(mergedSection.DebugSpawnHostsDisabled) &&
		!utility.FromBoolTPtr(mergedBeforeRef.DebugSpawnHostsDisabled)

	if debugSpawnHostsJustDisabled {
		terminateDebugHostsForProject(ctx, projectId, userId)
	}

	return &res, errors.Wrapf(catcher.Resolve(), "saving section '%s'", section)
}

// terminateDebugHostsForProject finds all running debug hosts for the given
// project and enqueues termination jobs for each one.
func terminateDebugHostsForProject(ctx context.Context, projectId, userId string) {
	debugHosts, err := host.FindTerminatableDebugHostsForProject(ctx, projectId)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "problem finding debug hosts to terminate after disabling debug spawn hosts setting",
			"project": projectId,
		}))
		return
	}
	if len(debugHosts) == 0 {
		return
	}

	queue := evergreen.GetEnvironment().RemoteQueue()
	ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
	catcher := grip.NewBasicCatcher()

	for _, h := range debugHosts {
		terminationJob := units.NewSpawnHostTerminationJob(&h, userId, ts, evergreen.ModifySpawnHostProjectSettings)
		if err := amboy.EnqueueUniqueJob(ctx, queue, terminationJob); err != nil {
			catcher.Wrapf(err, "enqueueing termination job for debug host '%s'", h.Id)
		}
	}

	if catcher.HasErrors() {
		grip.Error(message.WrapError(catcher.Resolve(), message.Fields{
			"message":    "problem enqueueing debug host termination jobs after disabling debug spawn hosts setting",
			"project":    projectId,
			"num_hosts":  len(debugHosts),
			"num_errors": catcher.Len(),
		}))
	} else {
		grip.Info(message.Fields{
			"message":   "enqueued debug host termination jobs after disabling debug spawn hosts setting",
			"project":   projectId,
			"num_hosts": len(debugHosts),
		})
	}
}

// getUnredactedSubscriptions parses the new subscriptions for any values that are redacted and if they are redacted,
// it replaces the placeholder string with the unredacted value.
func getUnredactedSubscriptions(unredactedPreviousSubscriptions []event.Subscription, redactedNewSubscriptions []restModel.APISubscription) ([]restModel.APISubscription, error) {
	unredactedNewSubscriptions := []restModel.APISubscription{}
	for _, redactedSub := range redactedNewSubscriptions {
		// Apply all subscription unredaction functions.
		err := unredactWebhookSubscription(unredactedPreviousSubscriptions, &redactedSub)
		if err != nil {
			return nil, errors.Wrapf(err, "unredacting webhook subscription")
		}
		// Add to final subscription slice.
		unredactedNewSubscriptions = append(unredactedNewSubscriptions, redactedSub)
	}
	return unredactedNewSubscriptions, nil
}

// unredactWebhookSubscription unredacts the webhook secret and the Authorization header for the subscription.
// If the given subscription isn't a webhook one, it no-ops.
func unredactWebhookSubscription(unredactedPreviousSubscriptions []event.Subscription, redactedNewSubscription *restModel.APISubscription) error {
	redactedNewWebhookSubscription := redactedNewSubscription.Subscriber.WebhookSubscriber
	// If no webhook is present, we can no-op.
	if redactedNewWebhookSubscription == nil {
		return nil
	}

	// Get the unredacted webhook subscription.
	unredactedPreviousWebhookSubscription, err := getSubscription[event.WebhookSubscriber](unredactedPreviousSubscriptions, utility.FromStringPtr(redactedNewSubscription.ID))
	if err != nil || unredactedPreviousWebhookSubscription == nil {
		return err
	}

	// Unredact the secret if it was redacted.
	if utility.FromStringPtr(redactedNewWebhookSubscription.Secret) == evergreen.RedactedValue {
		redactedNewWebhookSubscription.Secret = utility.ToStringPtr(string(unredactedPreviousWebhookSubscription.Secret))
	}

	// Unredact the Authorization header if it was redacted.
	unredactedNewHeaders := []restModel.APIWebhookHeader{}
	for _, redactedHeader := range redactedNewWebhookSubscription.Headers {
		// If the header is the Authorization header and set to redacted, replace it with the unredacted value.
		if utility.FromStringPtr(redactedHeader.Key) == "Authorization" && utility.FromStringPtr(redactedHeader.Value) == evergreen.RedactedValue {
			redactedHeader.Value = utility.ToStringPtr(unredactedPreviousWebhookSubscription.GetHeader("Authorization"))
		}
		unredactedNewHeaders = append(unredactedNewHeaders, redactedHeader)
	}
	redactedNewWebhookSubscription.Headers = unredactedNewHeaders

	return nil
}

// getSubscription gets a subscription by id and asserts a type on it. If
// the type does not match for that id, it returns an error.
func getSubscription[T any](subscriptions []event.Subscription, id string) (*T, error) {
	for _, subscription := range subscriptions {
		if subscription.ID == id {
			return event.GetSubscriptionTarget[T](subscription)
		}
	}
	return nil, nil
}

func validateModifiedIdentifier(ctx context.Context, pRef *model.ProjectRef) error {
	conflictingRef, err := model.FindBranchProjectRef(ctx, pRef.Identifier)
	if err != nil {
		return errors.Wrapf(err, "checking for conflicting project ref")
	}
	if conflictingRef != nil && conflictingRef.Id != pRef.Id {
		return errors.Errorf("identifier '%s' is already being used for another project", conflictingRef.Id)
	}
	if !projectIDRegexp.MatchString(pRef.Identifier) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("project identifier '%s' contains invalid characters", pRef.Identifier),
		}
	}
	return nil
}

// handleGithubConflicts returns an error containing any potential Github project conflicts.
func handleGithubConflicts(ctx context.Context, pRef *model.ProjectRef, reason string) error {
	if !pRef.IsPRTestingEnabled() && !pRef.CommitQueue.IsEnabled() && !pRef.IsGithubChecksEnabled() {
		return nil // if nothing is toggled on, then there's no reason to look for conflicts
	}
	conflictMsgs := []string{}
	conflicts, err := pRef.GetGithubProjectConflicts(ctx)
	if err != nil {
		return errors.Wrapf(err, "getting GitHub project conflicts")
	}
	if pRef.IsPRTestingEnabled() && len(conflicts.PRTestingIdentifiers) > 0 {
		conflictingIdentifiers := strings.Join(conflicts.PRTestingIdentifiers, ", ")
		conflictMsgs = append(conflictMsgs, fmt.Sprintf("PR testing (projects: %s)", conflictingIdentifiers))
	}
	if pRef.CommitQueue.IsEnabled() && len(conflicts.CommitQueueIdentifiers) > 0 {
		conflictingIdentifiers := strings.Join(conflicts.CommitQueueIdentifiers, ", ")
		conflictMsgs = append(conflictMsgs, fmt.Sprintf("the commit queue (projects: %s)", conflictingIdentifiers))
	}
	if pRef.IsGithubChecksEnabled() && len(conflicts.CommitCheckIdentifiers) > 0 {
		conflictingIdentifiers := strings.Join(conflicts.CommitCheckIdentifiers, ", ")
		conflictMsgs = append(conflictMsgs, fmt.Sprintf("commit checks (projects: %s)", conflictingIdentifiers))
	}

	if len(conflictMsgs) > 0 {
		return errors.Errorf("%s would create conflicts for %s. Please turn off these settings or address conflicts and try again.",
			reason, strings.Join(conflictMsgs, " and "))
	}
	return nil
}
