package data

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// DBProjectConnector is a struct that implements the Project related methods
// from the Connector through interactions with the backing database.
type DBProjectConnector struct{}

// FindProjectById queries the database for the project matching the projectRef.Identifier.
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
func (pc *DBProjectConnector) CreateProject(projectRef *model.ProjectRef) error {
	err := projectRef.Insert()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("project with id '%s' was not inserted", projectRef.Identifier),
		}
	}
	return nil
}

// UpdateProject updates the given model.ProjectRef.Identifier.
func (pc *DBProjectConnector) UpdateProject(projectRef *model.ProjectRef) error {
	err := projectRef.Update()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("project with id '%s' was not updated", projectRef.Identifier),
		}
	}
	return nil
}

// FindProjects queries the backing database for the specified projects
func (pc *DBProjectConnector) FindProjects(key string, limit int, sortDir int, isAuthenticated bool) ([]model.ProjectRef, error) {
	projects, err := model.FindProjectRefs(key, limit, sortDir, isAuthenticated)
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching projects starting at project '%s'", key)
	}

	return projects, nil
}

// FindProjectVarsById returns the variables associated with the given project.
func (pc *DBProjectConnector) FindProjectVarsById(id string) (*restModel.APIProjectVars, error) {
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
	vars.RedactPrivateVars()
	varsModel := restModel.DbProjectVarsToRestModel(*vars)
	return &varsModel, nil
}

// UpdateProjectVars adds new variables, overwrites variables, and deletes variables for the given project.
func (pc *DBProjectConnector) UpdateProjectVars(vars *model.ProjectVars, varsToDelete []string) error {
	_, err := vars.FindAndModify(varsToDelete)
	if err != nil {
		return errors.Wrapf(err, "problem updating variables for project '%s'", vars.Id)
	}

	return nil
}

func (ac *DBProjectConnector) GetProjectEventLog(id string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
	events, err := model.ProjectEventsBefore(id, before, n)
	if err != nil {
		return nil, err
	}

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

// MockPatchConnector is a struct that implements the Patch related methods
// from the Connector through interactions with he backing database.
type MockProjectConnector struct {
	CachedProjects []model.ProjectRef
	CachedVars     []*model.ProjectVars
	CachedEvents   []restModel.APIProjectEvent
}

// FindProjects queries the cached projects slice for the matching projects.
// Assumes CachedProjects is sorted in alphabetical order of project identifier.
func (pc *MockProjectConnector) FindProjects(key string, limit int, sortDir int, isAuthenticated bool) ([]model.ProjectRef, error) {
	projects := []model.ProjectRef{}
	if sortDir > 0 {
		for i := 0; i < len(pc.CachedProjects); i++ {
			p := pc.CachedProjects[i]
			visible := isAuthenticated || (!isAuthenticated && !p.Private)
			if p.Identifier >= key && visible {
				projects = append(projects, p)
				if len(projects) == limit {
					break
				}
			}
		}
	} else {
		for i := len(pc.CachedProjects) - 1; i >= 0; i-- {
			p := pc.CachedProjects[i]
			visible := isAuthenticated || (!isAuthenticated && !p.Private)
			if p.Identifier < key && visible {
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
		if p.Identifier == projectId {
			return &p, nil
		}
	}
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("project with id '%s' not found", projectId),
	}
}

func (pc *MockProjectConnector) CreateProject(projectRef *model.ProjectRef) error {
	for _, p := range pc.CachedProjects {
		if p.Identifier == projectRef.Identifier {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("project with id '%s' was not inserted", projectRef.Identifier),
			}
		}
	}
	return nil
}

func (pc *MockProjectConnector) UpdateProject(projectRef *model.ProjectRef) error {
	for _, p := range pc.CachedProjects {
		if p.Identifier == projectRef.Identifier {
			return nil
		}
	}
	return gimlet.ErrorResponse{
		StatusCode: http.StatusInternalServerError,
		Message:    fmt.Sprintf("project with id '%s' was not updated", projectRef.Identifier),
	}
}

func (pc *MockProjectConnector) FindProjectVarsById(id string) (*restModel.APIProjectVars, error) {
	for _, v := range pc.CachedVars {
		if v.Id == id {
			res := restModel.DbProjectVarsToRestModel(*v)
			return &res, nil
		}
	}
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("variables for project '%s' not found", id),
	}
}

func (pc *MockProjectConnector) UpdateProjectVars(vars *model.ProjectVars, varsToDelete []string) error {
	for _, cachedVars := range pc.CachedVars {
		if cachedVars.Id == vars.Id {
			// update cached variables by adding new variables and deleting variables
			for key, val := range vars.Vars {
				cachedVars.Vars[key] = val
			}
			for key, val := range vars.PrivateVars {
				cachedVars.PrivateVars[key] = val
			}
			for _, varToDelete := range varsToDelete {
				delete(cachedVars.Vars, varToDelete)
				delete(cachedVars.PrivateVars, varToDelete)
			}

			// return modified variables
			vars.Vars = cachedVars.Vars
			vars.PrivateVars = cachedVars.PrivateVars
			return nil
		}
	}
	return gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("variables for project '%s' not found", vars.Id),
	}
}

func (pc *MockProjectConnector) GetProjectEventLog(id string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
	return pc.CachedEvents, nil
}

func (pc *MockProjectConnector) GetProjectWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch string) (*model.ProjectRef, error) {
	for _, p := range pc.CachedProjects {
		if p.Owner == owner && p.Repo == repo && p.Branch == branch {
			return &p, nil
		}
	}

	return nil, errors.Errorf("can't query for projectRef %s/%s tracking %s", owner, repo, branch)
}
