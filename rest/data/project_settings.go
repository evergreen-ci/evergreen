package data

import (
	"context"
	"fmt"
	"net/http"
	"strings"

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

type CopyProjectOpts struct {
	ProjectIdToCopy      string
	NewProjectIdentifier string
	NewProjectId         string
}

func (sc *DBConnector) CopyProject(ctx context.Context, opts CopyProjectOpts) (*restModel.APIProjectRef, error) {
	projectToCopy, err := sc.FindProjectById(opts.ProjectIdToCopy, false, false)
	if err != nil {
		return nil, errors.Wrapf(err, "Database error finding project '%s'", opts.ProjectIdToCopy)
	}
	if projectToCopy == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project '%s' doesn't exist", opts.ProjectIdToCopy),
		}
	}

	oldId := projectToCopy.Id
	// project ID will be validated or generated during CreateProject
	if opts.NewProjectId != "" {
		projectToCopy.Id = opts.NewProjectId
	} else {
		projectToCopy.Id = ""
	}

	// copy project, disable necessary settings
	oldIdentifier := projectToCopy.Identifier
	projectToCopy.Identifier = opts.NewProjectIdentifier
	projectToCopy.Enabled = utility.FalsePtr()
	projectToCopy.PRTestingEnabled = utility.FalsePtr()
	projectToCopy.ManualPRTestingEnabled = utility.FalsePtr()
	projectToCopy.CommitQueue.Enabled = nil
	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err := sc.CreateProject(projectToCopy, u); err != nil {
		return nil, err
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
		return nil, errors.Wrapf(err, "Database error updating admins for project '%s'", opts.NewProjectIdentifier)
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

	// If the project ref doesn't use the repo, or we're using a repo ref, then this will just be the same as the passed in ref.
	// Used to verify that if something is set to nil, we properly validate using the merged project ref.
	mergedProjectRef, err := model.GetProjectRefMergedWithRepo(*newProjectRef)
	if err != nil {
		return nil, errors.Wrapf(err, "error merging project ref")
	}
	mergedBeforeRef, err := model.GetProjectRefMergedWithRepo(before.ProjectRef)
	if err != nil {
		return nil, errors.Wrap(err, "error getting the original merged project ref")
	}

	catcher := grip.NewBasicCatcher()
	modified := false
	switch section {
	case model.ProjectPageGeneralSection:
		// only need to check Github conflicts once so we use else if statements to handle this
		if mergedProjectRef.Owner != mergedBeforeRef.Owner || mergedProjectRef.Repo != mergedBeforeRef.Repo {
			if err = handleGithubConflicts(mergedProjectRef, "Changing owner/repo"); err != nil {
				return nil, err
			}
			// check if webhook is enabled if the owner/repo has changed
			_, err = sc.EnableWebhooks(ctx, mergedProjectRef)
			if err != nil {
				return nil, errors.Wrapf(err, "Error enabling webhooks for project '%s'", projectId)
			}
			modified = true
		} else if mergedProjectRef.IsEnabled() && !mergedBeforeRef.IsEnabled() {
			if err = handleGithubConflicts(mergedProjectRef, "Enabling project"); err != nil {
				return nil, err
			}
		} else if mergedProjectRef.Branch != mergedBeforeRef.Branch {
			if err = handleGithubConflicts(mergedProjectRef, "Changing branch"); err != nil {
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
			if modified, err = newProjectRef.UpdateAdminRoles(adminsToAdd, adminsToDelete); err != nil {
				catcher.Wrap(err, "error updating project admin roles")
				if !modified { // return before we save any admin updates to the project ref collection
					return nil, catcher.Resolve()
				}
			}
			if makeRestricted {
				catcher.Wrap(mergedBeforeRef.MakeRestricted(), "error making branch restricted")
				modified = true
			}
			if makeUnrestricted {
				catcher.Wrap(mergedBeforeRef.MakeUnrestricted(), "error making branch unrestricted")
				modified = true
			}
		}
	case model.ProjectPageVariablesSection:
		for key, value := range before.Vars.Vars {
			// Private variables are redacted in the UI, so re-set to the real value
			// before updating (assuming the value isn't deleted).
			if before.Vars.PrivateVars[key] && changes.Vars.PrivateVars[key] {
				changes.Vars.Vars[key] = value
			}
		}
		if err = sc.UpdateProjectVars(projectId, &changes.Vars, true); err != nil { // destructively modifies vars
			return nil, errors.Wrapf(err, "Database error updating variables for project '%s'", projectId)
		}
		modified = true
	case model.ProjectPageGithubAndCQSection:
		if err = handleGithubConflicts(mergedProjectRef, "Toggling GitHub features"); err != nil {
			return nil, err
		}

		modified, err = sc.UpdateAliasesForSection(projectId, changes.Aliases, before.Aliases, section)
		catcher.Add(err)
	case model.ProjectPagePatchAliasSection:
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
			after.Vars = *after.Vars.RedactPrivateVars() // ensure that we're not returning private variables back to the UI
			res, err = restModel.DbProjectSettingsToRestModel(*after)
			if err != nil {
				catcher.Wrapf(err, "error converting project settings")
			}
		}
	}
	return &res, errors.Wrapf(catcher.Resolve(), "error saving section '%s'", section)
}

func handleGithubConflicts(pRef *model.ProjectRef, reason string) error {
	if !pRef.IsPRTestingEnabled() && !pRef.CommitQueue.IsEnabled() && !pRef.IsGithubChecksEnabled() {
		return nil // if nothing is toggled on, then there's no reason to look for conflicts
	}
	conflictMsgs := []string{}
	conflicts, err := pRef.GetGithubProjectConflicts()
	if err != nil {
		return errors.Wrapf(err, "error getting github project conflicts")
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
