package data

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

// DBProjectConnector is a struct that implements the Project related methods
// from the Connector through interactions with the backing database.
type DBProjectConnector struct{}

// FindProjectById queries the database for the project matching the projectRef.Id.
func (pc *DBProjectConnector) FindProjectById(id string, includeRepo bool) (*model.ProjectRef, error) {
	var p *model.ProjectRef
	var err error
	if includeRepo {
		p, err = model.FindMergedProjectRef(id)
	} else {
		p, err = model.FindBranchProjectRef(id)
	}
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project with id '%s' not found", id),
		}
	}
	return p, nil
}

// CreateProject inserts the given model.ProjectRef.
func (pc *DBProjectConnector) CreateProject(projectRef *model.ProjectRef, u *user.DBUser) error {
	err := projectRef.Add(u)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "project with id '%s' was not inserted", projectRef.Id).Error(),
		}
	}
	return nil
}

// UpdateProject updates the given model.ProjectRef.Id.
func (pc *DBProjectConnector) UpdateProject(projectRef *model.ProjectRef) error {
	err := projectRef.Update()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("project with id '%s' was not updated", projectRef.Id),
		}
	}
	return nil
}

// UpdateRepo updates the given model.RepoRef.Id. We use EnsureCommitQueueExistsForProject to ensure that only values relevant to repos get updated.
func (pc *DBProjectConnector) UpdateRepo(repoRef *model.RepoRef) error {
	err := repoRef.Upsert()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("repo with id '%s' was not updated", repoRef.Id),
		}
	}
	return nil
}

func (pc *DBProjectConnector) GetProjectFromFile(ctx context.Context, pRef model.ProjectRef, file string, token string) (*model.Project, *model.ParserProject, error) {
	opts := model.GetProjectOpts{
		Ref:        &pRef,
		Revision:   pRef.Branch,
		RemotePath: file,
		Token:      token,
	}
	return model.GetProjectFromFile(ctx, opts)
}

// EnableWebhooks returns true if a hook for the given owner/repo exists or was inserted.
func (pc *DBProjectConnector) EnableWebhooks(ctx context.Context, projectRef *model.ProjectRef) (bool, error) {
	hook, err := model.FindGithubHook(projectRef.Owner, projectRef.Repo)
	if err != nil {
		return false, errors.Wrapf(err, "Database error finding github hook for project '%s'", projectRef.Id)
	}
	if hook != nil {
		projectRef.TracksPushEvents = utility.TruePtr()
		return true, nil
	}

	settings, err := evergreen.GetConfig()
	if err != nil {
		return false, errors.Wrap(err, "error finding evergreen settings")
	}

	hook, err = model.SetupNewGithubHook(ctx, *settings, projectRef.Owner, projectRef.Repo)
	if err != nil {
		// don't return error:
		// sometimes people change a project to track a personal
		// branch we don't have access to
		grip.Error(message.WrapError(err, message.Fields{
			"source":             "patch project",
			"message":            "can't setup webhook",
			"project":            projectRef.Id,
			"project_identifier": projectRef.Identifier,
			"owner":              projectRef.Owner,
			"repo":               projectRef.Repo,
		}))
		projectRef.TracksPushEvents = utility.FalsePtr()
		return false, nil
	}

	if err = hook.Insert(); err != nil {
		return false, errors.Wrapf(err, "error inserting new webhook for project '%s'", projectRef.Id)
	}
	projectRef.TracksPushEvents = utility.TruePtr()
	return true, nil
}

func (pc *DBProjectConnector) EnablePRTesting(projectRef *model.ProjectRef) error {
	conflictingRefs, err := model.FindMergedEnabledProjectRefsByRepoAndBranch(projectRef.Owner, projectRef.Repo, projectRef.Branch)
	if err != nil {
		return errors.Wrap(err, "error finding project refs")
	}
	for _, ref := range conflictingRefs {
		if ref.IsPRTestingEnabled() && ref.Id != projectRef.Id {
			return errors.Errorf("Cannot enable PR Testing in this repo, must disable in other projects first")
		}
	}
	return nil
}

func (pc *DBProjectConnector) EnableCommitQueue(projectRef *model.ProjectRef) error {
	if ok, err := projectRef.CanEnableCommitQueue(); err != nil {
		return errors.Wrap(err, "error enabling commit queue")
	} else if !ok {
		return errors.Errorf("Cannot enable commit queue in this repo, must disable in other projects first")
	}

	return commitqueue.EnsureCommitQueueExistsForProject(projectRef.Id)
}

func (pc *DBProjectConnector) UpdateProjectRevision(projectID, revision string) error {
	if err := model.UpdateLastRevision(projectID, revision); err != nil {
		return errors.Wrapf(err, "error updating revision for project '%s'", projectID)
	}

	return nil
}

// FindProjects queries the backing database for the specified projects
func (pc *DBProjectConnector) FindProjects(key string, limit int, sortDir int) ([]model.ProjectRef, error) {
	projects, err := model.FindProjectRefs(key, limit, sortDir)
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching projects starting at project '%s'", key)
	}

	return projects, nil
}

// FindProjectVarsById returns the variables associated with the project and repo (if given).
func (pc *DBProjectConnector) FindProjectVarsById(id string, repoId string, redact bool) (*restModel.APIProjectVars, error) {
	var repoVars *model.ProjectVars
	var err error
	if repoId != "" {
		repoVars, err = model.FindOneProjectVars(repoId)
		if err != nil {
			return nil, errors.Wrapf(err, "problem fetching variables for repo '%s'", repoId)
		}
		if repoVars == nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("variables for repo '%s' not found", repoId),
			}
		}
	}
	var vars *model.ProjectVars
	if id != "" {
		vars, err = model.FindOneProjectVars(id)
		if err != nil {
			return nil, errors.Wrapf(err, "problem fetching variables for project '%s'", id)
		}
		if vars == nil {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("variables for project '%s' not found", id),
			}
		}
		vars.MergeWithRepoVars(repoVars)
	} else {
		vars = repoVars
	}

	if redact {
		vars = vars.RedactPrivateVars()
	}

	varsModel := restModel.APIProjectVars{}
	if err := varsModel.BuildFromService(vars); err != nil {
		return nil, errors.Wrap(err, "error building project variables from service")
	}
	return &varsModel, nil
}

func (pc *DBProjectConnector) UpdateAdminRoles(project *model.ProjectRef, toAdd, toDelete []string) error {
	if project == nil {
		return errors.New("no project found")
	}
	return project.UpdateAdminRoles(toAdd, toDelete)
}

// RemoveAdminFromProjects removes a user from all Admins slices of every project
func (pc *DBProjectConnector) RemoveAdminFromProjects(toDelete string) error {
	return model.RemoveAdminFromProjects(toDelete)
}

// UpdateProjectVars adds new variables, overwrites variables, and deletes variables for the given project.
func (pc *DBProjectConnector) UpdateProjectVars(projectId string, varsModel *restModel.APIProjectVars, overwrite bool) error {
	if varsModel == nil {
		return nil
	}
	v, err := varsModel.ToService()
	if err != nil {
		return errors.Wrap(err, "problem converting to project variable model")
	}
	vars := v.(*model.ProjectVars)
	vars.Id = projectId

	if overwrite {
		if _, err = vars.Upsert(); err != nil {
			return errors.Wrapf(err, "problem overwriting variables for project '%s'", vars.Id)
		}
	} else {
		_, err = vars.FindAndModify(varsModel.VarsToDelete)
		if err != nil {
			return errors.Wrapf(err, "problem updating variables for project '%s'", vars.Id)
		}
	}

	vars = vars.RedactPrivateVars()
	varsModel.Vars = vars.Vars
	varsModel.PrivateVars = vars.PrivateVars
	varsModel.RestrictedVars = vars.RestrictedVars
	varsModel.VarsToDelete = []string{}
	return nil
}

func (pc *DBProjectConnector) UpdateProjectVarsByValue(toReplace, replacement, username string, dryRun bool) (map[string]string, error) {
	catcher := grip.NewBasicCatcher()
	matchingProjects, err := model.GetVarsByValue(toReplace)
	if err != nil {
		catcher.Wrap(err, "failed to fetch projects with matching value")
	}
	if matchingProjects == nil {
		catcher.New("no projects with matching value found")
	}
	changes := map[string]string{}
	for _, project := range matchingProjects {
		for key, val := range project.Vars {
			if val == toReplace {
				if !dryRun {
					originalVars := make(map[string]string)
					for k, v := range project.Vars {
						originalVars[k] = v
					}
					before := model.ProjectSettings{
						Vars: model.ProjectVars{
							Id:   project.Id,
							Vars: originalVars,
						},
					}

					project.Vars[key] = replacement
					_, err = project.Upsert()
					if err != nil {
						catcher.Wrapf(err, "problem overwriting variables for project '%s'", project.Id)
					}

					after := model.ProjectSettings{
						Vars: model.ProjectVars{
							Id:   project.Id,
							Vars: project.Vars,
						},
					}

					if err = model.LogProjectModified(project.Id, username, &before, &after); err != nil {
						catcher.Wrapf(err, "Error logging project modification for project '%s'", project.Id)
					}
				}
				changes[project.Id] = key
			}
		}
	}
	return changes, catcher.Resolve()
}

func (pc *DBProjectConnector) CopyProjectVars(oldProjectId, newProjectId string) error {
	vars, err := model.FindOneProjectVars(oldProjectId)
	if err != nil {
		return errors.Wrapf(err, "error finding variables for project '%s'", oldProjectId)
	}
	vars.Id = newProjectId
	return errors.Wrapf(vars.Insert(), "error inserting variables for project '%s", newProjectId)
}

func (ac *DBProjectConnector) GetProjectEventLog(project string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
	id, err := model.GetIdForProject(project)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"func":    "GetProjectEventLog",
			"message": "error getting id for project",
			"project": project,
		}))
		// don't return an error here to preserve existing behavior
		return nil, nil
	}
	events, err := model.ProjectEventsBefore(id, before, n)
	if err != nil {
		return nil, err
	}
	events.RedactPrivateVars()

	out := []restModel.APIProjectEvent{}
	catcher := grip.NewBasicCatcher()
	for _, evt := range events {
		apiEvent := restModel.APIProjectEvent{}
		err = apiEvent.BuildFromService(evt)
		if err != nil {
			catcher.Add(err)
			continue
		}
		out = append(out, apiEvent)
	}

	return out, catcher.Resolve()
}

func (ac *DBProjectConnector) GetProjectWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch string) (*model.ProjectRef, error) {
	proj, err := model.FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, errors.Wrapf(err, "can't query for projectRef %s/%s tracking %s", owner, repo, branch)
	}

	return proj, nil
}

func (ac *DBProjectConnector) FindEnabledProjectRefsByOwnerAndRepo(owner, repo string) ([]model.ProjectRef, error) {
	return model.FindMergedEnabledProjectRefsByOwnerAndRepo(owner, repo)
}

func (pc *DBProjectConnector) GetProjectSettings(p *model.ProjectRef) (*model.ProjectSettings, error) {
	return model.GetProjectSettings(p)
}

func (pc *DBProjectConnector) GetProjectAliasResults(p *model.Project, alias string, includeDeps bool) ([]restModel.APIVariantTasks, error) {
	projectAliases, err := model.FindAliasInProjectOrRepo(p.Identifier, alias)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no alias named '%s' for project '%s'", alias, p.Identifier),
		}
	}
	matches := []restModel.APIVariantTasks{}
	for _, projectAlias := range projectAliases {
		requester := getRequesterFromAlias(projectAlias.Alias)
		_, _, variantTasks := p.ResolvePatchVTs(nil, nil, requester, projectAlias.Alias, includeDeps)
		for _, variantTask := range variantTasks {
			matches = append(matches, restModel.APIVariantTasksBuildFromService(variantTask))
		}
	}

	return matches, nil
}

func getRequesterFromAlias(alias string) string {
	if alias == evergreen.GithubPRAlias {
		return evergreen.GithubPRRequester
	}
	if alias == evergreen.GitTagAlias {
		return evergreen.GitTagRequester
	}
	if alias == evergreen.CommitQueueAlias {
		return evergreen.MergeTestRequester
	}
	return evergreen.PatchVersionRequester
}

// MockPatchConnector is a struct that implements the Patch related methods
// from the Connector through interactions with he backing database.
type MockProjectConnector struct {
	CachedProjects []model.ProjectRef
	CachedVars     []*model.ProjectVars
	CachedEvents   []restModel.APIProjectEvent
}

func (pc *MockProjectConnector) FindProjectById(projectId string, includeRepo bool) (*model.ProjectRef, error) {
	for _, p := range pc.CachedProjects {
		if p.Id == projectId || p.Identifier == projectId {
			return &p, nil
		}
	}
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("project with id '%s' not found", projectId),
	}
}

func (pc *MockProjectConnector) CreateProject(projectRef *model.ProjectRef, u *user.DBUser) error {
	projectRef.Id = mgobson.NewObjectId().Hex()
	for _, p := range pc.CachedProjects {
		if p.Id == projectRef.Id {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("project with id '%s' was not inserted", projectRef.Id),
			}
		}
	}
	pc.CachedProjects = append(pc.CachedProjects, *projectRef)
	return nil
}

// FindProjects queries the cached projects slice for the matching projects.
// Assumes CachedProjects is sorted in alphabetical order of project identifier.
func (pc *MockProjectConnector) FindProjects(key string, limit int, sortDir int) ([]model.ProjectRef, error) {
	projects := []model.ProjectRef{}
	if sortDir > 0 {
		for i := 0; i < len(pc.CachedProjects); i++ {
			p := pc.CachedProjects[i]
			if p.Id >= key {
				projects = append(projects, p)
				if len(projects) == limit {
					break
				}
			}
		}
	} else {
		for i := len(pc.CachedProjects) - 1; i >= 0; i-- {
			p := pc.CachedProjects[i]
			if p.Id < key {
				projects = append(projects, p)
				if len(projects) == limit {
					break
				}
			}
		}
	}
	return projects, nil
}

func (pc *MockProjectConnector) UpdateProject(projectRef *model.ProjectRef) error {
	for i, p := range pc.CachedProjects {
		if p.Id == projectRef.Id {
			pc.CachedProjects[i] = *projectRef
			return nil
		}
	}
	return gimlet.ErrorResponse{
		StatusCode: http.StatusInternalServerError,
		Message:    fmt.Sprintf("project with id '%s' was not updated", projectRef.Id),
	}
}

func (pc *MockProjectConnector) UpdateRepo(repoRef *model.RepoRef) error {
	return nil
}

func (pc *MockProjectConnector) UpdateAdminRoles(project *model.ProjectRef, toAdd, toDelete []string) error {
	return nil
}

func (pc *MockProjectConnector) RemoveAdminFromProjects(toDelete string) error {
	return nil
}

func (pc *MockProjectConnector) GetProjectFromFile(ctx context.Context, pRef model.ProjectRef, file string, token string) (*model.Project, *model.ParserProject, error) {
	config := `
buildvariants:
- name: v1
  run_on: d
  tasks:
  - name: t1
tasks:
- name: t1
`
	p := &model.Project{}
	opts := &model.GetProjectOpts{
		Ref:          &pRef,
		RemotePath:   file,
		Token:        token,
		ReadFileFrom: model.ReadFromLocal,
	}
	pp, err := model.LoadProjectInto(ctx, []byte(config), opts, pRef.Id, p)
	return p, pp, err
}

func (pc *MockProjectConnector) FindProjectVarsById(id, repoId string, redact bool) (*restModel.APIProjectVars, error) {
	varsModel := &restModel.APIProjectVars{}
	repoVars := &model.ProjectVars{
		Id:   id,
		Vars: map[string]string{},
	}
	res := &model.ProjectVars{
		Id:   id,
		Vars: map[string]string{},
	}
	found := false
	for _, v := range pc.CachedVars {
		if v.Id == id {
			found = true
			for key, val := range v.Vars {
				res.Vars[key] = val
			}
			res.PrivateVars = v.PrivateVars
		}
		if v.Id == repoId {
			found = true
			for key, val := range v.Vars {
				repoVars.Vars[key] = val
			}
			repoVars.PrivateVars = v.PrivateVars
		}
	}
	if !found {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("variables for project '%s' not found", id),
		}
	}
	res.MergeWithRepoVars(repoVars)
	if redact {
		res = res.RedactPrivateVars()
	}
	if err := varsModel.BuildFromService(res); err != nil {
		return nil, errors.Wrapf(err, "error building project variables from service")
	}
	return varsModel, nil
}

func (pc *MockProjectConnector) UpdateProjectVars(projectId string, varsModel *restModel.APIProjectVars, overwrite bool) error {
	tempVars := &model.ProjectVars{
		Id:   projectId,
		Vars: map[string]string{},
	}
	for _, cachedVars := range pc.CachedVars {
		if cachedVars.Id == projectId {
			if overwrite {
				cachedVars.Vars = map[string]string{}
				cachedVars.PrivateVars = map[string]bool{}
			}
			// update cached variables by adding new variables and deleting variables
			for key, val := range varsModel.Vars {
				cachedVars.Vars[key] = val
			}
			for key, private := range varsModel.PrivateVars {
				if private {
					cachedVars.PrivateVars[key] = true // don't unredact existing variables
				}
			}
			for _, varToDelete := range varsModel.VarsToDelete {
				delete(cachedVars.Vars, varToDelete)
				delete(cachedVars.PrivateVars, varToDelete)
			}
			for k, v := range cachedVars.Vars {
				tempVars.Vars[k] = v
			}
			tempVars.PrivateVars = cachedVars.PrivateVars
			tempVars = tempVars.RedactPrivateVars()
			// return modified variables
			varsModel.Vars = tempVars.Vars
			varsModel.PrivateVars = tempVars.PrivateVars
			varsModel.VarsToDelete = []string{}
			return nil
		}
	}
	// handle new project
	tempVars.Vars = varsModel.Vars
	tempVars.PrivateVars = varsModel.PrivateVars
	tempVars.Id = projectId
	pc.CachedVars = append(pc.CachedVars, tempVars)
	// redact private variables
	tempVars = tempVars.RedactPrivateVars()
	varsModel.Vars = tempVars.Vars
	return nil
}

func (pc *MockProjectConnector) UpdateProjectVarsByValue(toReplace, replacement, username string, dryRun bool) (map[string]string, error) {
	changes := map[string]string{}
	for _, cachedVars := range pc.CachedVars {
		for key, val := range cachedVars.Vars {
			if toReplace == val {
				if !dryRun {
					cachedVars.Vars[key] = replacement
				}
				changes[cachedVars.Id] = key
			}
		}
	}
	return changes, nil
}

func (pc *MockProjectConnector) CopyProjectVars(oldProjectId, newProjectId string) error {
	newVars := model.ProjectVars{Id: newProjectId}
	for _, v := range pc.CachedVars {
		if v.Id == oldProjectId {
			newVars.Vars = v.Vars
			newVars.PrivateVars = v.PrivateVars
			newVars.RestrictedVars = v.RestrictedVars
			pc.CachedVars = append(pc.CachedVars, &newVars)
			return nil
		}
	}
	return errors.Errorf("error finding variables for project '%s'", oldProjectId)
}

func (pc *MockProjectConnector) GetProjectEventLog(id string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
	return pc.CachedEvents, nil
}

func (pc *MockProjectConnector) GetProjectWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch string) (*model.ProjectRef, error) {
	for _, p := range pc.CachedProjects {
		if p.Owner == owner && p.Repo == repo && p.Branch == branch && p.CommitQueue.IsEnabled() {
			return &p, nil
		}
	}
	return nil, nil
}

func (pc *MockProjectConnector) FindEnabledProjectRefsByOwnerAndRepo(owner, repo string) ([]model.ProjectRef, error) {
	refs := []model.ProjectRef{}
	for _, p := range pc.CachedProjects {
		if p.Owner == owner && p.Repo == repo && p.IsEnabled() {
			refs = append(refs, p)
		}
	}
	return refs, nil
}

func (pc *MockProjectConnector) EnableWebhooks(ctx context.Context, projectRef *model.ProjectRef) (bool, error) {
	return true, nil
}

func (pc *MockProjectConnector) EnableCommitQueue(projectRef *model.ProjectRef) error {
	return nil
}

func (pc *MockProjectConnector) EnablePRTesting(projectRef *model.ProjectRef) error {
	return nil
}

func (pc *MockProjectConnector) UpdateProjectRevision(projectID, revision string) error {
	return nil
}

func (pc *MockProjectConnector) GetProjectSettings(p *model.ProjectRef) (*model.ProjectSettings, error) {
	return &model.ProjectSettings{}, nil
}

func (pc *MockProjectConnector) GetProjectAliasResults(*model.Project, string, bool) ([]restModel.APIVariantTasks, error) {
	return nil, nil
}
