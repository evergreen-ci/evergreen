package graphql

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/api"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	werrors "github.com/pkg/errors"
)

// BbCreateTicket is the resolver for the bbCreateTicket field.
func (r *mutationResolver) BbCreateTicket(ctx context.Context, taskID string, execution *int) (bool, error) {
	err := annotationPermissionHelper(ctx, taskID, execution)
	if err != nil {
		return false, err
	}
	httpStatus, err := data.BbFileTicket(ctx, taskID, *execution)
	if err != nil {
		return false, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}
	return true, nil
}

// AddAnnotationIssue is the resolver for the addAnnotationIssue field.
func (r *mutationResolver) AddAnnotationIssue(ctx context.Context, taskID string, execution int, apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	err := annotationPermissionHelper(ctx, taskID, utility.ToIntPtr(execution))
	if err != nil {
		return false, err
	}
	usr := mustHaveUser(ctx)
	issue := restModel.APIIssueLinkToService(apiIssue)
	if err := util.CheckURL(issue.URL); err != nil {
		return false, InputValidationError.Send(ctx, fmt.Sprintf("issue does not have valid URL: %s", err.Error()))
	}
	if isIssue {
		if err := task.AddIssueToAnnotation(ctx, taskID, execution, *issue, usr.Username()); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("adding issue: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.AddSuspectedIssueToAnnotation(ctx, taskID, execution, *issue, usr.Username()); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("adding suspected issue: %s", err.Error()))
		}
		return true, nil
	}
}

// EditAnnotationNote is the resolver for the editAnnotationNote field.
func (r *mutationResolver) EditAnnotationNote(ctx context.Context, taskID string, execution int, originalMessage string, newMessage string) (bool, error) {
	err := annotationPermissionHelper(ctx, taskID, utility.ToIntPtr(execution))
	if err != nil {
		return false, err
	}
	usr := mustHaveUser(ctx)
	if err := annotations.UpdateAnnotationNote(ctx, taskID, execution, originalMessage, newMessage, usr.Username()); err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("updating note: %s", err.Error()))
	}
	return true, nil
}

// MoveAnnotationIssue is the resolver for the moveAnnotationIssue field.
func (r *mutationResolver) MoveAnnotationIssue(ctx context.Context, taskID string, execution int, apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	err := annotationPermissionHelper(ctx, taskID, utility.ToIntPtr(execution))
	if err != nil {
		return false, err
	}
	usr := mustHaveUser(ctx)
	issue := restModel.APIIssueLinkToService(apiIssue)
	if isIssue {
		if err := task.MoveIssueToSuspectedIssue(ctx, taskID, execution, *issue, usr.Username()); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("moving issue to suspected issues: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := task.MoveSuspectedIssueToIssue(ctx, taskID, execution, *issue, usr.Username()); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("moving suspected issue to issues: %s", err.Error()))
		}
		return true, nil
	}
}

// RemoveAnnotationIssue is the resolver for the removeAnnotationIssue field.
func (r *mutationResolver) RemoveAnnotationIssue(ctx context.Context, taskID string, execution int, apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	err := annotationPermissionHelper(ctx, taskID, utility.ToIntPtr(execution))
	if err != nil {
		return false, err
	}
	issue := restModel.APIIssueLinkToService(apiIssue)
	if isIssue {
		if err := task.RemoveIssueFromAnnotation(ctx, taskID, execution, *issue); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("deleting issue: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.RemoveSuspectedIssueFromAnnotation(ctx, taskID, execution, *issue); err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("deleting suspected issue: %s", err.Error()))
		}
		return true, nil
	}
}

// SetAnnotationMetadataLinks is the resolver for the setAnnotationMetadataLinks field.
func (r *mutationResolver) SetAnnotationMetadataLinks(ctx context.Context, taskID string, execution int, metadataLinks []*restModel.APIMetadataLink) (bool, error) {
	err := annotationPermissionHelper(ctx, taskID, utility.ToIntPtr(execution))
	if err != nil {
		return false, err
	}
	usr := mustHaveUser(ctx)
	modelMetadataLinks := restModel.APIMetadataLinksToService(metadataLinks)
	if err := annotations.ValidateMetadataLinks(modelMetadataLinks...); err != nil {
		return false, InputValidationError.Send(ctx, fmt.Sprintf("invalid metadata link: %s", err.Error()))
	}
	if err := annotations.SetAnnotationMetadataLinks(ctx, taskID, execution, usr.Username(), modelMetadataLinks...); err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("setting metadata link: %s", err.Error()))
	}
	return true, nil
}

// SaveAdminSettings is the resolver for the saveAdminSettings field.
func (r *mutationResolver) SaveAdminSettings(ctx context.Context, adminSettings restModel.APIAdminSettings) (*restModel.APIAdminSettings, error) {
	oldSettings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	newSettingsUnredacted, err := data.SetEvergreenSettings(ctx, &adminSettings, oldSettings, mustHaveUser(ctx), false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("setting admin settings: %s", err.Error()))
	}
	if err = newSettingsUnredacted.Validate(); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("setting admin settings: %s", err.Error()))
	}

	oldSettingsRedacted, err := evergreen.GetConfigWithoutSecrets(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	newApiAdminSettings := restModel.NewConfigModel()
	if err := newApiAdminSettings.BuildFromService(newSettingsUnredacted); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting updated settings to API model: %s", err.Error()))
	}
	newSettingsRedacted, err := data.SetEvergreenSettings(ctx, newApiAdminSettings, oldSettingsRedacted, mustHaveUser(ctx), true)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("setting updated Evergreen configuration: %s", err.Error()))
	}

	updatedAdminSettings := restModel.NewConfigModel()
	if err := updatedAdminSettings.BuildFromService(newSettingsRedacted); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting updated settings to API model: %s", err.Error()))
	}
	return updatedAdminSettings, nil
}

// SetServiceFlags is the resolver for the setServiceFlags field.
func (r *mutationResolver) SetServiceFlags(ctx context.Context, updatedFlags []*ServiceFlagInput) ([]*ServiceFlag, error) {
	usr := mustHaveUser(ctx)
	currentFlags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting service flags: %s", err.Error()))
	}
	oldFlags := *currentFlags
	for _, flag := range updatedFlags {
		if flag == nil {
			continue
		}
		if err = currentFlags.SetByName(flag.Name, flag.Enabled); err != nil {
			return nil, InputValidationError.Send(ctx, err.Error())
		}
	}
	if err = evergreen.SetServiceFlags(ctx, *currentFlags); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("setting service flags: %s", err.Error()))
	}
	if err = event.LogAdminEvent(ctx, currentFlags.SectionId(), &oldFlags, currentFlags, usr.Username()); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("logging service flag changes: %s", err.Error()))
	}
	entries := currentFlags.ToSlice()
	result := make([]*ServiceFlag, 0, len(entries))
	for _, e := range entries {
		result = append(result, &ServiceFlag{Name: e.Name, Enabled: e.Enabled})
	}
	return result, nil
}

// RestartAdminTasks is the resolver for the restartAdminTasks field.
func (r *mutationResolver) RestartAdminTasks(ctx context.Context, opts model.RestartOptions) (*RestartAdminTasksPayload, error) {
	env := evergreen.GetEnvironment()
	usr := mustHaveUser(ctx)
	opts.User = usr.Username()

	// Start with DryRun = true to get the list of tasks that will be restarted.
	opts.DryRun = true
	results, err := data.RestartFailedTasks(ctx, env.RemoteQueue(), opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching restart tasks: %s", err.Error()))
	}
	numRestartedTasks := len(results.ItemsRestarted)

	// Actually restart the tasks by setting DryRun = false.
	opts.DryRun = false
	if _, err = data.RestartFailedTasks(ctx, env.RemoteQueue(), opts); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("restarting tasks: %s", err.Error()))
	}
	return &RestartAdminTasksPayload{
		NumRestartedTasks: numRestartedTasks,
	}, nil
}

// DeleteDistro is the resolver for the deleteDistro field.
func (r *mutationResolver) DeleteDistro(ctx context.Context, opts DeleteDistroInput) (*DeleteDistroPayload, error) {
	usr := mustHaveUser(ctx)
	if err := data.DeleteDistroById(ctx, usr, opts.DistroID); err != nil {
		gimletErr, ok := err.(gimlet.ErrorResponse)
		if ok {
			return nil, mapHTTPStatusToGqlError(ctx, gimletErr.StatusCode, err)
		}
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("deleting distro '%s': %s", opts.DistroID, err.Error()))
	}
	return &DeleteDistroPayload{
		DeletedDistroID: opts.DistroID,
	}, nil
}

// CopyDistro is the resolver for the copyDistro field.
func (r *mutationResolver) CopyDistro(ctx context.Context, opts restModel.CopyDistroOpts) (*NewDistroPayload, error) {
	usr := mustHaveUser(ctx)

	if err := data.CopyDistro(ctx, usr, opts); err != nil {
		gimletErr, ok := err.(gimlet.ErrorResponse)
		if ok {
			return nil, mapHTTPStatusToGqlError(ctx, gimletErr.StatusCode, err)
		}
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("copying distro: %s", err.Error()))
	}

	return &NewDistroPayload{
		NewDistroID: opts.NewDistroId,
	}, nil
}

// CreateDistro is the resolver for the createDistro field.
func (r *mutationResolver) CreateDistro(ctx context.Context, opts CreateDistroInput) (*NewDistroPayload, error) {
	usr := mustHaveUser(ctx)

	if err := data.CreateDistro(ctx, usr, opts.NewDistroID, utility.FromBoolPtr(opts.SingleTaskDistro)); err != nil {
		gimletErr, ok := err.(gimlet.ErrorResponse)
		if ok {
			return nil, mapHTTPStatusToGqlError(ctx, gimletErr.StatusCode, err)
		}
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("creating distro: %s", err.Error()))
	}

	return &NewDistroPayload{
		NewDistroID: opts.NewDistroID,
	}, nil
}

// SaveDistro is the resolver for the saveDistro field. The entire distro object is provided as input (not just the updated fields) in order to validate all distro settings.
func (r *mutationResolver) SaveDistro(ctx context.Context, opts SaveDistroInput) (*SaveDistroPayload, error) {
	usr := mustHaveUser(ctx)
	d := opts.Distro.ToService()
	oldDistro, err := distro.FindOneId(ctx, d.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching distro '%s': %s", d.Id, err.Error()))
	}
	if oldDistro == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("distro '%s' not found", d.Id))
	}

	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	validationErrs, err := validator.CheckDistro(ctx, d, settings, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	if len(validationErrs) != 0 {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("validating changes for distro '%s': '%s'", d.Id, validationErrs.String()))
	}

	if err = data.UpdateDistro(ctx, oldDistro, d); err != nil {
		gimletErr, ok := err.(gimlet.ErrorResponse)
		if ok {
			return nil, mapHTTPStatusToGqlError(ctx, gimletErr.StatusCode, err)
		}
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("updating distro '%s': %s", d.Id, err.Error()))
	}
	event.LogDistroModified(ctx, d.Id, usr.Username(), oldDistro.DistroData(), d.DistroData())

	// AMI events are not displayed in the event log, but are used by the backend to determine if hosts have become stale.
	if d.GetDefaultAMI() != oldDistro.GetDefaultAMI() {
		event.LogDistroAMIModified(ctx, d.Id, usr.Username())
	}

	numHostsUpdated, err := handleDistroOnSaveOperation(ctx, d.Id, opts.OnSave, usr.Username())
	if err != nil {
		graphql.AddError(ctx, PartialError.Send(ctx, err.Error()))
	}

	return &SaveDistroPayload{
		Distro:    opts.Distro,
		HostCount: numHostsUpdated,
	}, nil
}

// ReprovisionToNew is the resolver for the reprovisionToNew field.
func (r *mutationResolver) ReprovisionToNew(ctx context.Context, hostIds []string) (int, error) {
	user := mustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(ctx, user, hostIds)
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetReprovisionToNewCallback(ctx, evergreen.GetEnvironment(), user.Username()))
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, werrors.Errorf("marking selected hosts as needing to reprovision: %s", err.Error()))
	}

	return hostsUpdated, nil
}

// RestartJasper is the resolver for the restartJasper field.
func (r *mutationResolver) RestartJasper(ctx context.Context, hostIds []string) (int, error) {
	user := mustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(ctx, user, hostIds)
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetRestartJasperCallback(ctx, evergreen.GetEnvironment(), user.Username()))
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, werrors.Errorf("marking selected hosts as needing Jasper service restarted: %s", err.Error()))
	}

	return hostsUpdated, nil
}

// UpdateHostStatus is the resolver for the updateHostStatus field.
func (r *mutationResolver) UpdateHostStatus(ctx context.Context, hostIds []string, status string, notes *string) (int, error) {
	user := mustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(ctx, user, hostIds)
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetUpdateHostStatusCallback(ctx, evergreen.GetEnvironment(), status, *notes, user))
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	return hostsUpdated, nil
}

// SetPatchVisibility is the resolver for the setPatchVisibility field.
func (r *mutationResolver) SetPatchVisibility(ctx context.Context, patchIds []string, hidden bool) ([]*restModel.APIPatch, error) {
	user := mustHaveUser(ctx)
	updatedPatches := []*restModel.APIPatch{}
	patches, err := patch.Find(ctx, patch.ByStringIds(patchIds))

	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patches '%s': %s", patchIds, err.Error()))
	}

	for _, p := range patches {
		if !userCanModifyPatch(ctx, user, p) {
			return nil, Forbidden.Send(ctx, fmt.Sprintf("not authorized to change visibility of patch '%s'", p.Id))
		}
		err = p.SetPatchVisibility(ctx, hidden)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("setting visibility for patch '%s': %s", p.Id, err.Error()))
		}
		apiPatch := restModel.APIPatch{}
		err = apiPatch.BuildFromService(ctx, p, &restModel.APIPatchArgs{IncludeProjectIdentifier: true})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting patch '%s' to APIPatch: %s", p.Id, err.Error()))
		}
		updatedPatches = append(updatedPatches, &apiPatch)
	}
	return updatedPatches, nil
}

// SchedulePatch is the resolver for the schedulePatch field.
func (r *mutationResolver) SchedulePatch(ctx context.Context, patchID string, configure PatchConfigure) (*restModel.APIPatch, error) {
	patchUpdateReq := buildFromGqlInput(configure)
	usr := mustHaveUser(ctx)
	patchUpdateReq.Caller = usr.Id
	version, err := model.VersionFindOneId(ctx, patchID)
	if err != nil && !adb.ResultsNotFound(err) {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch '%s': %s", patchID, err.Error()))
	}
	statusCode, err := units.SchedulePatch(ctx, evergreen.GetEnvironment(), patchID, version, patchUpdateReq)
	if err != nil {
		return nil, mapHTTPStatusToGqlError(ctx, statusCode, werrors.Errorf("scheduling patch '%s': %s", patchID, err.Error()))
	}
	scheduledPatch, err := data.FindPatchById(ctx, patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting scheduled patch '%s': %s", patchID, err.Error()))
	}
	return scheduledPatch, nil
}

// AttachProjectToNewRepo is the resolver for the attachProjectToNewRepo field.
func (r *mutationResolver) AttachProjectToNewRepo(ctx context.Context, project MoveProjectInput) (*restModel.APIProjectRef, error) {
	usr := mustHaveUser(ctx)
	pRef, err := data.FindProjectById(ctx, project.ProjectID, false, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", project.ProjectID, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", project.ProjectID))
	}
	pRef.Owner = project.NewOwner
	pRef.Repo = project.NewRepo

	if err = pRef.AttachToNewRepo(ctx, usr); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("updating owner/repo for project '%s': %s", project.ProjectID, err.Error()))
	}

	res := &restModel.APIProjectRef{}
	if err = res.BuildFromService(ctx, *pRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting project '%s' to APIProjectRef: %s", project.ProjectID, err.Error()))
	}
	return res, nil
}

// AttachProjectToRepo is the resolver for the attachProjectToRepo field.
func (r *mutationResolver) AttachProjectToRepo(ctx context.Context, projectID string) (*restModel.APIProjectRef, error) {
	usr := mustHaveUser(ctx)
	pRef, err := data.FindProjectById(ctx, projectID, false, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectID, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectID))
	}
	if err = pRef.AttachToRepo(ctx, usr); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("attaching project '%s' to repo: %s", projectID, err.Error()))
	}

	res := &restModel.APIProjectRef{}
	if err := res.BuildFromService(ctx, *pRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting project '%s' to APIProjectRef: %s", projectID, err.Error()))
	}
	return res, nil
}

// CreateProject is the resolver for the createProject field.
func (r *mutationResolver) CreateProject(ctx context.Context, project restModel.APIProjectRef) (*restModel.APIProjectRef, error) {
	dbProjectRef, err := project.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting APIProjectRef '%s' to service: %s", utility.FromStringPtr(project.Id), err.Error()))
	}
	u := gimlet.GetUser(ctx).(*user.DBUser)

	if created, err := data.CreateProject(ctx, evergreen.GetEnvironment(), dbProjectRef, u); err != nil {
		if !created {
			apiErr, ok := err.(gimlet.ErrorResponse)
			if ok {
				if apiErr.StatusCode == http.StatusBadRequest {
					return nil, InputValidationError.Send(ctx, apiErr.Message)
				}
			}
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		graphql.AddError(ctx, PartialError.Send(ctx, err.Error()))
	}

	projectIdentifier := utility.FromStringPtr(project.Identifier)
	projectRef, err := model.FindBranchProjectRef(ctx, projectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectIdentifier, err.Error()))
	}
	if projectRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectIdentifier))
	}
	apiProjectRef := restModel.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(ctx, *projectRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting project '%s' to APIProjectRef: %s", projectIdentifier, err.Error()))
	}

	return &apiProjectRef, nil
}

// CopyProject is the resolver for the copyProject field.
func (r *mutationResolver) CopyProject(ctx context.Context, project restModel.CopyProjectOpts) (*restModel.APIProjectRef, error) {
	projectRef, err := data.CopyProject(ctx, evergreen.GetEnvironment(), project)
	if projectRef == nil && err != nil {
		apiErr, ok := err.(gimlet.ErrorResponse) // make sure bad request errors are handled correctly; all else should be treated as internal server error
		if ok {
			if apiErr.StatusCode == http.StatusBadRequest {
				return nil, InputValidationError.Send(ctx, apiErr.Message)
			}
			// StatusNotFound and other error codes are really internal errors bc we determine this input
			return nil, InternalServerError.Send(ctx, apiErr.Message)
		}
		return nil, InternalServerError.Send(ctx, err.Error())

	}
	if err != nil {
		// Use AddError to bypass gqlgen restriction that data and errors cannot be returned in the same response
		// https://github.com/99designs/gqlgen/issues/1191
		graphql.AddError(ctx, PartialError.Send(ctx, err.Error()))
	}
	return projectRef, nil
}

// DeactivateStepbackTask is the resolver for the deactivateStepbackTask field.
func (r *mutationResolver) DeactivateStepbackTask(ctx context.Context, opts DeactivateStepbackTaskInput) (bool, error) {
	usr := mustHaveUser(ctx)
	if err := task.DeactivateStepbackTask(ctx, opts.ProjectID, opts.BuildVariantName, opts.TaskName, usr.Username()); err != nil {
		return false, InternalServerError.Send(ctx, err.Error())
	}
	return true, nil
}

// DefaultSectionToRepo is the resolver for the defaultSectionToRepo field.
func (r *mutationResolver) DefaultSectionToRepo(ctx context.Context, opts DefaultSectionToRepoInput) (*string, error) {
	usr := mustHaveUser(ctx)
	if err := model.DefaultSectionToRepo(ctx, opts.ProjectID, model.ProjectPageSection(opts.Section), usr.Username()); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("defaulting to repo for section: %s", err.Error()))
	}
	return &opts.ProjectID, nil
}

// DeleteGithubAppCredentials is the resolver for the deleteGithubAppCredentials field.
func (r *mutationResolver) DeleteGithubAppCredentials(ctx context.Context, opts DeleteGithubAppCredentialsInput) (*DeleteGithubAppCredentialsPayload, error) {
	usr := mustHaveUser(ctx)
	app, err := githubapp.FindOneGitHubAppAuth(ctx, opts.ProjectID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching GitHub app for project '%s': %s", opts.ProjectID, err.Error()))
	}
	if app == nil {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("project '%s' does not have a GitHub app defined", opts.ProjectID))
	}
	if err = githubapp.RemoveGitHubAppAuth(ctx, app); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("removing GitHub app auth for project '%s': %s", opts.ProjectID, err.Error()))
	}
	before := model.ProjectSettings{
		GitHubAppAuth: *app,
	}
	after := model.ProjectSettings{
		GitHubAppAuth: githubapp.GithubAppAuth{},
	}
	if err = model.LogProjectModified(ctx, opts.ProjectID, usr.Id, &before, &after); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("logging modification for project '%s': %s", opts.ProjectID, err.Error()))
	}
	return &DeleteGithubAppCredentialsPayload{
		OldAppID: int(app.AppID),
	}, nil
}

// DeleteProject is the resolver for the deleteProject field.
func (r *mutationResolver) DeleteProject(ctx context.Context, projectID string) (bool, error) {
	if err := data.HideBranch(ctx, projectID); err != nil {
		gimletErr, ok := err.(gimlet.ErrorResponse)
		if ok {
			return false, mapHTTPStatusToGqlError(ctx, gimletErr.StatusCode, err)
		}
		return false, InternalServerError.Send(ctx, fmt.Sprintf("deleting project '%s': %s", projectID, err.Error()))
	}
	return true, nil
}

// DetachProjectFromRepo is the resolver for the detachProjectFromRepo field.
func (r *mutationResolver) DetachProjectFromRepo(ctx context.Context, projectID string) (*restModel.APIProjectRef, error) {
	usr := mustHaveUser(ctx)
	pRef, err := data.FindProjectById(ctx, projectID, false, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectID, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectID))
	}
	if err = pRef.DetachFromRepo(ctx, usr); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("detaching project '%s' from repo: %s", projectID, err.Error()))
	}

	res := &restModel.APIProjectRef{}
	if err := res.BuildFromService(ctx, *pRef); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting project '%s' to APIProjectRef: %s", projectID, err.Error()))
	}
	return res, nil
}

// ForceRepotrackerRun is the resolver for the forceRepotrackerRun field.
func (r *mutationResolver) ForceRepotrackerRun(ctx context.Context, projectID string) (bool, error) {
	ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
	j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), projectID)
	if err := amboy.EnqueueUniqueJob(ctx, evergreen.GetEnvironment().RemoteQueue(), j); err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("creating Repotracker job: %s", err.Error()))
	}
	return true, nil
}

// PromoteVarsToRepo is the resolver for the promoteVarsToRepo field.
func (r *mutationResolver) PromoteVarsToRepo(ctx context.Context, opts PromoteVarsToRepoInput) (bool, error) {
	usr := mustHaveUser(ctx)
	if err := data.PromoteVarsToRepo(ctx, opts.ProjectID, opts.VarNames, usr.Username()); err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("promoting variables to repo for project '%s': %s", opts.ProjectID, err.Error()))

	}
	return true, nil
}

// SaveProjectSettingsForSection is the resolver for the saveProjectSettingsForSection field.
func (r *mutationResolver) SaveProjectSettingsForSection(ctx context.Context, projectSettings *restModel.APIProjectSettings, section ProjectSettingsSection) (*restModel.APIProjectSettings, error) {
	projectId := utility.FromStringPtr(projectSettings.ProjectRef.Id)
	usr := mustHaveUser(ctx)
	changes, err := data.SaveProjectSettingsForSection(ctx, projectId, projectSettings, model.ProjectPageSection(section), false, usr.Username())
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return changes, nil
}

// SaveRepoSettingsForSection is the resolver for the saveRepoSettingsForSection field.
func (r *mutationResolver) SaveRepoSettingsForSection(ctx context.Context, repoSettings *restModel.APIProjectSettings, section ProjectSettingsSection) (*restModel.APIProjectSettings, error) {
	projectId := utility.FromStringPtr(repoSettings.ProjectRef.Id)
	usr := mustHaveUser(ctx)
	changes, err := data.SaveProjectSettingsForSection(ctx, projectId, repoSettings, model.ProjectPageSection(section), true, usr.Username())
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return changes, nil
}

// SetLastRevision is the resolver for the setLastRevision field.
func (r *mutationResolver) SetLastRevision(ctx context.Context, opts SetLastRevisionInput) (*SetLastRevisionPayload, error) {
	if len(opts.Revision) < gitHashLength {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("insufficient length: must provide %d characters for revision", gitHashLength))
	}

	project, err := model.FindBranchProjectRef(ctx, opts.ProjectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", opts.ProjectIdentifier, err.Error()))
	}
	if project == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", opts.ProjectIdentifier))
	}

	if err = model.UpdateLastRevision(ctx, project.Id, opts.Revision); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("updating last revision for '%s': %s", opts.ProjectIdentifier, err.Error()))
	}

	if err = project.SetRepotrackerError(ctx, &model.RepositoryErrorDetails{}); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("clearing repotracker error for '%s': %s", opts.ProjectIdentifier, err.Error()))
	}

	// Run repotracker job because the last revision for the project has been updated.
	ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
	j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), project.Id)
	if err = amboy.EnqueueUniqueJob(ctx, evergreen.GetEnvironment().RemoteQueue(), j); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("creating Repotracker catchup job: %s", err.Error()))
	}
	return &SetLastRevisionPayload{
		MergeBaseRevision: opts.Revision,
	}, nil
}

// AttachVolumeToHost is the resolver for the attachVolumeToHost field.
func (r *mutationResolver) AttachVolumeToHost(ctx context.Context, volumeAndHost VolumeHost) (bool, error) {
	statusCode, err := cloud.AttachVolume(ctx, volumeAndHost.VolumeID, volumeAndHost.HostID)
	if err != nil {
		return false, mapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	return statusCode == http.StatusOK, nil
}

// DetachVolumeFromHost is the resolver for the detachVolumeFromHost field.
func (r *mutationResolver) DetachVolumeFromHost(ctx context.Context, volumeID string) (bool, error) {
	statusCode, err := cloud.DetachVolume(ctx, volumeID)
	if err != nil {
		return false, mapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	return statusCode == http.StatusOK, nil
}

// EditSpawnHost is the resolver for the editSpawnHost field.
func (r *mutationResolver) EditSpawnHost(ctx context.Context, spawnHost *EditSpawnHostInput) (*restModel.APIHost, error) {
	var v *host.Volume
	usr := mustHaveUser(ctx)
	h, err := host.FindOneByIdOrTag(ctx, spawnHost.HostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching host '%s': %s", spawnHost.HostID, err.Error()))
	}
	if h == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("host '%s' not found", spawnHost.HostID))
	}

	if !host.CanUpdateSpawnHost(ctx, h, usr) {
		return nil, Forbidden.Send(ctx, fmt.Sprintf("not authorized to modify host '%s'", spawnHost.HostID))
	}

	opts := host.HostModifyOptions{}
	if spawnHost.DisplayName != nil {
		opts.NewName = *spawnHost.DisplayName
	}
	if spawnHost.NoExpiration != nil {
		opts.NoExpiration = spawnHost.NoExpiration
	}
	if spawnHost.Expiration != nil {
		opts.AddHours = (*spawnHost.Expiration).Sub(h.ExpirationTime)
	}
	if spawnHost.InstanceType != nil {
		var config *evergreen.Settings
		config, err = evergreen.GetConfig(ctx)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
		}
		allowedTypes := config.Providers.AWS.AllowedInstanceTypes

		err = cloud.CheckInstanceTypeValid(ctx, h.Distro, utility.FromStringPtr(spawnHost.InstanceType), allowedTypes)
		if err != nil {
			return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid instance type '%s': %s", utility.FromStringPtr(spawnHost.InstanceType), err.Error()))
		}
		opts.InstanceType = *spawnHost.InstanceType
	}
	if spawnHost.AddedInstanceTags != nil || spawnHost.DeletedInstanceTags != nil {
		addedTags := []host.Tag{}
		deletedTags := []string{}
		for _, tag := range spawnHost.AddedInstanceTags {
			tag.CanBeModified = true
			addedTags = append(addedTags, *tag)
		}
		for _, tag := range spawnHost.DeletedInstanceTags {
			deletedTags = append(deletedTags, tag.Key)
		}
		opts.AddInstanceTags = addedTags
		opts.DeleteInstanceTags = deletedTags
	}
	if spawnHost.Volume != nil {
		v, err = host.FindVolumeByID(ctx, *spawnHost.Volume)
		if err != nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("fetching volume '%s': %s", utility.FromStringPtr(spawnHost.Volume), err.Error()))
		}
		if v.AvailabilityZone != h.Zone {
			return nil, InputValidationError.Send(ctx, "mounted volume and spawn host must be in the same availability zone")
		}
		opts.AttachVolume = *spawnHost.Volume
	}
	if spawnHost.PublicKey != nil {
		if h.Status != evergreen.HostRunning {
			return nil, InputValidationError.Send(ctx, fmt.Sprintf("host must be running to add a public key but is %s", h.Status))
		}
		if utility.FromBoolPtr(spawnHost.SavePublicKey) {
			if err = savePublicKey(ctx, *spawnHost.PublicKey); err != nil {
				return nil, err
			}
		}
		opts.AddKey = spawnHost.PublicKey.Key
		if opts.AddKey == "" {
			opts.AddKey, err = usr.GetPublicKey(spawnHost.PublicKey.Name)
			if err != nil {
				return nil, InputValidationError.Send(ctx, fmt.Sprintf("no matching key found for name '%s'", spawnHost.PublicKey.Name))
			}
		}
	}

	if spawnHost.SleepSchedule != nil {
		if err = h.UpdateSleepSchedule(ctx, *spawnHost.SleepSchedule, time.Now()); err != nil {
			gimletErr, ok := err.(gimlet.ErrorResponse)
			if ok {
				return nil, mapHTTPStatusToGqlError(ctx, gimletErr.StatusCode, err)
			}
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("setting sleep schedule for host '%s': %s", h.Id, err.Error()))
		}
	}

	if err = cloud.ModifySpawnHost(ctx, evergreen.GetEnvironment(), h, opts); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("modifying spawn host '%s': %s", h.Id, err.Error()))
	}
	if spawnHost.ServicePassword != nil {
		_, err = cloud.SetHostRDPPassword(ctx, evergreen.GetEnvironment(), h, *spawnHost.ServicePassword)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("setting spawn host password: %s", err.Error()))
		}
	}

	apiHost := restModel.APIHost{}
	apiHost.BuildFromService(h, nil)
	return &apiHost, nil
}

// MigrateVolume is the resolver for the migrateVolume field.
func (r *mutationResolver) MigrateVolume(ctx context.Context, volumeID string, spawnHostInput *SpawnHostInput) (bool, error) {
	usr := mustHaveUser(ctx)
	options, err := getHostRequestOptions(ctx, usr, spawnHostInput)
	if err != nil {
		return false, err
	}
	return data.MigrateVolume(ctx, volumeID, options, usr, evergreen.GetEnvironment())
}

// SpawnHost is the resolver for the spawnHost field.
func (r *mutationResolver) SpawnHost(ctx context.Context, spawnHostInput *SpawnHostInput) (*restModel.APIHost, error) {
	usr := mustHaveUser(ctx)
	options, err := getHostRequestOptions(ctx, usr, spawnHostInput)
	if err != nil {
		return nil, err
	}

	d, err := distro.FindOneId(ctx, options.DistroID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching distro '%s': %s", options.DistroID, err.Error()))
	}
	if d == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("distro '%s' not found", options.DistroID))
	}

	// Some distros only support a subset of the available regions.
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	availableRegions := d.GetRegionsList(settings.Providers.AWS.AllowedRegions)
	if !utility.StringSliceContains(availableRegions, options.Region) {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("distro '%s' only supports spawn hosts in the following regions: %s", options.DistroID, strings.Join(availableRegions, ", ")))
	}

	// Only admins can spawn admin-only distros.
	if !usr.HasDistroCreatePermission(ctx) {
		if d.AdminOnly {
			// Admin-only distros can only be spawned by distro admins.
			return nil, Forbidden.Send(ctx, fmt.Sprintf("not authorized to spawn host in admin-only distro '%s'", options.DistroID))
		}
	}

	spawnHost, err := data.NewIntentHost(ctx, options, usr, evergreen.GetEnvironment())
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("spawning host: %s", err.Error()))
	}
	if spawnHost == nil {
		return nil, InternalServerError.Send(ctx, "creating intent for spawn host")
	}
	apiHost := restModel.APIHost{}
	apiHost.BuildFromService(spawnHost, nil)
	return &apiHost, nil
}

// SpawnVolume is the resolver for the spawnVolume field.
func (r *mutationResolver) SpawnVolume(ctx context.Context, spawnVolumeInput SpawnVolumeInput) (bool, error) {
	err := validateVolumeExpirationInput(ctx, spawnVolumeInput.Expiration, spawnVolumeInput.NoExpiration)
	if err != nil {
		return false, err
	}

	usr := mustHaveUser(ctx)
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	maxVolumeFromSettings := settings.Providers.AWS.MaxVolumeSizePerUser
	if err := cloud.CheckVolumeLimitExceeded(ctx, usr.Username(), spawnVolumeInput.Size, maxVolumeFromSettings); err != nil {
		return false, InputValidationError.Send(ctx, err.Error())
	}

	volumeRequest := host.Volume{
		AvailabilityZone: spawnVolumeInput.AvailabilityZone,
		Size:             int32(spawnVolumeInput.Size),
		Type:             spawnVolumeInput.Type,
		CreatedBy:        usr.Id,
	}
	vol, statusCode, err := cloud.RequestNewVolume(ctx, volumeRequest)
	if err != nil {
		return false, mapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	if vol == nil {
		return false, InternalServerError.Send(ctx, "creating volume")
	}
	var additionalOptions restModel.VolumeModifyOptions
	if spawnVolumeInput.Expiration != nil {
		var newExpiration time.Time
		newExpiration, err = restModel.FromTimePtr(spawnVolumeInput.Expiration)
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("converting expiration time: %s", err.Error()))
		}
		additionalOptions.Expiration = newExpiration
	} else if spawnVolumeInput.NoExpiration != nil && *spawnVolumeInput.NoExpiration {
		// this value should only ever be true or nil
		additionalOptions.NoExpiration = true
	}
	err = applyVolumeOptions(ctx, *vol, additionalOptions)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("applying expiration options to volume '%s': %s", vol.ID, err.Error()))
	}
	if spawnVolumeInput.Host != nil {
		statusCode, err := cloud.AttachVolume(ctx, vol.ID, utility.FromStringPtr(spawnVolumeInput.Host))
		if err != nil {
			return false, mapHTTPStatusToGqlError(ctx, statusCode, werrors.Wrapf(err, "attaching volume '%s' to host: %s", vol.ID, err.Error()))
		}
	}
	return true, nil
}

// RemoveVolume is the resolver for the removeVolume field.
func (r *mutationResolver) RemoveVolume(ctx context.Context, volumeID string) (bool, error) {
	statusCode, err := cloud.DeleteVolume(ctx, volumeID)
	if err != nil {
		return false, mapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	return statusCode == http.StatusOK, nil
}

// UpdateSpawnHostStatus is the resolver for the updateSpawnHostStatus field.
func (r *mutationResolver) UpdateSpawnHostStatus(ctx context.Context, updateSpawnHostStatusInput UpdateSpawnHostStatusInput) (*restModel.APIHost, error) {
	hostID := updateSpawnHostStatusInput.HostID
	action := updateSpawnHostStatusInput.Action
	shouldKeepOff := utility.FromBoolPtr(updateSpawnHostStatusInput.ShouldKeepOff)

	h, err := host.FindOneByIdOrTag(ctx, hostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching host '%s': %s", hostID, err.Error()))
	}
	if h == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("host '%s' not found", hostID))
	}
	usr := mustHaveUser(ctx)
	env := evergreen.GetEnvironment()

	if !host.CanUpdateSpawnHost(ctx, h, usr) {
		return nil, Forbidden.Send(ctx, fmt.Sprintf("not authorized to modify host '%s'", hostID))
	}

	var httpStatus int
	switch action {
	case SpawnHostStatusActionsStart:
		httpStatus, err = data.StartSpawnHost(ctx, env, usr, h)
	case SpawnHostStatusActionsStop:
		httpStatus, err = data.StopSpawnHost(ctx, env, usr, h, shouldKeepOff)
	case SpawnHostStatusActionsTerminate:
		httpStatus, err = data.TerminateSpawnHost(ctx, env, usr, h)
	case SpawnHostStatusActionsReboot:
		httpStatus, err = data.RebootSpawnHost(ctx, env, usr, h)
	default:
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("finding matching status for action '%s'", action))
	}
	if err != nil {
		if httpStatus == http.StatusInternalServerError {
			var parsedUrl, _ = url.Parse("/graphql/query")
			grip.Error(message.WrapError(err, message.Fields{
				"method":  "POST",
				"url":     parsedUrl,
				"code":    httpStatus,
				"action":  action,
				"request": gimlet.GetRequestID(ctx),
				"stack":   string(debug.Stack()),
			}))
		}
		return nil, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}
	apiHost := restModel.APIHost{}
	apiHost.BuildFromService(h, nil)
	return &apiHost, nil
}

// UpdateVolume is the resolver for the updateVolume field.
func (r *mutationResolver) UpdateVolume(ctx context.Context, updateVolumeInput UpdateVolumeInput) (bool, error) {
	volume, err := host.FindVolumeByID(ctx, updateVolumeInput.VolumeID)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("fetching volume '%s': %s", updateVolumeInput.VolumeID, err.Error()))
	}
	if volume == nil {
		return false, ResourceNotFound.Send(ctx, fmt.Sprintf("volume '%s' not found", updateVolumeInput.VolumeID))
	}
	err = validateVolumeExpirationInput(ctx, updateVolumeInput.Expiration, updateVolumeInput.NoExpiration)
	if err != nil {
		return false, err
	}
	err = validateVolumeName(ctx, updateVolumeInput.Name)
	if err != nil {
		return false, err
	}
	var updateOptions restModel.VolumeModifyOptions
	if updateVolumeInput.NoExpiration != nil {
		if *updateVolumeInput.NoExpiration {
			// this value should only ever be true or nil
			updateOptions.NoExpiration = true
		} else {
			// this value should only ever be true or nil
			updateOptions.HasExpiration = true
		}
	}
	if updateVolumeInput.Expiration != nil {
		var newExpiration time.Time
		newExpiration, err = restModel.FromTimePtr(updateVolumeInput.Expiration)
		if err != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("parsing time: %s", err.Error()))
		}
		updateOptions.Expiration = newExpiration
	}
	if updateVolumeInput.Name != nil {
		updateOptions.NewName = utility.FromStringPtr(updateVolumeInput.Name)
	}
	if updateVolumeInput.Size != nil {
		newSize := int32(utility.FromIntPtr(updateVolumeInput.Size))
		if newSize < volume.Size {
			// AWS does not allow decreasing volume size.
			return false, InputValidationError.Send(ctx, fmt.Sprintf("new size must be equal to or greater than current size (%dGiB)", volume.Size))
		}

		sizeIncrease := int(newSize - volume.Size)
		if sizeIncrease > 0 {
			usr := mustHaveUser(ctx)
			settings, err := evergreen.GetConfig(ctx)
			if err != nil {
				return false, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
			}
			maxVolumeFromSettings := settings.Providers.AWS.MaxVolumeSizePerUser
			if err := cloud.CheckVolumeLimitExceeded(ctx, usr.Username(), sizeIncrease, maxVolumeFromSettings); err != nil {
				return false, InputValidationError.Send(ctx, err.Error())
			}
		}

		updateOptions.Size = int32(utility.FromIntPtr(updateVolumeInput.Size))
	}
	err = applyVolumeOptions(ctx, *volume, updateOptions)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("updating volume '%s': %s", volume.ID, err.Error()))
	}
	return true, nil
}

// AbortTask is the resolver for the abortTask field.
func (r *mutationResolver) AbortTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	t, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	user := gimlet.GetUser(ctx).Username()
	err = model.AbortTask(ctx, taskID, user)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("aborting task '%s': %s", taskID, err.Error()))
	}
	t, err = task.FindOneId(ctx, taskID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	apiTask, err := getAPITaskFromTask(ctx, r.sc.GetURL(), *t)
	return apiTask, err
}

// OverrideTaskDependencies is the resolver for the overrideTaskDependencies field.
func (r *mutationResolver) OverrideTaskDependencies(ctx context.Context, taskID string) (*restModel.APITask, error) {
	currentUser := mustHaveUser(ctx)
	t, err := task.FindByIdExecution(ctx, taskID, nil)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	if err = t.SetOverrideDependencies(ctx, currentUser.Username()); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("overriding dependencies for task '%s': %s", taskID, err.Error()))
	}
	return getAPITaskFromTask(ctx, r.sc.GetURL(), *t)
}

// RestartTask is the resolver for the restartTask field.
func (r *mutationResolver) RestartTask(ctx context.Context, taskID string, failedOnly bool) (*restModel.APITask, error) {
	usr := mustHaveUser(ctx)
	username := usr.Username()
	t, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	if evergreen.IsGithubMergeQueueRequester(t.Requester) {
		return nil, InputValidationError.Send(ctx, "Merge queue tasks cannot be manually restarted.")
	}
	if err := model.ResetTaskOrDisplayTask(ctx, evergreen.GetEnvironment().Settings(), t, username, evergreen.UIPackage, failedOnly, nil); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("restarting task '%s': %s", taskID, err.Error()))
	}
	t, err = task.FindOneIdAndExecutionWithDisplayStatus(ctx, taskID, nil)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	apiTask, err := getAPITaskFromTask(ctx, r.sc.GetURL(), *t)
	return apiTask, err
}

// ScheduleTasks is the resolver for the scheduleTasks field.
func (r *mutationResolver) ScheduleTasks(ctx context.Context, versionID string, taskIds []string) ([]*restModel.APITask, error) {
	dbTasks, err := findAllTasksByIds(ctx, taskIds...)
	if err != nil {
		return nil, err
	}
	for _, t := range dbTasks {
		if t.Version != versionID && t.ParentPatchID != versionID {
			return nil, InputValidationError.Send(ctx, fmt.Sprintf("task '%s' does not belong to version '%s'", t.Id, versionID))
		}
	}

	scheduledTasks := []*restModel.APITask{}
	scheduled, err := setManyTasksScheduled(ctx, r.sc.GetURL(), true, taskIds...)
	if err != nil {
		return scheduledTasks, InternalServerError.Send(ctx, fmt.Sprintf("scheduling tasks: %s", err.Error()))
	}
	scheduledTasks = append(scheduledTasks, scheduled...)
	return scheduledTasks, nil
}

// SetTaskPriority is the resolver for the setTaskPriority field.
func (r *mutationResolver) SetTaskPriority(ctx context.Context, taskID string, priority int) (*restModel.APITask, error) {
	return setSingleTaskPriority(ctx, r.sc.GetURL(), taskID, priority)
}

// SetTaskPriorities is the resolver for the setTaskPriorities field.
func (r *mutationResolver) SetTaskPriorities(ctx context.Context, taskPriorities []*TaskPriority) ([]*restModel.APITask, error) {
	tasks := []*restModel.APITask{}
	for _, t := range taskPriorities {
		tsk, err := setSingleTaskPriority(ctx, r.sc.GetURL(), t.TaskID, t.Priority)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, tsk)
	}
	return tasks, nil
}

// UnscheduleTask is the resolver for the unscheduleTask field.
func (r *mutationResolver) UnscheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	scheduled, err := setManyTasksScheduled(ctx, r.sc.GetURL(), false, taskID)
	if err != nil {
		return nil, err
	}
	if len(scheduled) == 0 {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", taskID))
	}
	return scheduled[0], nil
}

// QuarantineTest is the resolver for the quarantineTest field.
func (r *mutationResolver) QuarantineTest(ctx context.Context, opts QuarantineTestInput) (*QuarantineTestPayload, error) {
	t, err := task.FindOneId(ctx, opts.TaskID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", opts.TaskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", opts.TaskID))
	}
	if err = data.SetTestQuarantined(ctx, t.Project, t.BuildVariant, t.DisplayName, opts.TestName, true); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("quarantining test '%s' on task '%s' on build variant '%s' on project '%s' : %s", opts.TestName, opts.TaskID, t.BuildVariant, t.Project, err.Error()))
	}
	return &QuarantineTestPayload{
		Success: true,
	}, nil
}

// AddFavoriteProject is the resolver for the addFavoriteProject field.
func (r *mutationResolver) AddFavoriteProject(ctx context.Context, opts AddFavoriteProjectInput) (*restModel.APIProjectRef, error) {
	p, err := model.FindBranchProjectRef(ctx, opts.ProjectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", opts.ProjectIdentifier, err.Error()))
	}
	if p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", opts.ProjectIdentifier))
	}

	usr := mustHaveUser(ctx)
	err = usr.AddFavoritedProject(ctx, opts.ProjectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	apiProjectRef := restModel.APIProjectRef{}
	err = apiProjectRef.BuildFromService(ctx, *p)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting project '%s' to APIProjectRef: %s", opts.ProjectIdentifier, err.Error()))
	}
	return &apiProjectRef, nil
}

// ClearMySubscriptions is the resolver for the clearMySubscriptions field.
func (r *mutationResolver) ClearMySubscriptions(ctx context.Context) (int, error) {
	usr := mustHaveUser(ctx)
	username := usr.Username()
	subs, err := event.FindSubscriptionsByOwner(ctx, username, event.OwnerTypePerson)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("retrieving subscriptions for user '%s': %s", usr.Id, err.Error()))
	}
	subIDs := removeGeneralSubscriptions(usr, subs)
	err = data.DeleteSubscriptions(ctx, username, subIDs)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("deleting subscriptions for user '%s': %s", usr.Id, err.Error()))
	}
	return len(subIDs), nil
}

// CreatePublicKey is the resolver for the createPublicKey field.
func (r *mutationResolver) CreatePublicKey(ctx context.Context, publicKeyInput PublicKeyInput) ([]*restModel.APIPubKey, error) {
	err := savePublicKey(ctx, publicKeyInput)
	if err != nil {
		return nil, err
	}
	myPublicKeys := getMyPublicKeys(ctx)
	return myPublicKeys, nil
}

// DeleteCursorAPIKey is the resolver for the deleteCursorAPIKey field.
func (r *mutationResolver) DeleteCursorAPIKey(ctx context.Context) (*DeleteCursorAPIKeyPayload, error) {
	usr := mustHaveUser(ctx)

	sageConfig := &evergreen.SageConfig{}
	if err := sageConfig.Get(ctx); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Sage config: %s", err.Error()))
	}

	sageClient, err := thirdparty.NewSageClient(sageConfig.BaseURL)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, "Sage service is not configured")
	}
	defer sageClient.Close()

	result, err := sageClient.DeleteCursorAPIKey(ctx, usr.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("deleting Cursor API key: %s", err.Error()))
	}

	return &DeleteCursorAPIKeyPayload{
		Success: result.Success,
	}, nil
}

// DeleteSubscriptions is the resolver for the deleteSubscriptions field.
func (r *mutationResolver) DeleteSubscriptions(ctx context.Context, subscriptionIds []string) (int, error) {
	usr := mustHaveUser(ctx)
	username := usr.Username()

	if err := data.DeleteSubscriptions(ctx, username, subscriptionIds); err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("deleting subscriptions for user '%s': %s", usr.Id, err.Error()))
	}
	return len(subscriptionIds), nil
}

// RemoveFavoriteProject is the resolver for the removeFavoriteProject field.
func (r *mutationResolver) RemoveFavoriteProject(ctx context.Context, opts RemoveFavoriteProjectInput) (*restModel.APIProjectRef, error) {
	p, err := model.FindBranchProjectRef(ctx, opts.ProjectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", opts.ProjectIdentifier, err.Error()))
	}
	if p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", opts.ProjectIdentifier))
	}

	usr := mustHaveUser(ctx)
	err = usr.RemoveFavoriteProject(ctx, opts.ProjectIdentifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("removing project '%s': %s", opts.ProjectIdentifier, err.Error()))
	}
	apiProjectRef := restModel.APIProjectRef{}
	err = apiProjectRef.BuildFromService(ctx, *p)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting project '%s' to APIProjectRef: %s", opts.ProjectIdentifier, err.Error()))
	}
	return &apiProjectRef, nil
}

// RemovePublicKey is the resolver for the removePublicKey field.
func (r *mutationResolver) RemovePublicKey(ctx context.Context, keyName string) ([]*restModel.APIPubKey, error) {
	if !doesPublicKeyNameAlreadyExist(ctx, keyName) {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("key name '%s' does not exist", keyName))
	}
	err := mustHaveUser(ctx).DeletePublicKey(ctx, keyName)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("deleting public key: %s", err.Error()))
	}
	myPublicKeys := getMyPublicKeys(ctx)
	return myPublicKeys, nil
}

// ResetAPIKey is the resolver for the resetAPIKey field.
func (r *mutationResolver) ResetAPIKey(ctx context.Context) (*UserConfig, error) {
	usr := mustHaveUser(ctx)
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Evergreen configuration: %s", err.Error()))
	}
	if !usr.OnlyAPI && settings.ServiceFlags.StaticAPIKeysDisabled {
		return nil, Forbidden.Send(ctx, "static API keys are disabled")
	}
	newKey := utility.RandomString()
	if err := usr.UpdateAPIKey(ctx, newKey); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("updating user API key: %s", err.Error()))
	}
	return &UserConfig{
		User:   usr.Username(),
		APIKey: newKey,
	}, nil
}

// SaveSubscription is the resolver for the saveSubscription field.
func (r *mutationResolver) SaveSubscription(ctx context.Context, subscription restModel.APISubscription) (bool, error) {
	usr := mustHaveUser(ctx)
	username := usr.Username()
	idType, id, err := getResourceTypeAndIdFromSubscriptionSelectors(ctx, subscription.Selectors)
	if err != nil {
		return false, err
	}
	switch idType {
	case "task":
		t, taskErr := task.FindOneId(ctx, id)
		if taskErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("fetching task '%s': %s", id, taskErr.Error()))
		}
		if t == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("task '%s' not found", id))
		}
	case "build":
		b, buildErr := build.FindOneId(ctx, id)
		if buildErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("fetching build '%s': %s", id, buildErr.Error()))
		}
		if b == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("build '%s' not found", id))
		}
	case "version":
		v, versionErr := model.VersionFindOneId(ctx, id)
		if versionErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", id, versionErr.Error()))
		}
		if v == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", id))
		}
	case "project":
		p, projectErr := data.FindProjectById(ctx, id, false, false)
		if projectErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", id, projectErr.Error()))
		}
		if p == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", id))
		}
	default:
		return false, InputValidationError.Send(ctx, "selectors do not include a target version, build, project, or task ID")
	}
	err = data.SaveSubscriptions(ctx, username, []restModel.APISubscription{subscription}, false)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("saving subscription: %s", err.Error()))
	}
	return true, nil
}

// SetCursorAPIKey is the resolver for the setCursorAPIKey field.
func (r *mutationResolver) SetCursorAPIKey(ctx context.Context, apiKey string) (*SetCursorAPIKeyPayload, error) {
	usr := mustHaveUser(ctx)

	sageConfig := &evergreen.SageConfig{}
	if err := sageConfig.Get(ctx); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting Sage config: %s", err.Error()))
	}

	sageClient, err := thirdparty.NewSageClient(sageConfig.BaseURL)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, "Sage service is not configured")
	}
	defer sageClient.Close()

	result, err := sageClient.SetCursorAPIKey(ctx, usr.Id, apiKey)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("setting Cursor API key: %s", err.Error()))
	}

	return &SetCursorAPIKeyPayload{
		Success:     result.Success,
		KeyLastFour: utility.ToStringPtr(result.KeyLastFour),
	}, nil
}

// UpdateBetaFeatures is the resolver for the updateBetaFeatures field.
func (r *mutationResolver) UpdateBetaFeatures(ctx context.Context, opts UpdateBetaFeaturesInput) (*UpdateBetaFeaturesPayload, error) {
	usr := mustHaveUser(ctx)
	newBetaFeatureSettings := opts.BetaFeatures.ToService()

	if err := usr.UpdateBetaFeatures(ctx, newBetaFeatureSettings); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("updating beta features for user '%s': %s", usr.Id, err.Error()))
	}

	betaFeatures := restModel.APIBetaFeatures{}
	betaFeatures.BuildFromService(usr.BetaFeatures)
	return &UpdateBetaFeaturesPayload{
		BetaFeatures: &betaFeatures,
	}, nil
}

// UpdateParsleySettings is the resolver for the updateParsleySettings field.
func (r *mutationResolver) UpdateParsleySettings(ctx context.Context, opts UpdateParsleySettingsInput) (*UpdateParsleySettingsPayload, error) {
	usr := mustHaveUser(ctx)
	newSettings := opts.ParsleySettings.ToService()

	changes := parsley.MergeExistingParsleySettings(usr.ParsleySettings, newSettings)
	if err := usr.UpdateParsleySettings(ctx, changes); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("updating Parsley settings for user '%s': %s", usr.Id, err.Error()))
	}

	parsleySettings := restModel.APIParsleySettings{}
	parsleySettings.BuildFromService(usr.ParsleySettings)
	return &UpdateParsleySettingsPayload{
		ParsleySettings: &parsleySettings,
	}, nil
}

// UpdatePublicKey is the resolver for the updatePublicKey field.
func (r *mutationResolver) UpdatePublicKey(ctx context.Context, targetKeyName string, updateInfo PublicKeyInput) ([]*restModel.APIPubKey, error) {
	if !doesPublicKeyNameAlreadyExist(ctx, targetKeyName) {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("key name '%s' does not exist", targetKeyName))
	}
	if updateInfo.Name != targetKeyName && doesPublicKeyNameAlreadyExist(ctx, updateInfo.Name) {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("key name '%s' already exists", targetKeyName))
	}
	err := verifyPublicKey(ctx, updateInfo)
	if err != nil {
		return nil, err
	}
	usr := mustHaveUser(ctx)
	err = usr.UpdatePublicKey(ctx, targetKeyName, updateInfo.Name, updateInfo.Key)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("updating public key '%s': %s", targetKeyName, err.Error()))
	}
	myPublicKeys := getMyPublicKeys(ctx)
	return myPublicKeys, nil
}

// UpdateUserSettings is the resolver for the updateUserSettings field.
func (r *mutationResolver) UpdateUserSettings(ctx context.Context, userSettings *restModel.APIUserSettings) (bool, error) {
	usr := mustHaveUser(ctx)

	updatedUserSettings, err := restModel.UpdateUserSettings(ctx, usr, *userSettings)
	if err != nil {
		return false, InternalServerError.Send(ctx, err.Error())
	}
	err = data.UpdateSettings(ctx, usr, *updatedUserSettings)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("saving settings for user '%s': %s", usr.Id, err.Error()))
	}
	return true, nil
}

// RestartVersions is the resolver for the restartVersions field.
func (r *mutationResolver) RestartVersions(ctx context.Context, versionID string, abort bool, versionsToRestart []*model.VersionToRestart) ([]*restModel.APIVersion, error) {
	if len(versionsToRestart) == 0 {
		return nil, InputValidationError.Send(ctx, "No versions provided. You must provide at least one version to restart.")
	}
	modifications := model.VersionModification{
		Action:            evergreen.RestartAction,
		Abort:             abort,
		VersionsToRestart: versionsToRestart,
	}
	err := modifyVersionHandler(ctx, versionID, modifications)
	if err != nil {
		return nil, err
	}
	versions := []*restModel.APIVersion{}
	for _, version := range versionsToRestart {
		if version.VersionId != nil {
			currVersionID := utility.FromStringPtr(version.VersionId)
			v, versionErr := model.VersionFindOneIdWithBuildVariants(ctx, currVersionID)
			if versionErr != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching version '%s': %s", currVersionID, versionErr.Error()))
			}
			if v == nil {
				return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", currVersionID))
			}
			apiVersion := restModel.APIVersion{}
			apiVersion.BuildFromService(ctx, *v)
			versions = append(versions, &apiVersion)
		}
	}
	return versions, nil
}

// ScheduleUndispatchedBaseTasks is the resolver for the scheduleUndispatchedBaseTasks field.
func (r *mutationResolver) ScheduleUndispatchedBaseTasks(ctx context.Context, versionID string) ([]*restModel.APITask, error) {
	opts := task.GetTasksByVersionOptions{
		Statuses:              evergreen.TaskFailureStatuses,
		IncludeExecutionTasks: true,
	}
	tasks, _, err := task.GetTasksByVersion(ctx, versionID, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting tasks for patch '%s': %s ", versionID, err.Error()))
	}

	scheduledTasks := []*restModel.APITask{}
	tasksToSchedule := make(map[string]bool)

	for _, t := range tasks {
		// If a task is a generated task don't schedule it until we get all of the generated tasks we want to generate
		if t.GeneratedBy == "" {
			// We can ignore an error while fetching tasks because this could just mean the task didn't exist on the base commit.
			baseTask, _ := t.FindTaskOnBaseCommit(ctx)
			if baseTask != nil && baseTask.Status == evergreen.TaskUndispatched {
				tasksToSchedule[baseTask.Id] = true
			}
			// If a task is generated lets find its base task if it exists otherwise we need to generate it
		} else if t.GeneratedBy != "" {
			baseTask, _ := t.FindTaskOnBaseCommit(ctx)
			// If the task is undispatched or doesn't exist on the base commit then we want to schedule
			if baseTask == nil {
				generatorTask, err := task.FindByIdExecution(ctx, t.GeneratedBy, nil)
				if err != nil {
					return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching generator task '%s': %s", t.GeneratedBy, err.Error()))
				}
				if generatorTask != nil {
					baseGeneratorTask, _ := generatorTask.FindTaskOnBaseCommit(ctx)
					// If baseGeneratorTask is nil then it didn't exist on the base task and we can't do anything
					if baseGeneratorTask != nil && baseGeneratorTask.Status == evergreen.TaskUndispatched {
						err = baseGeneratorTask.SetGeneratedTasksToActivate(ctx, t.BuildVariant, t.DisplayName)
						if err != nil {
							return nil, InternalServerError.Send(ctx, fmt.Sprintf("activating generated tasks: %s", err.Error()))
						}
						tasksToSchedule[baseGeneratorTask.Id] = true

					}
				}
			} else if baseTask.Status == evergreen.TaskUndispatched {
				tasksToSchedule[baseTask.Id] = true
			}

		}
	}

	taskIDs := []string{}
	for taskId := range tasksToSchedule {
		taskIDs = append(taskIDs, taskId)
	}
	scheduled, err := setManyTasksScheduled(ctx, r.sc.GetURL(), true, taskIDs...)
	if err != nil {
		return nil, err
	}
	scheduledTasks = append(scheduledTasks, scheduled...)
	// sort scheduledTasks by display name to guarantee the order of the tasks
	sort.Slice(scheduledTasks, func(i, j int) bool {
		return utility.FromStringPtr(scheduledTasks[i].DisplayName) < utility.FromStringPtr(scheduledTasks[j].DisplayName)
	})

	return scheduledTasks, nil
}

// SetVersionPriority is the resolver for the setVersionPriority field.
func (r *mutationResolver) SetVersionPriority(ctx context.Context, versionID string, priority int) (*string, error) {
	modifications := model.VersionModification{
		Action:   evergreen.SetPriorityAction,
		Priority: int64(priority),
	}
	err := modifyVersionHandler(ctx, versionID, modifications)
	if err != nil {
		return nil, err
	}
	return &versionID, nil
}

// UnscheduleVersionTasks is the resolver for the unscheduleVersionTasks field.
func (r *mutationResolver) UnscheduleVersionTasks(ctx context.Context, versionID string, abort bool) (*string, error) {
	modifications := model.VersionModification{
		Action: evergreen.SetActiveAction,
		Active: false,
		Abort:  abort,
	}
	err := modifyVersionHandler(ctx, versionID, modifications)
	if err != nil {
		return nil, err
	}
	return &versionID, nil
}

// Mutation returns MutationResolver implementation.
func (r *Resolver) Mutation() MutationResolver { return &mutationResolver{r} }

type mutationResolver struct{ *Resolver }
