package data

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// DBProjectConnector is a struct that implements the Project related methods
// from the Connector through interactions with the backing database.
type DBProjectConnector struct{}

// FindProjectById queries the database for the project matching the projectRef.Id.
func (pc *DBProjectConnector) FindProjectById(id string) (*model.ProjectRef, error) {
	p, err := model.FindOneProjectRef(id)
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
			Message:    fmt.Sprintf("project with id '%s' was not inserted", projectRef.Id),
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
		projectRef.TracksPushEvents = true
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
			"source":  "patch project",
			"message": "can't setup webhook",
			"project": projectRef.Id,
			"owner":   projectRef.Owner,
			"repo":    projectRef.Repo,
		}))
		projectRef.TracksPushEvents = false
		return false, nil
	}

	if err = hook.Insert(); err != nil {
		return false, errors.Wrapf(err, "error inserting new webhook for project '%s'", projectRef.Id)
	}
	projectRef.TracksPushEvents = true
	return true, nil
}

func (pc *DBProjectConnector) EnablePRTesting(projectRef *model.ProjectRef) error {
	conflictingRefs, err := model.FindProjectRefsByRepoAndBranch(projectRef.Owner, projectRef.Repo, projectRef.Branch)
	if err != nil {
		return errors.Wrap(err, "error finding project refs")
	}
	for _, ref := range conflictingRefs {
		if ref.PRTestingEnabled && ref.Id != projectRef.Id {
			return errors.Errorf("Cannot enable PR Testing in this repo, must disable in other projects first")
		}
	}
	return nil
}

func (pc *DBProjectConnector) EnableCommitQueue(projectRef *model.ProjectRef, commitQueueParams model.CommitQueueParams) error {
	if ok, err := projectRef.CanEnableCommitQueue(); err != nil {
		return errors.Wrap(err, "error enabling commit queue")
	} else if !ok {
		return errors.Errorf("Cannot enable commit queue in this repo, must disable in other projects first")
	}

	cq, err := commitqueue.FindOneId(projectRef.Id)
	if err != nil {
		return errors.Wrapf(err, "database error finding commit queue")
	}
	if cq == nil {
		cq = &commitqueue.CommitQueue{ProjectID: projectRef.Id}
		if err = commitqueue.InsertQueue(cq); err != nil {
			return errors.Wrapf(err, "problem inserting new commit queue")
		}
	}
	return nil
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

// FindProjectVarsById returns the variables associated with the given project.
func (pc *DBProjectConnector) FindProjectVarsById(id string, redact bool) (*restModel.APIProjectVars, error) {
	vars, err := model.FindOneProjectVars(id)
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching variables for project '%s'", id)
	}
	if vars == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("variables for project '%s' not found", id),
		}
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

func (pc *DBProjectConnector) CopyProjectVars(oldProjectId, newProjectId string) error {
	vars, err := model.FindOneProjectVars(oldProjectId)
	if err != nil {
		return errors.Wrapf(err, "error finding variables for project '%s'", oldProjectId)
	}
	vars.Id = newProjectId
	return errors.Wrapf(vars.Insert(), "error inserting variables for project '%s", newProjectId)
}

func (ac *DBProjectConnector) GetProjectEventLog(id string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
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
	return model.FindEnabledProjectRefsByOwnerAndRepo(owner, repo)
}

func (ac *DBProjectConnector) GetVersionsInProject(identifier, requester string, limit, startOrder int) ([]restModel.APIVersion, error) {
	projectId, err := model.FindIdForProject(identifier)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding project '%s'", identifier)
	}
	versions, err := model.VersionFind(model.VersionsByRequesterOrdered(projectId, requester, limit, startOrder))
	if err != nil {
		return nil, errors.Wrap(err, "error finding versions")
	}
	catcher := grip.NewBasicCatcher()
	out := []restModel.APIVersion{}
	for _, dbVersion := range versions {
		restVersion := restModel.APIVersion{}
		catcher.Add(restVersion.BuildFromService(&dbVersion))
		out = append(out, restVersion)
	}

	return out, catcher.Resolve()
}

func (pc *DBProjectConnector) GetProjectSettingsEvent(p *model.ProjectRef) (*model.ProjectSettingsEvent, error) {
	hook, err := model.FindGithubHook(p.Owner, p.Repo)
	if err != nil {
		return nil, errors.Wrapf(err, "Database error finding github hook for project '%s'", p.Id)
	}
	projectVars, err := model.FindOneProjectVars(p.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding variables for project '%s'", p.Id)
	}
	if projectVars == nil {
		projectVars = &model.ProjectVars{}
	}
	projectAliases, err := model.FindAliasesForProject(p.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding aliases for project '%s'", p.Id)
	}
	subscriptions, err := event.FindSubscriptionsByOwner(p.Id, event.OwnerTypeProject)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding subscription for project '%s'", p.Id)
	}
	projectSettingsEvent := model.ProjectSettingsEvent{
		ProjectRef:         *p,
		GitHubHooksEnabled: hook != nil,
		Vars:               *projectVars,
		Aliases:            projectAliases,
		Subscriptions:      subscriptions,
	}
	return &projectSettingsEvent, nil
}

func (pc *DBProjectConnector) GetProjectAliasResults(p *model.Project, alias string, includeDeps bool) ([]restModel.APIVariantTasks, error) {
	projectAliases, err := model.FindAliasInProject(p.Identifier, alias)
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
	if alias == evergreen.GithubAlias {
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

func (pc *MockProjectConnector) FindProjectById(projectId string) (*model.ProjectRef, error) {
	for _, p := range pc.CachedProjects {
		if p.Id == projectId {
			return &p, nil
		}
	}
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("project with id '%s' not found", projectId),
	}
}

func (pc *MockProjectConnector) CreateProject(projectRef *model.ProjectRef, u *user.DBUser) error {
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

func (pc *MockProjectConnector) UpdateProject(projectRef *model.ProjectRef) error {
	for _, p := range pc.CachedProjects {
		if p.Id == projectRef.Id {
			return nil
		}
	}
	return gimlet.ErrorResponse{
		StatusCode: http.StatusInternalServerError,
		Message:    fmt.Sprintf("project with id '%s' was not updated", projectRef.Id),
	}
}

func (pc *MockProjectConnector) UpdateAdminRoles(project *model.ProjectRef, toAdd, toDelete []string) error {
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
	pp, err := model.LoadProjectInto([]byte(config), pRef.Id, p)
	return p, pp, err
}

func (pc *MockProjectConnector) FindProjectVarsById(id string, redact bool) (*restModel.APIProjectVars, error) {
	varsModel := &restModel.APIProjectVars{}
	res := &model.ProjectVars{
		Id:   id,
		Vars: map[string]string{},
	}
	for _, v := range pc.CachedVars {
		if v.Id == id {
			for key, val := range v.Vars {
				res.Vars[key] = val
			}
			res.PrivateVars = v.PrivateVars
			if redact {
				res = res.RedactPrivateVars()
			}

			if err := varsModel.BuildFromService(res); err != nil {
				return nil, errors.Wrapf(err, "error building project variables from service")
			}
			return varsModel, nil
		}
	}
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("variables for project '%s' not found", id),
	}
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
		if p.Owner == owner && p.Repo == repo && p.Branch == branch && p.CommitQueue.Enabled == true {
			return &p, nil
		}
	}
	return nil, nil
}

func (pc *MockProjectConnector) FindEnabledProjectRefsByOwnerAndRepo(owner, repo string) ([]model.ProjectRef, error) {
	refs := []model.ProjectRef{}
	for _, p := range pc.CachedProjects {
		if p.Owner == owner && p.Repo == repo && p.Enabled == true {
			refs = append(refs, p)
		}
	}
	return refs, nil
}

func (pc *MockProjectConnector) EnableWebhooks(ctx context.Context, projectRef *model.ProjectRef) (bool, error) {
	return true, nil
}

func (pc *MockProjectConnector) EnableCommitQueue(projectRef *model.ProjectRef, commitQueueParams model.CommitQueueParams) error {
	return nil
}

func (pc *MockProjectConnector) EnablePRTesting(projectRef *model.ProjectRef) error {
	return nil
}

func (pc *MockProjectConnector) UpdateProjectRevision(projectID, revision string) error {
	return nil
}

func (ac *MockProjectConnector) GetVersionsInProject(project, requester string, limit, startOrder int) ([]restModel.APIVersion, error) {
	return nil, nil
}

func (pc *MockProjectConnector) GetProjectSettingsEvent(p *model.ProjectRef) (*model.ProjectSettingsEvent, error) {
	if len(p.Owner) == 0 || len(p.Repo) == 0 {
		return nil, errors.New("Owner and repository must not be empty strings")
	}
	return &model.ProjectSettingsEvent{}, nil
}

func (pc *MockProjectConnector) GetProjectAliasResults(*model.Project, string, bool) ([]restModel.APIVariantTasks, error) {
	return nil, nil
}
