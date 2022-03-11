package data

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const EventLogLimit = 10

// DBProjectConnector is a struct that implements the Project related methods
// from the Connector through interactions with the backing database.
type DBProjectConnector struct{}

// FindProjectById queries the database for the project matching the projectRef.Id.
func FindProjectById(id string, includeRepo bool, includeParserProject bool) (*model.ProjectRef, error) {
	var p *model.ProjectRef
	var err error
	if includeRepo && includeParserProject {
		p, err = model.FindMergedProjectRef(id, "", true)
	} else if includeRepo {
		p, err = model.FindMergedProjectRef(id, "", false)
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
			Message:    errors.Wrapf(err, "project with id '%s' was not inserted", projectRef.Id).Error(),
		}
	}
	return nil
}

// UpdateProject updates the given model.ProjectRef.Id.
func UpdateProject(projectRef *model.ProjectRef) error {
	err := projectRef.Update()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("project with id '%s' was not updated", projectRef.Id),
		}
	}
	return nil
}

func VerifyUniqueProject(name string) error {
	_, err := FindProjectById(name, false, false)
	if err == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot reuse '%s' for project", name),
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

// UpdateRepo updates the given model.RepoRef.Id. We use EnsureCommitQueueExistsForProject to ensure that only values relevant to repos get updated.
func UpdateRepo(repoRef *model.RepoRef) error {
	err := repoRef.Upsert()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("repo with id '%s' was not updated", repoRef.Id),
		}
	}
	return nil
}

// EnableWebhooks returns true if a hook for the given owner/repo exists or was inserted.
func EnableWebhooks(ctx context.Context, projectRef *model.ProjectRef) (bool, error) {
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

func EnablePRTesting(projectRef *model.ProjectRef) error {
	conflicts, err := projectRef.GetGithubProjectConflicts()
	if err != nil {
		return errors.Wrap(err, "error finding project refs")
	}
	if len(conflicts.PRTestingIdentifiers) > 0 {
		return errors.Errorf("Cannot enable PR Testing in this repo, must disable in other projects first")

	}
	return nil
}

func EnableCommitQueue(projectRef *model.ProjectRef) error {
	if ok, err := projectRef.CanEnableCommitQueue(); err != nil {
		return errors.Wrap(err, "error enabling commit queue")
	} else if !ok {
		return errors.Errorf("Cannot enable commit queue in this repo, must disable in other projects first")
	}

	return commitqueue.EnsureCommitQueueExistsForProject(projectRef.Id)
}

func UpdateProjectRevision(projectID, revision string) error {
	if err := model.UpdateLastRevision(projectID, revision); err != nil {
		return errors.Wrapf(err, "error updating revision for project '%s'", projectID)
	}

	return nil
}

// FindProjects queries the backing database for the specified projects
func FindProjects(key string, limit int, sortDir int) ([]model.ProjectRef, error) {
	projects, err := model.FindProjectRefs(key, limit, sortDir)
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching projects starting at project '%s'", key)
	}

	return projects, nil
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
		return nil, errors.Wrap(err, "error building project variables from service")
	}
	return &varsModel, nil
}

func UpdateAdminRoles(project *model.ProjectRef, toAdd, toDelete []string) error {
	if project == nil {
		return errors.New("no project found")
	}
	_, err := project.UpdateAdminRoles(toAdd, toDelete)
	return err
}

// UpdateProjectVars adds new variables, overwrites variables, and deletes variables for the given project.
func UpdateProjectVars(projectId string, varsModel *restModel.APIProjectVars, overwrite bool) error {
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
	varsModel.AdminOnlyVars = vars.AdminOnlyVars
	varsModel.VarsToDelete = []string{}
	return nil
}

func UpdateProjectVarsByValue(toReplace, replacement, username string, dryRun bool) (map[string][]string, error) {
	catcher := grip.NewBasicCatcher()
	matchingProjects, err := model.GetVarsByValue(toReplace)
	if err != nil {
		catcher.Wrap(err, "failed to fetch projects with matching value")
	}
	if matchingProjects == nil {
		catcher.New("no projects with matching value found")
	}
	changes := map[string][]string{}
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
				changes[project.Id] = append(changes[project.Id], key)
			}
		}
	}
	return changes, catcher.Resolve()
}

func CopyProjectVars(oldProjectId, newProjectId string) error {
	vars, err := model.FindOneProjectVars(oldProjectId)
	if err != nil {
		return errors.Wrapf(err, "error finding variables for project '%s'", oldProjectId)
	}
	vars.Id = newProjectId
	return errors.Wrapf(vars.Insert(), "error inserting variables for project '%s", newProjectId)
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
	return getEventsById(id, before, n)
}

func GetRepoEventLog(repoId string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
	return getEventsById(repoId, before, n)
}

func getEventsById(id string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
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
			catcher.Add(err)
			continue
		}
		out = append(out, apiEvent)
	}
	return out, catcher.Resolve()
}

func GetProjectWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch string) (*model.ProjectRef, error) {
	proj, err := model.FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, errors.Wrapf(err, "can't query for projectRef %s/%s tracking %s", owner, repo, branch)
	}

	return proj, nil
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
