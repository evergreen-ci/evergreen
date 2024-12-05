package data

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
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
		Fields: map[string]interface{}{
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

	err = notification.InsertMany(*n)
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
		if err := ValidateProjectName(projectRef.Identifier); err != nil {
			return false, err
		}
	}
	if projectRef.Id == "" {
		if projectRef.Id == "" {
			projectRef.Id = mgobson.NewObjectId().Hex()
		}
	}
	if err := ValidateProjectName(projectRef.Id); err != nil {
		return false, err
	}
	// Always warn because created projects are never enabled.
	warningCatcher := grip.NewBasicCatcher()
	statusCode, err := model.ValidateEnabledProjectsLimit(projectRef.Id, env.Settings(), nil, projectRef)
	if err != nil {
		if statusCode != http.StatusBadRequest {
			return false, gimlet.ErrorResponse{
				StatusCode: statusCode,
				Message:    errors.Wrapf(err, "validating project '%s'", projectRef.Identifier).Error(),
			}
		}
		warningCatcher.Add(err)
	}

	existingContainerSecrets := projectRef.ContainerSecrets
	projectRef.ContainerSecrets = nil

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

	if err = projectRef.Add(u); err != nil {
		return false, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "inserting project '%s'", projectRef.Identifier).Error(),
		}
	}

	if err = tryCopyingContainerSecrets(ctx, env.Settings(), existingContainerSecrets, projectRef); err != nil {
		return false, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "copying container secrets for project '%s'", projectRef.Identifier).Error(),
		}
	}

	err = model.LogProjectAdded(projectRef.Id, u.DisplayName())
	grip.Error(message.WrapError(err, message.Fields{
		"message":            "problem logging project added",
		"project_id":         projectRef.Id,
		"project_identifier": projectRef.Identifier,
		"user":               u.DisplayName(),
	}))
	return true, warningCatcher.Resolve()
}

func tryCopyingContainerSecrets(ctx context.Context, settings *evergreen.Settings, existingSecrets []model.ContainerSecret, pRef *model.ProjectRef) error {
	smClient, err := cloud.MakeSecretsManagerClient(ctx, settings)
	if err != nil {
		return errors.Wrap(err, "setting up Secrets Manager client to store newly-created project's container secrets")
	}

	vault, err := cloud.MakeSecretsManagerVault(smClient)
	if err != nil {
		return errors.Wrap(err, "setting up Secrets Manager vault to store newly-created project's container secrets")
	}

	secrets, err := getCopiedContainerSecrets(ctx, settings, vault, pRef.Id, existingSecrets)
	if err != nil {
		return errors.Wrapf(err, "copying existing container secrets")
	}
	if err := pRef.SetContainerSecrets(secrets); err != nil {
		return errors.Wrap(err, "setting container secrets")
	}

	// Under the hood, this is updating the container secrets in the DB project
	// ref, but this function's copy of the in-memory project ref won't reflect
	// those changes.
	if err := UpsertContainerSecrets(ctx, vault, secrets); err != nil {
		return errors.Wrapf(err, "upserting container secrets")
	}

	return nil
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
func ValidateProjectName(name string) error {
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
	tasks, err := model.GetTasksWithOptions(projectName, taskName, opts)
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
	varsModel.BuildFromService(*vars)
	return &varsModel, nil
}

// UpdateProjectVars adds new variables, overwrites variables, and deletes
// variables for the given project. If overwrite is true, the project variables
// will be fully replaced by those in varsModel. Otherwise, it will only set the
// value for variables that are explicitly present in varsModel and will not
// delete variables that are omitted.
func UpdateProjectVars(projectId string, varsModel *restModel.APIProjectVars, overwrite bool) error {
	if varsModel == nil {
		return nil
	}
	vars := varsModel.ToService()
	vars.Id = projectId

	// Avoid accidentally overwriting private variables, for example if the GET route is used to populate PATCH.
	for key, val := range varsModel.Vars {
		if val == "" {
			delete(varsModel.Vars, key)
		}
	}
	if overwrite {
		if _, err := vars.Upsert(); err != nil {
			return errors.Wrapf(err, "overwriting variables for project '%s'", vars.Id)
		}
	} else {
		_, err := vars.FindAndModify(varsModel.VarsToDelete)
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
	events.ApplyDefaults()

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

func (pc *DBProjectConnector) GetProjectFromFile(ctx context.Context, pRef model.ProjectRef, file string) (model.ProjectInfo, error) {
	opts := model.GetProjectOpts{
		Ref:        &pRef,
		Revision:   pRef.Branch,
		RemotePath: file,
	}
	return model.GetProjectFromFile(ctx, opts)
}

// HideBranch is used to "delete" a project via the rest route or the UI. It overwrites the project with a skeleton project.
// It also clears project admin roles, project aliases, and project vars.
func HideBranch(projectID string) error {
	pRef, err := model.FindBranchProjectRef(projectID)
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
		Id:                    pRef.Id,
		Owner:                 pRef.Owner,
		Repo:                  pRef.Repo,
		Branch:                pRef.Branch,
		RepoRefId:             pRef.RepoRefId,
		Enabled:               false,
		Hidden:                utility.TruePtr(),
		ParameterStoreEnabled: pRef.ParameterStoreEnabled,
	}
	if err := skeletonProj.Upsert(); err != nil {
		return errors.Wrapf(err, "updating project '%s'", pRef.Id)
	}
	if err := model.UpdateAdminRoles(pRef, nil, pRef.Admins); err != nil {
		return errors.Wrapf(err, "removing project admin roles")
	}

	projectAliases, err := model.FindAliasesForProjectFromDb(pRef.Id)
	if err != nil {
		return errors.Wrapf(err, "finding aliases for project '%s'", pRef.Id)
	}
	for _, alias := range projectAliases {
		if err := model.RemoveProjectAlias(alias.ID.Hex()); err != nil {
			return errors.Wrapf(err, "removing project alias '%s' for project '%s'", alias.ID.Hex(), pRef.Id)
		}
	}

	skeletonProjVars := model.ProjectVars{
		Id: pRef.Id,
	}
	if _, err := skeletonProjVars.Upsert(); err != nil {
		return errors.Wrapf(err, "updating vars for project '%s'", pRef.Id)
	}

	return nil
}
