package route

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/repos/{repo_id}

type repoIDGetHandler struct {
	repoName string
	sc       data.Connector
}

func makeGetRepoByID(sc data.Connector) gimlet.RouteHandler {
	return &repoIDGetHandler{
		sc: sc,
	}
}

func (h *repoIDGetHandler) Factory() gimlet.RouteHandler {
	return &repoIDGetHandler{
		sc: h.sc,
	}
}

func (h *repoIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.repoName = gimlet.GetVars(r)["repo_id"]
	return nil
}

func (h *repoIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	repo, err := dbModel.FindOneRepoRef(h.repoName)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	if repo == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("repo '%s' doesn't exist", h.repoName))
	}

	repoModel := &model.APIProjectRef{}
	if err = repoModel.BuildFromService(repo.ProjectRef); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "problem converting repo document"))
	}

	repoVars, err := h.sc.FindProjectVarsById("", repo.Id, true)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	repoModel.Variables = *repoVars

	if repoModel.Aliases, err = h.sc.FindProjectAliases("", repo.Id, nil); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(repoModel)
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/repos/{repo_id}

type repoIDPatchHandler struct {
	repoName      string
	originalRepo  *dbModel.RepoRef
	newRepoRef    *dbModel.RepoRef
	apiNewRepoRef *model.APIProjectRef
	user          *user.DBUser

	sc       data.Connector
	settings *evergreen.Settings
}

func makePatchRepoByID(sc data.Connector, settings *evergreen.Settings) gimlet.RouteHandler {
	return &repoIDPatchHandler{
		sc:       sc,
		settings: settings,
	}
}

func (h *repoIDPatchHandler) Factory() gimlet.RouteHandler {
	return &repoIDPatchHandler{
		sc:       h.sc,
		settings: h.settings,
	}
}

func (h *repoIDPatchHandler) Parse(ctx context.Context, r *http.Request) error {
	h.repoName = gimlet.GetVars(r)["repo_id"]
	h.user = MustHaveUser(ctx)
	body := util.NewRequestReader(r)
	defer body.Close()
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Argument read error")
	}

	// get the old repo
	h.originalRepo, err = dbModel.FindOneRepoRef(h.repoName)
	if err != nil {
		return err
	}
	if h.originalRepo == nil {
		return errors.Errorf("repo '%s' doesn't exist", h.repoName)
	}

	h.apiNewRepoRef = &model.APIProjectRef{}
	if err = h.apiNewRepoRef.BuildFromService(h.originalRepo.ProjectRef); err != nil {
		return errors.Wrap(err, "API error converting from model.ProjectRef to model.APIProjectRef")
	}

	// erase contents so apiNewRepoRef will only be populated with new elements for these fields
	h.apiNewRepoRef.Admins = nil
	h.apiNewRepoRef.GitTagAuthorizedUsers = nil
	h.apiNewRepoRef.GitTagAuthorizedTeams = nil
	if err = json.Unmarshal(b, h.apiNewRepoRef); err != nil {
		return errors.Wrap(err, "API error while unmarshalling JSON")
	}

	// read the new changes onto it
	i, err := h.apiNewRepoRef.ToService()
	if err != nil {
		return errors.Wrap(err, "API error converting from model.APIProjectRef to model.ProjectRef")
	}
	pRef, ok := i.(*dbModel.ProjectRef)
	if !ok {
		return errors.Errorf("unexpected type %T for model.ProjectRef", i)
	}
	h.newRepoRef = &dbModel.RepoRef{ProjectRef: *pRef}
	return nil
}

func (h *repoIDPatchHandler) Run(ctx context.Context) gimlet.Responder {

	before, err := h.sc.GetProjectSettings(&h.newRepoRef.ProjectRef)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error getting ProjectSettings before update for repo '%s'", h.repoName))
	}

	catcher := grip.NewSimpleCatcher()
	catcher.Add(h.newRepoRef.ValidateOwnerAndRepo(h.settings.GithubOrgs))

	// validate triggers before updating project
	for i, trigger := range h.newRepoRef.Triggers {
		catcher.Add(trigger.Validate(h.newRepoRef.Id))
		if trigger.DefinitionID == "" {
			h.newRepoRef.Triggers[i].DefinitionID = utility.RandomString()
		}
	}
	for i := range h.newRepoRef.PatchTriggerAliases {
		h.newRepoRef.PatchTriggerAliases[i], err = dbModel.ValidateTriggerDefinition(h.newRepoRef.PatchTriggerAliases[i], h.newRepoRef.Id)
		catcher.Add(err)
	}
	for i, buildDef := range h.newRepoRef.PeriodicBuilds {
		catcher.Wrapf(buildDef.Validate(), "invalid periodic build definition on line %d", i+1)
	}
	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(catcher.Resolve())
	}

	// update admins
	adminsToDelete := utility.FromStringPtrSlice(h.apiNewRepoRef.DeleteAdmins)
	adminsToAdd := h.newRepoRef.Admins
	allAdmins := utility.UniqueStrings(append(h.originalRepo.Admins, adminsToAdd...))          // get original and new admin
	h.newRepoRef.Admins, _ = utility.StringSliceSymmetricDifference(allAdmins, adminsToDelete) // add users that are in allAdmins and not in adminsToDelete

	usersToDelete := utility.FromStringPtrSlice(h.apiNewRepoRef.DeleteGitTagAuthorizedUsers)
	allAuthorizedUsers := utility.UniqueStrings(append(h.originalRepo.GitTagAuthorizedUsers, h.newRepoRef.GitTagAuthorizedUsers...))
	h.newRepoRef.GitTagAuthorizedUsers, _ = utility.StringSliceSymmetricDifference(allAuthorizedUsers, usersToDelete)

	teamsToDelete := utility.FromStringPtrSlice(h.apiNewRepoRef.DeleteGitTagAuthorizedTeams)
	allAuthorizedTeams := utility.UniqueStrings(append(h.originalRepo.GitTagAuthorizedTeams, h.newRepoRef.GitTagAuthorizedTeams...))
	h.newRepoRef.GitTagAuthorizedTeams, _ = utility.StringSliceSymmetricDifference(allAuthorizedTeams, teamsToDelete)

	repoAliases, err := h.sc.FindProjectAliases("", h.newRepoRef.Id, h.apiNewRepoRef.Aliases)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	// get every project that uses the repo, and merge them
	branchProjects, err := dbModel.FindMergedProjectRefsForRepo(h.newRepoRef)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "error finding branch projects for repo"))
	}
	if err = h.validateBranchesForRepo(ctx, h.newRepoRef, branchProjects, repoAliases); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	if h.originalRepo.Restricted != h.newRepoRef.Restricted {
		if h.newRepoRef.IsRestricted() {
			err = h.newRepoRef.MakeRestricted(branchProjects)
		} else {
			err = h.newRepoRef.MakeUnrestricted(branchProjects)
		}
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	// complete all updates
	if err = h.sc.UpdateRepo(h.newRepoRef); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for update() for '%s'", h.repoName))
	}
	if err = h.sc.UpdateProjectVars(h.newRepoRef.Id, &h.apiNewRepoRef.Variables, false); err != nil { // destructively modifies h.apiNewRepoRef.Variables
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error updating variables for project '%s'", h.repoName))
	}
	if err = h.sc.UpdateProjectAliases(h.newRepoRef.Id, h.apiNewRepoRef.Aliases); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error updating aliases for project '%s'", h.repoName))
	}

	if err = h.newRepoRef.UpdateAdminRoles(adminsToAdd, adminsToDelete); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Database error updating admins for project '%s'", h.repoName))
	}

	if err = h.sc.SaveSubscriptions(h.newRepoRef.Id, h.apiNewRepoRef.Subscriptions, true); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error saving subscriptions for project '%s'", h.repoName))
	}

	toDelete := []string{}
	for _, deleteSub := range h.apiNewRepoRef.DeleteSubscriptions {
		toDelete = append(toDelete, utility.FromStringPtr(deleteSub))
	}
	if err = h.sc.DeleteSubscriptions(h.newRepoRef.Id, toDelete); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error deleting subscriptions for project '%s'", h.repoName))
	}

	after, err := h.sc.GetProjectSettings(&h.newRepoRef.ProjectRef)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error getting ProjectSettings after update for project '%s'", h.repoName))
	}
	if err = dbModel.LogProjectModified(h.newRepoRef.Id, h.user.Username(), before, after); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error logging project modification for project '%s'", h.repoName))
	}

	if h.newRepoRef.Owner != h.originalRepo.Owner || h.newRepoRef.Repo != h.originalRepo.Repo {
		if err = dbModel.UpdateOwnerAndRepoForBranchProjects(h.newRepoRef.Id, h.newRepoRef.Owner, h.newRepoRef.Repo); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error updating owner repo for branch projects"))
		}

	}
	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusOK); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusOK))
	}
	return responder
}

func (h repoIDPatchHandler) validateBranchesForRepo(ctx context.Context, newRepoRef *dbModel.RepoRef, mergedRepos []dbModel.ProjectRef, aliases []model.APIProjectAlias) error {
	hasHook, err := h.sc.EnableWebhooks(ctx, &newRepoRef.ProjectRef)
	if err != nil {
		return errors.Wrapf(err, "error enabling webhooks for repo '%s'", h.repoName)
	}

	catcher := grip.NewBasicCatcher()

	// If we're enabling commit queue testing PR testing, verify that only one enabled project ref per branch has true or nil set.
	// If anything that uses webhooks is enabled, ensure that webhooks are configured.
	branchInfo := map[string]struct {
		commitQueueIds []string
		prTestingIds   []string
		gitTagIds      []string
		githubCheckIds []string
	}{}
	for _, p := range mergedRepos {
		if !p.IsEnabled() {
			continue
		}
		counts := branchInfo[p.Branch]
		if p.CommitQueue.IsEnabled() {
			counts.commitQueueIds = append(counts.commitQueueIds, p.Id)
		}
		if p.IsPRTestingEnabled() {
			counts.prTestingIds = append(counts.prTestingIds, p.Id)
		}
		if p.IsGitTagVersionsEnabled() {
			counts.gitTagIds = append(counts.gitTagIds, p.Id)
		}
		if p.IsGithubChecksEnabled() {
			counts.githubCheckIds = append(counts.githubCheckIds, p.Id)
		}
		branchInfo[p.Branch] = counts
	}

	for branch, info := range branchInfo {
		if len(info.commitQueueIds) > 0 {
			catcher.ErrorfWhen(len(info.commitQueueIds) > 1, "commit queue is enabled in multiple projects for branch '%s': %v", branch, info.commitQueueIds)
			catcher.ErrorfWhen(!hasHook, "cannot enable commit queue for repo, must enable Github webhooks first")
		}
		if len(info.prTestingIds) > 0 {
			catcher.ErrorfWhen(len(info.prTestingIds) > 1, "PR testing is enabled in multiple projects for branch '%s': %v", branch, info.prTestingIds)
			catcher.ErrorfWhen(!hasHook, "cannot enable PR testing for repo, must enable Github webhooks first")
		}
		if len(info.githubCheckIds) > 0 {
			catcher.ErrorfWhen(!hasHook, "cannot enable github checks for repo, must enable Github webhooks first")
		}
		if len(info.gitTagIds) > 0 {
			catcher.ErrorfWhen(!hasHook, "cannot enable git tag versions for repo, must enable Github webhooks first")
		}
	}

	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	for _, info := range branchInfo {
		if newRepoRef.IsPRTestingEnabled() || len(info.prTestingIds) > 0 {
			if !hasAliasDefined(aliases, evergreen.GithubPRAlias) {
				if newRepoRef.IsPRTestingEnabled() {
					catcher.Errorf("if repo PR testing enabled, must have aliases")
				} else if len(info.prTestingIds) > 0 {
					// verify that the project with PR testing enabled has aliases defined
					branchAliases, err := h.sc.FindProjectAliases(info.prTestingIds[0], "", nil)
					if err != nil {
						return errors.Wrapf(err, "error getting branch '%s' aliases", info.prTestingIds[0])
					}
					catcher.ErrorfWhen(!hasAliasDefined(branchAliases, evergreen.GithubPRAlias),
						"branch '%s' has PR testing enabled but has no aliases defined", info.prTestingIds[0])
				}
			}
		}

		if newRepoRef.CommitQueue.IsEnabled() || len(info.commitQueueIds) > 0 {
			if !hasAliasDefined(aliases, evergreen.CommitQueueAlias) {
				if newRepoRef.CommitQueue.IsEnabled() {
					catcher.Errorf("if repo commit queue enabled, must have aliases")
				} else if len(info.commitQueueIds) > 0 {
					// verify that the branch with the commit queue enabled has aliases defined in the branch
					branchAliases, err := h.sc.FindProjectAliases(info.commitQueueIds[0], "", nil)
					if err != nil {
						return errors.Wrapf(err, "error getting branch '%s' aliases", info.commitQueueIds[0])
					}
					catcher.ErrorfWhen(!hasAliasDefined(branchAliases, evergreen.CommitQueueAlias),
						"branch '%s' has the commit queue enabled but has no aliases defined", info.commitQueueIds[0])
				}
			}
		}
		if newRepoRef.IsGitTagVersionsEnabled() || len(info.gitTagIds) > 0 {
			if !hasAliasDefined(aliases, evergreen.GitTagAlias) {
				if newRepoRef.IsGitTagVersionsEnabled() {
					catcher.Errorf("if repo git tags enabled, must have aliases")
				} else if len(info.gitTagIds) > 0 {
					for _, branchId := range info.gitTagIds {
						// verify that the branch with git tag versions enabled has aliases defined in the branch
						branchAliases, err := h.sc.FindProjectAliases(branchId, "", nil)
						if err != nil {
							return errors.Wrapf(err, "error getting branch '%s' aliases", branchId)
						}
						catcher.ErrorfWhen(!hasAliasDefined(branchAliases, evergreen.GitTagAlias), "branch '%s' has git tag versions enabled but has no aliases defined", branchId)
					}
				}
			}
		}
		if newRepoRef.IsGithubChecksEnabled() || len(info.githubCheckIds) > 0 {
			if !hasAliasDefined(aliases, evergreen.GithubChecksAlias) {
				if newRepoRef.IsGithubChecksEnabled() {
					catcher.Errorf("if repo github checks enabled, must have aliases")
				} else if len(info.githubCheckIds) > 0 {
					for _, branchId := range info.githubCheckIds {
						// verify that the branch with github checks versions enabled has aliases defined in the branch
						branchAliases, err := h.sc.FindProjectAliases(branchId, "", nil)
						if err != nil {
							return errors.Wrapf(err, "error getting branch '%s' aliases", branchId)
						}
						catcher.ErrorfWhen(!hasAliasDefined(branchAliases, evergreen.GithubChecksAlias), "branch '%s' has github checks enabled but has no aliases defined", branchId)
					}
				}
			}
		}
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	// if we've turned on the commit queue for some branch, we need to verify that it has the commit queue enabled
	for _, info := range branchInfo {
		if len(info.commitQueueIds) > 0 {
			catcher.Add(commitqueue.EnsureCommitQueueExistsForProject(info.commitQueueIds[0]))
		}
	}
	return catcher.Resolve()
}
