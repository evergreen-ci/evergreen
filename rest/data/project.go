package data

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
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

// FindProjectById queries the database for the project matching the projectRef.Id. If the bool flag is set,
// the project config properties in the project YAML will be merged into the result if the properties are
// not set on the project page.
func FindProjectById(ctx context.Context, id string, includeRepo bool, includeProjectConfig bool) (*model.ProjectRef, error) {
	var p *model.ProjectRef
	var err error
	if includeRepo && includeProjectConfig {
		p, err = model.FindMergedProjectRef(ctx, id, "", true)
	} else if includeRepo {
		p, err = model.FindMergedProjectRef(ctx, id, "", false)
	} else {
		p, err = model.FindBranchProjectRef(ctx, id)
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

// RequestS3Creds creates a JIRA ticket that requests S3 credentials to be added for the specified project.
// TODO DEVPROD-5553: Remove the function after project completion.
func RequestS3Creds(ctx context.Context, projectIdentifier, userEmail string) error {
	if projectIdentifier == "" {
		return errors.New("project identifier cannot be empty")
	}
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "getting evergreen settings")
	}
	if settings.ProjectCreation.JiraProject == "" {
		return nil
	}
	summary := fmt.Sprintf("Create AWS bucket for s3 uploads for '%s' project", projectIdentifier)
	description := fmt.Sprintf("Could you create an s3 bucket and role arn for the new [%s|%s/project/%s/settings/general] project?", projectIdentifier, settings.Ui.UIv2Url, projectIdentifier)
	jiraIssue := message.JiraIssue{
		Project:     settings.ProjectCreation.JiraProject,
		Summary:     summary,
		Description: description,
		Reporter:    userEmail,
		Fields: map[string]any{
			evergreen.DevProdServiceFieldName: evergreen.DevProdJiraServiceField,
		},
	}

	sub := event.Subscriber{
		Type: event.JIRAIssueSubscriberType,
		Target: event.JIRAIssueSubscriber{
			Project:   settings.ProjectCreation.JiraProject,
			IssueType: "Task",
		},
	}
	n, err := notification.New("", utility.RandomString(), &sub, jiraIssue)
	if err != nil {
		return err
	}

	err = notification.InsertMany(ctx, *n)
	if err != nil {
		return errors.Wrap(err, "batch inserting notifications")
	}
	return nil
}

// CreateProject creates a new project ref from the given one and performs other
// initial setup for new projects such as populating initial project variables
// and creating new webhooks. If the given project ref already has container
// secrets, the new project ref receives copies of the existing ones.
// Returns true if the project was successfully created.
func CreateProject(ctx context.Context, env evergreen.Environment, projectRef *model.ProjectRef, u *user.DBUser) (bool, error) {
	if projectRef.Identifier != "" {
		if err := ValidateProjectName(ctx, projectRef.Identifier); err != nil {
			return false, err
		}
	}
	if projectRef.Id == "" {
		if projectRef.Id == "" {
			projectRef.Id = mgobson.NewObjectId().Hex()
		}
	}
	if err := ValidateProjectName(ctx, projectRef.Id); err != nil {
		return false, err
	}
	// Always warn because created projects are never enabled.
	warningCatcher := grip.NewBasicCatcher()
	statusCode, err := model.ValidateEnabledProjectsLimit(ctx, env.Settings(), nil, projectRef)
	if err != nil {
		if statusCode != http.StatusBadRequest {
			return false, gimlet.ErrorResponse{
				StatusCode: statusCode,
				Message:    errors.Wrapf(err, "validating project '%s'", projectRef.Identifier).Error(),
			}
		}
		warningCatcher.Add(err)
	}

	_, err = model.SetTracksPushEvents(ctx, projectRef)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message":            "error setting project tracks push events",
			"project_id":         projectRef.Id,
			"project_identifier": projectRef.Identifier,
			"owner":              projectRef.Owner,
			"repo":               projectRef.Repo,
		}))
	}

	if err = projectRef.Add(ctx, u); err != nil {
		return false, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "inserting project '%s'", projectRef.Identifier).Error(),
		}
	}

	err = model.LogProjectAdded(ctx, projectRef.Id, u.DisplayName())
	grip.Error(message.WrapError(err, message.Fields{
		"message":            "problem logging project added",
		"project_id":         projectRef.Id,
		"project_identifier": projectRef.Identifier,
		"user":               u.DisplayName(),
	}))
	return true, warningCatcher.Resolve()
}

// projectIDRegexp includes all the allowed characters in a project
// ID/identifier (i.e. those that do not require percent encoding for URLs).
// These are listed in RFC-3986:
// https://www.rfc-editor.org/rfc/rfc3986#section-2.3
// Note that this also includes parentheses and spaces, which may actually
// require percent encoding, for backward compatibility because some projects
// have already chosen to use those characters in their project ID/identifier.
var projectIDRegexp = regexp.MustCompile(`^[0-9a-zA-Z-._~\(\) ]*$`)

// ValidateProjectName checks that a project ID / identifier is not already in use
// and has only valid characters.
func ValidateProjectName(ctx context.Context, name string) error {
	_, err := FindProjectById(ctx, name, false, false)
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

	if !projectIDRegexp.MatchString(name) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("project name '%s' contains invalid characters", name),
		}
	}

	return nil
}

// GetProjectTasksWithOptions finds the previous tasks that have run on a project that adhere to the passed in options.
func GetProjectTasksWithOptions(ctx context.Context, projectName string, taskName string, opts model.GetProjectTasksOpts) ([]restModel.APITask, error) {
	tasks, err := model.GetTasksWithOptions(ctx, projectName, taskName, opts)
	if err != nil {
		return nil, err
	}
	res := []restModel.APITask{}
	for _, t := range tasks {
		apiTask := restModel.APITask{}
		if err = apiTask.BuildFromService(ctx, &t, &restModel.APITaskArgs{
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
func FindProjectVarsById(ctx context.Context, id string, repoId string, redact bool) (*restModel.APIProjectVars, error) {
	var repoVars *model.ProjectVars
	var err error
	if repoId != "" {
		repoVars, err = model.FindOneProjectVars(ctx, repoId)
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
		vars, err = model.FindOneProjectVars(ctx, id)
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
	varsModel.BuildFromService(*vars)
	return &varsModel, nil
}

// UpdateProjectVars adds new variables, overwrites variables, and deletes
// variables for the given project. If overwrite is true, the project variables
// will be fully replaced by those in varsModel. Otherwise, it will only set the
// value for variables that are explicitly present in varsModel and will not
// delete variables that are omitted.
func UpdateProjectVars(ctx context.Context, projectId string, varsModel *restModel.APIProjectVars, overwrite bool) error {
	if varsModel == nil {
		return nil
	}
	vars := varsModel.ToService()
	vars.Id = projectId

	// Avoid accidentally overwriting private variables, for example if the GET route is used to populate PATCH.
	for key, val := range vars.Vars {
		if val == "" {
			delete(vars.Vars, key)
		}
	}
	if overwrite {
		if _, err := vars.Upsert(ctx); err != nil {
			return errors.Wrapf(err, "overwriting variables for project '%s'", vars.Id)
		}
	} else {
		_, err := vars.FindAndModify(ctx, varsModel.VarsToDelete)
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

func GetProjectEventLog(ctx context.Context, project string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
	id, err := model.GetIdForProject(ctx, project)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"func":    "GetProjectEventLog",
			"message": "error getting id for project",
			"project": project,
		}))
		// don't return an error here to preserve existing behavior
		return nil, nil
	}
	return GetEventsById(ctx, id, before, n)
}

func GetEventsById(ctx context.Context, id string, before time.Time, n int) ([]restModel.APIProjectEvent, error) {
	if n == 0 {
		n = EventLogLimit
	}
	events, err := model.ProjectEventsBefore(ctx, id, before, n)
	if err != nil {
		return nil, err
	}
	events.ApplyDefaults()

	out := []restModel.APIProjectEvent{}
	catcher := grip.NewBasicCatcher()
	for _, evt := range events {
		apiEvent := restModel.APIProjectEvent{}
		err = apiEvent.BuildFromService(ctx, evt)
		if err != nil {
			catcher.Wrapf(err, "converting event '%s' to API model", evt.ID)
			continue
		}
		out = append(out, apiEvent)
	}
	return out, catcher.Resolve()
}

func GetProjectAliasResults(ctx context.Context, p *model.Project, alias string, includeDeps bool) ([]restModel.APIVariantTasks, error) {
	projectAliases, err := model.FindAliasInProjectRepoOrConfig(ctx, p.Identifier, alias)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no alias named '%s' for project '%s'", alias, p.Identifier),
		}
	}
	matches := []restModel.APIVariantTasks{}
	for _, projectAlias := range projectAliases {
		requester := getRequesterFromAlias(projectAlias.Alias)
		_, _, variantTasks := p.ResolvePatchVTs(ctx, &patch.Patch{}, requester, projectAlias.Alias, includeDeps)
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
		return evergreen.GithubMergeRequester
	}
	return evergreen.PatchVersionRequester
}

func (pc *DBProjectConnector) GetProjectFromFile(ctx context.Context, pRef model.ProjectRef, file string) (model.ProjectInfo, error) {
	opts := model.GetProjectOpts{
		Ref:          &pRef,
		Revision:     pRef.Branch,
		RemotePath:   file,
		ReadFileFrom: model.ReadFromGithub,
	}
	return model.GetProjectFromFile(ctx, opts)
}

// HideBranch is used to "delete" a project via the rest route or the UI. It overwrites the project with a skeleton project.
// It also clears project admin roles, project aliases, and project vars.
func HideBranch(ctx context.Context, projectID string) error {
	pRef, err := model.FindBranchProjectRef(ctx, projectID)
	if err != nil {
		return errors.Wrapf(err, "finding project '%s'", projectID)
	}
	if pRef == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Errorf("project '%s' not found", projectID).Error(),
		}
	}

	if pRef.IsHidden() {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Errorf("project '%s' is already hidden", pRef.Id).Error(),
		}
	}

	skeletonProj := model.ProjectRef{
		Id:        pRef.Id,
		Owner:     pRef.Owner,
		Repo:      pRef.Repo,
		Branch:    pRef.Branch,
		RepoRefId: pRef.RepoRefId,
		Enabled:   false,
		Hidden:    utility.TruePtr(),
	}
	if err := skeletonProj.Replace(ctx); err != nil {
		return errors.Wrapf(err, "updating project '%s'", pRef.Id)
	}
	if err := model.UpdateAdminRoles(ctx, pRef, nil, pRef.Admins); err != nil {
		return errors.Wrapf(err, "removing project admin roles")
	}

	projectAliases, err := model.FindAliasesForProjectFromDb(ctx, pRef.Id)
	if err != nil {
		return errors.Wrapf(err, "finding aliases for project '%s'", pRef.Id)
	}
	for _, alias := range projectAliases {
		if err := model.RemoveProjectAlias(ctx, alias.ID.Hex()); err != nil {
			return errors.Wrapf(err, "removing project alias '%s' for project '%s'", alias.ID.Hex(), pRef.Id)
		}
	}

	skeletonProjVars := model.ProjectVars{
		Id: pRef.Id,
	}
	if _, err := skeletonProjVars.Upsert(ctx); err != nil {
		return errors.Wrapf(err, "updating vars for project '%s'", pRef.Id)
	}

	if err = pRef.SetGithubAppCredentials(ctx, 0, nil); err != nil {
		return errors.Wrapf(err, "removing GitHub app credentials for project '%s'", pRef.Id)
	}

	return nil
}
