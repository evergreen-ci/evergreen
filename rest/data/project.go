package data

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const EventLogLimit = 10

// DBProjectConnector is a struct that implements the Project related methods
// from the Connector through interactions with the backing database.
type DBProjectConnector struct{}

// FindProjectById queries the database for the project matching the projectRef.Id. If the bool flag is set,
// the project config properties in the project YAML will be merged into the result if the properties are
// not set on the project page.
func FindProjectById(id string, includeRepo bool, includeProjectConfig bool) (*model.ProjectRef, error) {
	var p *model.ProjectRef
	var err error
	if includeRepo && includeProjectConfig {
		p, err = model.FindMergedProjectRef(id, "", true)
	} else if includeRepo {
		p, err = model.FindMergedProjectRef(id, "", false)
	} else {
		p, err = model.FindBranchProjectRef(id)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "finding project '%s'", id)
	}
	if p == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project '%s' not found", id),
		}
	}
	return p, nil
}

// CreateProject inserts the given model.ProjectRef.
func CreateProject(projectRef *model.ProjectRef, u *user.DBUser) error {
	if projectRef.Identifier != "" {
		if err := VerifyUniqueProject(projectRef.Identifier); err != nil {
			return err
		}
	}
	if projectRef.Id != "" {
		if err := VerifyUniqueProject(projectRef.Id); err != nil {
			return err
		}
	}
	err := projectRef.Add(u)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "inserting project '%s'", projectRef.Identifier).Error(),
		}
	}

	newProjectVars := model.ProjectVars{
		Id: projectRef.Id,
	}

	err = newProjectVars.Insert()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "initializing project variables for project '%s'", projectRef.Identifier).Error(),
		}
	}
	err = model.LogProjectAdded(projectRef.Id, u.DisplayName())
	grip.Error(message.WrapError(err, message.Fields{
		"message":            "problem logging project added",
		"project_id":         projectRef.Id,
		"project_identifier": projectRef.Identifier,
		"user":               u.DisplayName(),
	}))
	return nil
}

// VerifyUniqueProject returns a bad request error if the project ID / identifier is already in use.
func VerifyUniqueProject(name string) error {
	_, err := FindProjectById(name, false, false)
	if err == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot use existing project name '%s'", name),
		}
	}
	apiErr, ok := err.(gimlet.ErrorResponse)
	if !ok {
		return errors.Errorf("Type assertion failed: type %T does not hold an error", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		return errors.Wrapf(err, "Database error verifying project '%s' doesn't already exist", name)
	}
	return nil
}

// GetProjectTasksWithOptions finds the previous tasks that have run on a project that adhere to the passed in options.
func GetProjectTasksWithOptions(projectName string, taskName string, opts model.GetProjectTasksOpts) ([]restModel.APITask, error) {
	tasks, err := model.GetTasksWithOptions(projectName, taskName, opts)
	if err != nil {
		return nil, err
	}
	res := []restModel.APITask{}
	for _, t := range tasks {
		apiTask := restModel.APITask{}
		if err = apiTask.BuildFromArgs(&t, &restModel.APITaskArgs{
			IncludeProjectIdentifier: true,
			IncludeAMI:               true,
		}); err != nil {
			return nil, errors.Wrap(err, "error building API tasks")
		}
		res = append(res, apiTask)
	}
	return res, nil
}

// FindProjectVarsById returns the variables associated with the project and repo (if given).
func FindProjectVarsById(id string, repoId string, redact bool) (*restModel.APIProjectVars, error) {
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
		return nil, errors.Wrap(err, "converting project variables to API model")
	}
	return &varsModel, nil
}

// UpdateProjectVars adds new variables, overwrites variables, and deletes variables for the given project.
func UpdateProjectVars(projectId string, varsModel *restModel.APIProjectVars, overwrite bool) error {
	if varsModel == nil {
		return nil
	}
	v, err := varsModel.ToService()
	if err != nil {
		return errors.Wrap(err, "converting project variables to service model")
	}
	vars := v.(*model.ProjectVars)
	vars.Id = projectId

	if overwrite {
		if _, err = vars.Upsert(); err != nil {
			return errors.Wrapf(err, "overwriting variables for project '%s'", vars.Id)
		}
	} else {
		_, err = vars.FindAndModify(varsModel.VarsToDelete)
		if err != nil {
			return errors.Wrapf(err, "updating variables for project '%s'", vars.Id)
		}
	}

	vars = vars.RedactPrivateVars()
	varsModel.Vars = vars.Vars
	varsModel.PrivateVars = vars.PrivateVars
	varsModel.AdminOnlyVars = vars.AdminOnlyVars
	varsModel.VarsToDelete = []string{}
	return nil
}

func GetProjectEventLog(project string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
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
	return GetEventsById(id, before, n)
}

func GetEventsById(id string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
	if n == 0 {
		n = EventLogLimit
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
			catcher.Wrapf(err, "converting event '%s' to API model", evt.ID)
			continue
		}
		out = append(out, apiEvent)
	}
	return out, catcher.Resolve()
}

func GetProjectAliasResults(p *model.Project, alias string, includeDeps bool) ([]restModel.APIVariantTasks, error) {
	projectAliases, err := model.FindAliasInProjectRepoOrConfig(p.Identifier, alias)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no alias named '%s' for project '%s'", alias, p.Identifier),
		}
	}
	matches := []restModel.APIVariantTasks{}
	for _, projectAlias := range projectAliases {
		requester := getRequesterFromAlias(projectAlias.Alias)
		_, _, variantTasks := p.ResolvePatchVTs(&patch.Patch{}, requester, projectAlias.Alias, includeDeps)
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

func (pc *DBProjectConnector) GetProjectFromFile(ctx context.Context, pRef model.ProjectRef, file string, token string) (model.ProjectInfo, error) {
	opts := model.GetProjectOpts{
		Ref:        &pRef,
		Revision:   pRef.Branch,
		RemotePath: file,
		Token:      token,
	}
	return model.GetProjectFromFile(ctx, opts)
}
